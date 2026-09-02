# StudyGuardian：AI 学习监督系统开发文档

> 文档版本：v0.3  
> 日期：2026-09-02  
> 目标平台：Windows 10 / Windows 11  
> 主开发环境：WSL2 Ubuntu  
> 开发方式：AI 主导开发（WSL Codex CLI 为主开发通道；Windows 负责真实运行与必要 E2E；开发完成后再进行独立 Codex 全量审计）  
> 项目定位：个人自用、长期后台运行、低打扰的 AI 学习监督系统  
> 暂定项目名：`StudyGuardian`
> 状态：v0.3 开发执行版；当前 4 项基础 PoC 已人工验证通过，Codex 开工后先补 localhost 三组件通信 PoC，再连续推进正式开发

## v0.3 核心修订

相对 v0.2，本版本进一步冻结以下开发执行约束：

1. Windows 项目运行、依赖、配置、数据、日志全部统一到 `D:\StudyGuardianDev`；项目不再把 Windows 侧文件散落到系统盘默认目录。
2. WSL 对应 Windows 路径统一为 `/mnt/d/StudyGuardianDev`。WSL 正式源码仍以 `~/projects/study-guardian` 为 Source of Truth。
3. 当前环境已经人工验证通过：ActivityWatch Window / AFK、WSL→Windows Go EXE、Desktop Pet Windows smoke test、`python-mss` 截图。Codex 不重复安装和推翻这些环境。
4. Phase 0 尚未完成的关键项为：Pet → Supervisor → Sensor 的 localhost 通信 PoC，以及部署脚本不得破坏 D 盘持久数据目录的安全检查。Codex 开工后优先补齐。
5. 本轮采用“单主开发通道”：WSL Codex CLI 可连续开发，不需要每个小任务等待人工审查；除非出现无法自行解决的硬阻塞，否则继续执行后续任务。
6. Git 必须小步提交：完成一个独立功能/修复并通过对应测试后立即 commit；远端可用时立即 push。禁止把一个 Phase 或大量功能压成一个最终大提交。
7. 所有 AI 面向用户的开发进度汇报、阶段总结、失败说明、最终汇报与审计报告必须使用中文；代码、标识符和 Conventional Commit message 可保持英文。
8. 开发结束后再启动一次独立 Codex 全量复查/审计，覆盖架构约束、测试、Windows/WSL 路径、隐私、安全、Git 历史、未完成项和技术债。

继承并继续有效的 v0.2 架构约束：

- 不使用单一 `ObservedState`，拆分 `InteractionState`、`TaskRelation`、`PrivacyState`。
- Build 与 Deploy 分离，不直接覆盖正在运行的 Windows `.exe`。
- Pet 只作为 UI Shell，禁止形成第二套监督逻辑。
- Supervisor 默认 `127.0.0.1:17321`，Screen Sensor 默认 `127.0.0.1:17322`，Pet 只连接 Supervisor。
- `UserMode` 状态转移、跨午夜、Sleep/Lock 时间语义必须可测试。
- ActivityWatch bucket 动态发现，禁止硬编码机器相关 bucket ID。

---

## 1. 项目背景

本项目解决的不是“统计今天电脑用了多久”，而是解决以下实际问题：

1. 学习高度依赖电脑和 AI，正常学习过程中会频繁在 VS Code、浏览器、ChatGPT、GitHub、文档、课程视频之间切换。
2. 游戏、短视频、小说等高刺激娱乐一旦启动，容易持续很久，仅靠使用后的意志力很难及时停止。
3. 手机无法由电脑直接监控，但如果学习时长时间没有键鼠操作、屏幕也长期不变化，可以间接判断“学习链条可能已经中断”。
4. 作息和学习节奏不固定时，很容易出现“电脑开了一上午，但一直没有真正开始学习”的情况。
5. 单纯晚间复盘发现问题太晚，需要在刚刚跑偏时进行低干扰提醒。
6. 系统不能变成“电子家长”，正常休息和娱乐必须允许存在；重点是监督娱乐是否侵占本应学习的时间。

因此系统核心不是“封锁娱乐”，而是：

> **让用户主动声明当前是学习还是休息；系统持续观察电脑行为，在学习链条中断、明显跑偏或休息过长时及时提醒。**

---

# 2. 总体设计原则

## 2.1 开源优先

凡是已有成熟开源能力，不重新造基础设施。

优先复用：

- ActivityWatch：窗口、浏览器、AFK、时间线；
- 开源桌宠项目：透明窗口、动画、托盘、气泡、开机启动；
- `python-mss`：Windows 屏幕捕获；
- SQLite：本地状态数据；
- 现成图像哈希 / OCR / AI SDK；
- Windows 原生通知或成熟开源通知库。

我们自己重点开发：

1. **Supervisor：学习监督大脑**
2. **Study Pet：针对学习监督的桌宠交互**
3. **Screen Sensor：非常薄的屏幕采集和变化检测适配层**
4. **AI Classification：与当前学习任务相关的语义判断**

---

## 2.2 不修改 ActivityWatch 主项目

ActivityWatch 作为外部基础设施直接安装和运行。

原因：

- 官方明确支持 Windows；
- 已有窗口和 AFK watcher；
- 本地存储；
- 提供 REST API；
- 项目成熟；
- 修改主项目会增加维护成本。

Supervisor 通过 Adapter 访问 ActivityWatch。

即使未来 ActivityWatch API 变化，也只修改 Adapter，不影响核心状态机。

---

## 2.3 手动意图与系统观察必须分离

这是项目非常重要的架构约束。

### 用户手动状态 `UserMode`

```text
STANDBY
STUDY
BREAK
OFF
```

它表示：

> “用户告诉系统自己现在打算干什么。”

### 系统观察不是单一状态

v0.1 中使用单一 `ObservedState`：

```text
FOCUSED
DISTRACTED
IDLE_STATIC
IDLE_DYNAMIC
UNKNOWN
SENSITIVE
```

该设计取消。

原因是这些值属于不同维度，现实中可以同时成立。例如：

```text
UserMode = STUDY
InteractionState = IDLE_DYNAMIC
TaskRelation = FOCUSED
PrivacyState = NORMAL
```

表示：用户声明正在学习，已经较长时间没有键鼠输入，但屏幕仍在变化，且内容与当前学习任务相关，例如正在观看课程视频。

因此系统观察结果拆成三个正交维度：

```text
InteractionState
├── ACTIVE
├── IDLE_STATIC
├── IDLE_DYNAMIC
└── UNKNOWN

TaskRelation
├── FOCUSED
├── DISTRACTED
└── UNKNOWN

PrivacyState
├── NORMAL
└── SENSITIVE
```

约束：

1. `InteractionState` 只描述人与电脑/屏幕的交互状态；
2. `TaskRelation` 只描述当前内容与学习任务的语义关系；
3. `PrivacyState` 只描述截图/云端分析是否允许；
4. AI Classifier 主要负责 `TaskRelation`，不负责隐私门禁；
5. `PrivacyState=SENSITIVE` 必须在截图之前由本地规则确定。

推荐内部结构：

```go
type Observation struct {
    Interaction InteractionState
    Relation    TaskRelation
    Privacy     PrivacyState
    Confidence  float64
}
```

`UserMode` 与 `Observation` 也不能混为一体。

例如：

```text
UserMode = STUDY
TaskRelation = DISTRACTED
```

表示用户声明正在学习，但系统高置信度发现当前行为与任务无关。

又例如：

```text
UserMode = BREAK
TaskRelation = DISTRACTED
```

不需要因为娱乐内容提醒，因为当前就是合法休息时间。

## 2.4 规则优先，AI 兜底

不要把每一张截图都交给大模型。

推荐判断顺序：

```text
ActivityWatch 元数据
        ↓
本地规则能否判断？
        ↓
屏幕是否发生变化？
        ↓
OCR / 标题 / 域名能否判断？
        ↓
仍然无法判断
        ↓
视觉 AI
```

这样可以降低：

- API 费用；
- 延迟；
- 隐私泄露风险；
- AI 误判；
- 对网络和模型可用性的依赖。

即使 AI API 暂时不可用，监督系统的基础功能也必须继续工作。

---

# 3. 经过复核后的开源技术选型

## 3.1 ActivityWatch —— 保留，作为核心活动数据源

项目：`ActivityWatch/activitywatch`

许可证：MPL-2.0

用途：

- 当前前台应用；
- 窗口标题；
- 浏览器活动；
- 键盘 / 鼠标是否处于 AFK；
- 活动时间线；
- 本地 REST API。

结论：

> **直接安装 Windows 官方版本，不 Fork，不改源码。**

ActivityWatch Browser Watcher / 浏览器扩展属于**推荐增强项**，用于尽量获得更完整的当前标签页 URL、标题等信息，但不能成为基础监督能力的硬依赖。

ActivityWatch Adapter 必须先枚举可用 buckets，再依据 bucket metadata / client / type 选择当前主机的数据源。

禁止把类似以下机器相关 ID 写死在业务代码中：

```text
aw-watcher-window_MYPC
aw-watcher-afk_MYPC
aw-watcher-web-chrome_MYPC
```

所有查询必须使用有界时间范围，例如最近 5 分钟；不得为了实时监督每轮读取完整历史时间线。

---

## 3.2 桌宠底座 —— 先做 Windows 验证，再选定

首选候选：

`UIU-Developers-Hub/desktop-pet`

特点：

- Windows 10+；
- Python + PyQt6；
- MIT；
- 透明置顶桌宠；
- 托盘；
- 气泡；
- 开机启动；
- 动画；
- 已有 idle / work tracker / proactive nudge 等概念。

优点：

- 和我们的需求非常接近；
- Python 修改成本低；
- AI 很容易理解和改造；
- Windows-specific 能力已经存在。

风险：

- 项目很小；
- 自动测试不完整；
- 不应把 Supervisor 逻辑继续堆入桌宠项目。

因此使用方式：

> **把它当 UI Shell，而不是系统核心。**

初期先 Clone/Fork 验证：

1. Windows 能否稳定启动；
2. 透明窗口是否正常；
3. 高 DPI 是否正常；
4. 多显示器是否正常；
5. 托盘是否正常；
6. 动画是否正常；
7. 开机启动是否正常；
8. 长期挂后台是否有明显 CPU / RAM 异常。

若失败，可替换其他 PyQt / Qt 桌宠项目。

桌宠与 Supervisor 之间必须通过稳定的本地 API 通信，因此更换桌宠底座不应影响 Supervisor。

---

## 3.3 屏幕采集 —— 不再押宝小型 ActivityWatch Screenshot Watcher

前期评估过：

- `Srakai/aw-watcher-screenshot`
- `InertialG/aw-watcher-screenshot`
- `kepptic/aw-watcher-enhanced`

其中：

- `Srakai/aw-watcher-screenshot` 只明确实测过 macOS；
- `InertialG/aw-watcher-screenshot` 功能很好，支持周期截图、感知哈希、WebP、ActivityWatch，但仓库没有把 Windows 实测作为强保证；
- `aw-watcher-enhanced` 明确提供 Windows 安装方式，并包含 OCR、屏幕变化检测、分类等能力，可作为重要参考，但项目规模仍较小。

因此正式方案调整为：

### 默认方案

使用成熟开源库：

`BoboTiG/python-mss`

特点：

- MIT；
- 纯 Python；
- 明确支持 Windows；
- 多显示器；
- 性能成熟；
- 非常适合 AI / CV 场景。

我们只写非常薄的：

```text
Screen Sensor
```

负责：

1. 按 Supervisor 指令截图；
2. 计算屏幕变化；
3. 必要时生成低分辨率分析图；
4. 返回图像元数据；
5. 默认不永久保存截图。

这不是重新造截图基础设施，而是用成熟开源库做一个适配器。

### 备选方案

如果 `InertialG/aw-watcher-screenshot` 在 Windows 实测稳定，可以直接复用 / Fork，减少 Screen Sensor 自己的代码。

### 参考方案

阅读 `aw-watcher-enhanced` 的：

- Adaptive OCR；
- OCR diff；
- idle 降频；
- 分类规则；
- privacy exclusion；

但避免无必要地复制其整个架构。

---

# 4. 总体系统架构

```text
┌─────────────────────────────────────────────────────────┐
│                       Windows 11                        │
│                                                         │
│  ┌───────────────────────────────────────────────────┐  │
│  │                   Study Pet                       │  │
│  │        PyQt6 / Desktop Pet UI Shell               │  │
│  │                                                   │  │
│  │ • 开始学习 / 休息 / 结束今天                     │  │
│  │ • 修改当前任务                                   │  │
│  │ • 动画 / 气泡 / 托盘 / 用户反馈                  │  │
│  └───────────────────────┬───────────────────────────┘  │
│                          │ HTTP localhost               │
│                          ▼                              │
│  ┌───────────────────────────────────────────────────┐  │
│  │             Go Supervisor :17321                  │  │
│  │                                                   │  │
│  │ UserMode State Machine                            │  │
│  │ Observation Engine                                │  │
│  │ Privacy Gate                                      │  │
│  │ Rule Engine                                       │  │
│  │ TaskRelation Classifier                           │  │
│  │ Reminder Engine                                   │  │
│  │ SQLite                                            │  │
│  └──────────────┬───────────────────────┬────────────┘  │
│                 │ REST                  │ HTTP localhost│
│                 ▼                       ▼               │
│  ┌──────────────────────┐   ┌────────────────────────┐ │
│  │    ActivityWatch     │   │ Screen Sensor :17322   │ │
│  │                      │   │                        │ │
│  │ window / title       │   │ python-mss             │ │
│  │ URL when available   │   │ pHash / dHash          │ │
│  │ AFK                  │   │ capture / diff         │ │
│  │ bounded timeline     │   │ optional OCR           │ │
│  └──────────────────────┘   └────────────────────────┘ │
│                                                         │
└─────────────────────────────────────────────────────────┘
```

固定通信方向：

```text
Pet → Supervisor
Supervisor → ActivityWatch
Supervisor → Screen Sensor
```

禁止：

```text
Pet ↔ Screen Sensor
Pet → ActivityWatch
Screen Sensor → ActivityWatch
```

原因：Supervisor 必须是唯一业务协调者。

# 5. 模块职责

## 5.1 ActivityWatch

只负责“采集事实”。

不负责：

- 判断是否学习；
- 判断是否摸鱼；
- 发督学通知；
- 管理学习状态；
- 直接控制桌宠。

Supervisor 定期读取 ActivityWatch 最近一段时间的事件。

---

## 5.2 Go Supervisor

这是唯一的系统核心。

它负责：

### 状态管理

```text
UserMode
STANDBY
STUDY
BREAK
OFF
```

以及系统观察：

```text
InteractionState
TaskRelation
PrivacyState
```

### 当前学习任务

例如：

```json
{
  "title": "Go Lab - interface",
  "started_at": "...",
  "mode": "STUDY"
}
```

### 活动判断

综合：

- ActivityWatch；
- 键鼠 AFK；
- 当前窗口；
- 当前网页；
- 屏幕变化；
- OCR；
- AI Classification。

### Privacy Gate

在任何截图或视觉 AI 之前，根据：

```text
app
domain
window metadata
user privacy configuration
```

先得到：

```text
PrivacyState = NORMAL | SENSITIVE
```

如果为 `SENSITIVE`：

```text
禁止请求 Screen Sensor 返回分析图
禁止调用云端视觉模型
只允许保留最小必要元数据
```

### 提醒策略

决定：

- 是否提醒；
- 提醒等级；
- 是否进入 cooldown；
- 桌宠使用什么动画；
- 是否同时弹 Windows 通知。

### 本地数据

只存：

- 状态切换；
- 当前任务；
- 休息记录；
- 偏离事件；
- 提醒事件；
- 分类结果缓存；
- 用户反馈；
- 设置。

不复制 ActivityWatch 的全部原始时间线。

## 5.3 Screen Sensor

职责必须保持非常窄。

第一版固定监听：

```text
127.0.0.1:17322
```

只允许 Supervisor 调用。

最小 API：

```text
GET  /healthz
POST /v1/capture
```

输入示例：

```json
{
  "monitor": "primary",
  "include_analysis_image": false,
  "max_width": 960
}
```

输出示例：

```json
{
  "timestamp": "...",
  "monitor": 1,
  "changed": true,
  "hash": "...",
  "analysis_image": "...optional..."
}
```

Screen Sensor 不负责：

- 学习规则；
- 休息规则；
- `TaskRelation`；
- 桌宠状态；
- 长期数据库；
- 用户任务；
- 调用 ActivityWatch；
- 决定是否允许捕获敏感窗口。

隐私判断由 Supervisor 的 Privacy Gate 在发出 capture request 前完成。

Sensor 自身仍需要做 Fail Safe：如果捕获 API 失败、Secure Desktop/UAC 无法截图或 Windows 不允许捕获，返回明确错误，不尝试绕过。

## 5.4 Study Pet

桌宠是 UI，不是大脑。

### V1 允许保留 / 开发

```text
透明置顶窗口
Sprite / GIF / 图集动画
拖动
托盘
气泡
快捷菜单
Supervisor API client
状态展示
用户反馈 UI
后期开机启动集成
```

### V1 必须删除、禁用或不得接入

如果上游桌宠已经包含以下能力，不允许因为“现成”而继续复用为业务核心：

```text
Own work tracker
Own distraction detector
Keyboard hook / keyboard activity business logic
Own SQLite business database
Own task database
Ollama / LLM direct calls
Smart nudge engine
Memory system
Foreground activity classification
Device-monitor based supervision logic
```

这些能力必须由 Supervisor 统一负责，或第一版完全不做。

Pet 只负责：

- 展示；
- 点击；
- 动画；
- 气泡；
- 简单菜单；
- 调用 Supervisor；
- 收集用户反馈。

Supervisor 挂掉时，桌宠可以显示：

```text
监督服务未连接
```

但不能自己复制一套监督逻辑。

# 6. 用户状态机

## 6.1 STANDBY

电脑已启动，今天没有进入正式学习阶段。

系统仍然记录电脑活动。

重点判断：

> 用户今天电脑已经活跃很久，但始终没有开始学习。

推荐初始规则：

```text
累计电脑 Active 时间 >= 60 分钟
AND
当天 STUDY 时长 == 0
→ 第一次提醒
```

后续提醒：

```text
每 30 分钟最多提醒一次
```

必须使用：

> **Active Time**

而不是单纯从电脑开机时间计算。

如果电脑开着但人在外面，不应不断提醒。

---

## 6.2 STUDY

用户主动点击：

```text
开始学习
```

可选输入：

```text
当前任务
```

例如：

```text
RepoLens：完成 Worker 状态机
```

进入严格监督。

---

## 6.3 BREAK

用户主动点击：

```text
开始休息
```

BREAK 时：

- 不判断娱乐是否合理；
- 不因为 Steam / Bilibili / 小说进行惩罚；
- 只关注休息是否过长。

推荐初始阈值：

```text
20 分钟：轻提醒
30 分钟：明显提醒
45 分钟：再次提醒
```

桌宠可提供：

```text
继续学习
延长休息 10 分钟
```

---

## 6.4 OFF

表示：

> 今天已经结束学习监督。

OFF 后：

- 不发督学提醒；
- 可以保留基础统计；
- 不主动进行 AI 分类；
- Screen Sensor 停止或大幅降频。

同一天重启电脑时，应保留 OFF，避免晚上关机重启后又开始提醒。

但是允许用户主动：

```text
OFF → STUDY
```

这样用户结束今天以后，如果临时又决定学习，不需要等待第二天。

## 6.5 UserMode 状态转移表

允许的用户主动转移：

```text
STANDBY → STUDY / OFF
STUDY   → BREAK / OFF
BREAK   → STUDY / OFF
OFF     → STUDY
```

不推荐隐式自动进入 `STUDY` 或 `BREAK`。

原则：

> 用户主动操作优先于系统推断。

系统可以提醒用户“是否开始学习/休息”，但不能擅自把娱乐行为切换成 `BREAK`。

## 6.6 跨自然日规则

按 Windows 当前本地时区计算自然日。

跨到新的本地日期后：

```text
OFF     → STANDBY
STUDY   → STANDBY
BREAK   → STANDBY
STANDBY → STANDBY
```

不允许因为程序持续运行而把前一天的 Study/Break Session 无限制延续到下一天。

跨日发生时：

1. 关闭前一日未结束 session；
2. 写入明确的 `daily_reset` 原因；
3. 新日进入 `STANDBY`；
4. 保留当前任务文本可作为 UI 建议，但不自动恢复 `STUDY`。

## 6.7 时间语义与 Clock

核心状态机、提醒与测试禁止到处直接调用 `time.Now()`。

Supervisor 应定义可替换时钟，例如：

```go
type Clock interface {
    Now() time.Time
}
```

生产使用 `RealClock`，测试使用 `FakeClock`。

所有以下功能必须基于该 Clock：

```text
Break timeout
Reminder cooldown
Standby active time
Distraction accumulation
Daily reset
Session duration
Recovery time
```

Windows Sleep / Hibernate / Lock 的暂停语义见“Windows 特殊情况”。

# 7. 系统观察模型

系统观察结果由三个维度组成，而不是单一枚举。

## 7.1 InteractionState

```text
ACTIVE
IDLE_STATIC
IDLE_DYNAMIC
UNKNOWN
```

### ACTIVE

近期存在有效键鼠活动，或 ActivityWatch 明确显示用户活跃。

### IDLE_STATIC

满足：

```text
长时间无键鼠
+
屏幕基本没有变化
```

代表：

> 学习链条很可能已经中断。

系统不需要知道用户是不是在玩手机，只需要知道学习状态长时间没有产生电脑交互。

### IDLE_DYNAMIC

满足：

```text
长时间无键鼠
+
屏幕持续变化
```

可能是：

- 课程视频；
- 娱乐视频；
- 自动播放；
- 会议。

因此必须继续结合 `TaskRelation`。

### UNKNOWN

当前证据不足、数据源故障或尚未完成采样。

不能仅因为 `InteractionState=UNKNOWN` 发偏离提醒。

---

## 7.2 TaskRelation

```text
FOCUSED
DISTRACTED
UNKNOWN
```

### FOCUSED

系统认为当前内容与学习任务相关。

例如：

```text
VS Code + Go 代码
ChatGPT + Go 问题
pkg.go.dev
GitHub + 当前项目
Bilibili + Go 教程
```

### DISTRACTED

系统高置信度认为正在进行与当前学习无关的娱乐。

例如：

```text
Steam 游戏
小说页面
游戏直播
明显的娱乐短视频
```

不能因为：

```text
Bilibili
ChatGPT
知乎
浏览器
```

就直接判定 `DISTRACTED`。

这些属于混合用途应用。

### UNKNOWN

当前语义证据不足。

不应强行提醒。

必要时才进入 OCR / AI fallback。

---

## 7.3 PrivacyState

```text
NORMAL
SENSITIVE
```

### NORMAL

允许按当前策略继续进行屏幕变化检测、OCR 或视觉 AI。

### SENSITIVE

当前窗口属于敏感应用或用户配置的隐私范围。

例如默认：

- 密码管理器；
- 银行 / 支付；
- 系统认证；
- 明确用户配置的隐私应用或域名。

行为：

```text
禁止截图
禁止 OCR
禁止云端视觉模型
仅保留必要元数据
```

`SENSITIVE` 不是 AI 分类结果。

必须在请求截图之前由本地 Privacy Gate 决定。

# 8. 学习判断链路

Supervisor 每个判断周期执行：

```text
1. 当前 UserMode 是什么？
        ↓
2. 处理跨日 / Lock / Sleep 等时间语义
        ↓
3. OFF / BREAK 是否需要语义监督？
        ↓
4. 获取 ActivityWatch 最近有界时间范围数据
        ↓
5. 得到当前 app / title / URL / AFK
        ↓
6. Privacy Gate → PrivacyState
        ↓
7. 本地元数据规则能否得到 TaskRelation？
        ↓
8. 是否需要 InteractionState 的屏幕变化证据？
        ↓
9. PrivacyState == SENSITIVE ?
   ├─ YES → 禁止截图；使用现有元数据继续或 UNKNOWN
   └─ NO
        ↓
10. 必要时调用 Screen Sensor
        ↓
11. 得到 ACTIVE / IDLE_STATIC / IDLE_DYNAMIC / UNKNOWN
        ↓
12. OCR / 标题 / 域名能否进一步判断 TaskRelation？
        ↓
13. 仍然不确定且确有必要？
        ↓
14. 调用 AI Classifier
        ↓
15. 得到 TaskRelation + confidence
        ↓
16. Reminder Engine 基于 UserMode + Observation 决策
```

关键约束：

```text
InteractionState ≠ TaskRelation
PrivacyState ≠ TaskRelation
```

例如：

```text
IDLE_DYNAMIC + FOCUSED
```

是完全合法且常见的状态。

# 9. STUDY 状态的初始规则

## 9.1 明确娱乐

例如：

```text
DISTRACTED 连续 >= 8 分钟
```

提醒：

```text
你已经偏离当前任务 8 分钟。
当前任务：Go Lab - interface
```

持续到 15 分钟：

```text
你已经偏离学习任务 15 分钟。
先回到当前任务，再决定是否进入休息。
```

不要每分钟重复提醒。

---

## 9.2 无输入 + 屏幕不动

初始建议：

```text
10 分钟：记录，不提醒
20 分钟：轻提醒
30 分钟：明显提醒
```

如果 `TaskRelation=FOCUSED`，且窗口/页面属于阅读、PDF、文档等长阅读场景，可以适当延长阈值。

---

## 9.3 无输入 + 屏幕变化

此时 `InteractionState=IDLE_DYNAMIC`。

随后独立判断 `TaskRelation`：

```text
课程视频 → TaskRelation = FOCUSED
娱乐视频 → TaskRelation = DISTRACTED
证据不足 → TaskRelation = UNKNOWN
```

不要把 `IDLE_DYNAMIC` 覆盖成 `FOCUSED` 或 `DISTRACTED`；两个维度应同时保留。

---

# 10. AI Classification 设计

AI 的主要职责是判断：

```text
TaskRelation
```

AI 不负责：

```text
UserMode
PrivacyState
Break timeout
Reminder cooldown
```

这些都由确定性的本地状态机或规则负责。

AI 不应该直接返回自然语言长篇分析。

必须强制结构化输出：

```json
{
  "relation": "FOCUSED",
  "confidence": 0.91,
  "activity": "watching Go interface tutorial",
  "task_related": true,
  "reason_short": "Go tutorial related to current task"
}
```

允许值：

```text
FOCUSED
DISTRACTED
UNKNOWN
```

输入尽量最小化：

```text
当前任务
app
window title
domain / URL
最近状态
必要时 OCR
必要时低分辨率截图
```

如果 `PrivacyState=SENSITIVE`：

```text
禁止把截图 / OCR 内容交给 AI
```

元数据是否允许进入模型也应服从隐私配置。

## 10.1 Provider 抽象

Supervisor 内部定义独立 Provider，例如：

```text
TaskRelationProvider
VisionProvider
```

以后可以替换：

- 云端文本/视觉模型；
- 本地 Ollama 多模态模型；
- 其他兼容接口。

不能让业务代码直接绑定某一家模型。

## 10.2 AI 调用缓存

对以下组合做缓存：

```text
app
title
domain
screen hash
current task
```

如果没有变化，不重复调用。

缓存只能作为性能优化。

任务变化、隐私状态变化或关键规则变化后必须允许缓存失效。

# 11. 桌宠设计

## 11.1 桌宠基础操作

建议：

### 左键单击

打开小型快捷菜单：

```text
当前：学习中

[ 开始学习 ]
[ 开始休息 ]
[ 结束今天 ]
[ 修改当前任务 ]
```

不要把“单击桌宠”直接定义成状态切换。

原因：

> 容易误触。

---

## 11.2 桌宠动画状态

建议至少包含：

### STANDBY

普通待机。

---

### STUDY + TaskRelation=FOCUSED

播放：

```text
看书
敲键盘
做笔记
```

这是项目非常重要的正反馈。

---

### STUDY + TaskRelation=DISTRACTED

桌宠：

```text
停止学习动画
皱眉 / 敲桌子
弹气泡
```

---

### BREAK

```text
喝水
睡觉
伸懒腰
躺着
```

---

### BREAK_TOO_LONG

桌宠主动起身提醒。

---

### ERROR

Supervisor / ActivityWatch 失联时显示错误状态。

---

# 12. 用户反馈闭环

误判不可避免。

每次关键提醒最好允许：

```text
[ 我其实在学习 ]
[ 确实跑偏了 ]
[ 开始休息 ]
```

这不是装饰功能。

V1 只记录反馈，不自动修改规则权重，避免在真实使用初期形成不可解释的“在线学习”。

后续稳定版本可以基于明确、可回滚的规则学习机制积累用户自己的分类偏好。

例如：

```text
用户多次把某个 Bilibili 标题判成学习
```

以后可以提高相似页面的学习概率。

---

# 13. 提醒策略

系统最大的产品风险不是“提醒太少”，而是：

> **提醒太多以后用户直接关闭软件。**

因此必须内建 Cooldown。

例如：

```text
同一问题 10 分钟内不重复通知
```

提醒等级：

```text
L1：桌宠动作
L2：桌宠气泡
L3：Windows Toast
```

第一版：

```text
只做到桌宠气泡 + Windows 本机通知
```

不做手机推送。

---

# 14. 数据与隐私

## 14.1 禁止 Keylogger

绝对不记录：

```text
用户具体敲了什么字
```

只允许使用：

```text
是否有输入
输入活动时间
ActivityWatch AFK 状态
```

---

## 14.2 截图默认不永久保存

推荐：

```text
capture
↓
hash / classify
↓
删除原图
```

只有 Debug Mode 可以临时保留。

Debug Mode 默认关闭。

---

## 14.3 Cloud Vision

如果未来使用云端视觉模型：

默认必须：

1. 排除敏感应用；
2. 截图降分辨率；
3. 能裁剪就不发送全屏；
4. 明确配置 Provider；
5. 不上传历史截图；
6. 不长期保留云端请求数据。

---

## 14.4 Localhost 安全

Supervisor API：

```text
仅监听 127.0.0.1
```

禁止：

```text
0.0.0.0
```

建议使用：

```text
Bearer Token
```

Token 首次运行生成，保存在：

```text
D:\StudyGuardianDev\config\auth.token
```

Pet 和 Sensor 使用同一 Token。

---

# 15. 本地 API 与 IPC

所有 StudyGuardian 自有 HTTP 服务只监听 localhost。

## 15.1 Supervisor

固定默认地址：

```text
127.0.0.1:17321
```

实际端口允许配置，但 V1 文档、脚本和测试默认使用 17321。

### GET /healthz

用于 Windows E2E / Deploy health check。

### GET /v1/status

返回示例：

```json
{
  "user_mode": "STUDY",
  "interaction_state": "ACTIVE",
  "task_relation": "FOCUSED",
  "privacy_state": "NORMAL",
  "confidence": 0.92,
  "task": "Go Lab - interface",
  "study_seconds": 3200,
  "break_seconds": 0,
  "last_activity_at": "...",
  "activitywatch_ok": true,
  "screen_sensor_ok": true
}
```

### POST /v1/mode/study

```json
{
  "task": "Go Lab - interface"
}
```

### POST /v1/mode/break

进入休息。

### POST /v1/mode/off

结束今天。

### POST /v1/task

修改任务。

### POST /v1/feedback

例如：

```json
{
  "event_id": "...",
  "feedback": "ACTUALLY_STUDYING"
}
```

### GET /v1/events/stream

后期可增加 SSE。

Pet 接收：

```text
mode change
observation change
notification
animation
health
```

V1 可以先每 1～2 秒轮询 `/v1/status`，避免过早增加复杂度。

---

## 15.2 Screen Sensor

固定默认地址：

```text
127.0.0.1:17322
```

仅供 Supervisor 调用。

### GET /healthz

### POST /v1/capture

只进行一次按需采集 / diff / 可选分析图生成。

Sensor 不提供学习业务 API。

---

## 15.3 认证

Supervisor 与 Sensor 都只监听：

```text
127.0.0.1
```

V1 使用首次运行生成的 Bearer Token。

Token 放在：

```text
D:\StudyGuardianDev\config\
```

Pet 和 Sensor 获得各自所需的最小访问凭据。

禁止日志记录 Authorization Header。

# 16. Supervisor 数据库

SQLite。

Go 推荐优先选择：

```text
modernc.org/sqlite
```

原因：

> CGo-free。

这样 WSL 可以更简单地：

```bash
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build
```

生成 Windows 可执行文件。

建议表：

```text
daily_state
sessions
tasks
observations
distraction_events
reminders
feedback
classification_cache
settings
```

数据库必须有 schema migration/version 机制。

不要复制 ActivityWatch 原始 events。

---

# 17. 开发环境

## 17.1 总原则

```text
WSL = Source + Build + Test + AI Coding
Windows = Runtime + GUI + System Integration + E2E
```

---

## 17.2 WSL

继续使用现有环境。

至少包含：

```text
Git
Go
Codex CLI
Python（只用于开发工具，不代表 Windows Python）
shell 工具
```

源码放：

```text
~/projects/study-guardian
```

不建议把正式源码放到 `/mnt/d/StudyGuardianDev` 这类 Windows 挂载目录；正式源码保留在 WSL 文件系统。

---

## 17.3 Windows

只安装运行所需环境。

第一阶段：

```text
ActivityWatch
Python 3.11+
桌宠依赖
Screen Sensor 依赖
```

Windows Go 初期可以不装。

Supervisor 在 WSL 交叉编译。

---

## 17.4 Windows 运行与持久数据目录

开发阶段所有 StudyGuardian Windows 侧内容统一放在 D 盘：

```text
D:\StudyGuardianDev\
├── bin\
├── pet\
├── sensor\
├── config\
│   ├── config.yaml
│   └── auth.token
├── data\
│   └── studyguardian.db
├── logs\
│   ├── supervisor.log
│   ├── pet.log
│   └── screen-sensor.log
├── run\
└── handoff\
```

对应 WSL：

```text
/mnt/d/StudyGuardianDev/
```

其中：

```text
bin / pet / sensor
= Windows 运行与部署内容

config / data / logs / run
= 持久数据与运行状态

handoff
= AI / E2E / 审计交接文件，可重建但不得被无意清空
```

核心原则：

> **deploy 脚本不得把 `D:\StudyGuardianDev` 根目录整棵删除或整体覆盖。**

只允许更新明确的部署目标文件；必须保留：

```text
D:\StudyGuardianDev\config
D:\StudyGuardianDev\data
D:\StudyGuardianDev\logs
D:\StudyGuardianDev\run
D:\StudyGuardianDev\handoff
```

桌宠 / Sensor 的 Windows `.venv` 如果已经存在且依赖满足，不应无理由重复创建或删除；只有依赖确实变化时才更新。

WSL 正式源码仍放：

```text
~/projects/study-guardian
```

不要因为 Windows 运行目录在 D 盘，就把正式源码改成 `/mnt/d/StudyGuardianDev`。

# 18. WSL → Windows 开发循环

Build 与 Deploy 必须分离。

## 18.1 Build

WSL 内先运行：

```bash
go test ./...

CGO_ENABLED=0 \
GOOS=windows \
GOARCH=amd64 \
go build \
  -o ./dist/windows/study-supervisor.exe \
  ./cmd/supervisor
```

生成物先留在 WSL repo：

```text
~/projects/study-guardian/dist/windows/
```

不要让 `go build` 直接覆盖：

```text
D:\StudyGuardianDev\bin\study-supervisor.exe
```

因为 Windows 正在运行的 exe 可能被锁定。

## 18.2 Deploy

统一通过：

```text
scripts/deploy-windows.sh
```

执行原则：

```text
1. 构建 / 确认 dist artifact
2. 请求旧 Supervisor 正常退出，必要时停止旧进程
3. 停止需要替换的 Pet / Sensor 进程
4. 只更新 D:\StudyGuardianDev 下明确的部署文件
5. 绝不整棵删除 D:\StudyGuardianDev
6. 保留 config / data / logs / run / handoff
7. 已存在且可用的 Windows venv 不无理由重建
8. 启动 Supervisor
9. 启动 Pet / Sensor（当前阶段需要时）
10. GET /healthz
11. 执行 Windows smoke test
```

部署安全检查必须证明：

```text
重复 deploy 多次
→ D:\StudyGuardianDev\data\studyguardian.db 不丢失
→ D:\StudyGuardianDev\config 不丢失
→ 日志目录仍存在
→ auth.token 不被重置
```

## 18.3 开发期启动

需要单独启动时可以通过 WSL 调 Windows：

```bash
powershell.exe -NoProfile -Command \
  "Start-Process 'D:\StudyGuardianDev\bin\study-supervisor.exe'"
```

但正式开发循环仍以 deploy script 为准。

# 19. Python 桌宠 / Screen Sensor 开发方式

源码真源仍然在 WSL：

```text
~/projects/study-guardian
```

运行前：

```text
deploy to D:\StudyGuardianDev
```

当前已经验证可用的 Windows Python / Pet venv / Sensor venv 不重复初始化；仅在依赖变化时更新。

Windows 使用 Windows Python。

不要使用 WSL Python 运行：

```text
PyQt
pywin32
Windows screenshot
```

---

# 20. AI 主导开发执行模式

本项目采用：

> **WSL Codex CLI 单主开发通道 + Windows 真实运行环境 + 开发完成后的独立 Codex 审计。**

目标是减少人工在两个 AI 之间频繁来回审查的成本。

## 20.1 WSL Codex CLI —— 主开发者

负责：

- 阅读本开发文档与仓库现状；
- 创建 / 修改正式源码；
- 写测试；
- Refactor；
- Git commit / push；
- Go 编译；
- Python Pet / Sensor 逻辑；
- API；
- 数据库；
- 文档；
- Windows build / deploy 脚本；
- 从 WSL 调用 Windows 命令完成可自动化的 smoke test；
- 维护 `TASKS.md`；
- 生成中文开发汇报。

### 连续执行原则

本轮允许 Codex 连续向后开发：

```text
读取文档
↓
检查现有环境与 repo
↓
补齐剩余 Phase 0
↓
Phase 1
↓
Phase 2
↓
Phase 3...
↓
达到当前可实现的 V1 范围
```

不需要每完成一个小任务就等待人工确认。

只有遇到以下情况才应停止并请求用户处理：

- 必须人工点击且无法通过现有 Windows 环境完成；
- 缺少账号 / API Key / 权限；
- 远端 Git 权限失败且无法自行恢复；
- 需要用户在两个不可兼容的产品方案之间做真实取舍；
- 继续执行可能破坏已有用户数据；
- 文档存在无法通过代码和现有环境自行判定的硬冲突。

普通报错、测试失败、依赖问题、路径问题、编译错误应优先自行诊断修复，不应立即把问题抛给用户。

## 20.2 Windows 真实运行环境

Windows 是真实 Runtime / GUI / System Integration / E2E 环境。

Codex 可以通过 WSL 调用：

```text
powershell.exe
cmd.exe
Windows Python
Windows exe
localhost API
ActivityWatch API
```

当前已人工验证通过的环境不要重复折腾：

```text
[✅] ActivityWatch Window / AFK
[✅] Browser Watcher / 浏览器数据（当前环境可用）
[✅] WSL Go → Windows EXE
[✅] Desktop Pet 原版 Windows smoke test
[✅] Windows Python 3.11
[✅] Screen Sensor venv + mss
[✅] python-mss 实际截图，图片正常
```

Codex 开工后首先补齐：

```text
[ ] Pet → Supervisor → Sensor localhost 通信 PoC
[ ] Deploy 不破坏 D 盘持久数据的安全检查
```

## 20.3 Windows 桌面端 AI 的角色

Windows 桌面端 AI 可以在需要时用于：

- 真实 GUI 点击；
- DPI / 多显示器肉眼验收；
- 托盘 / Toast / 动画验收；
- 收集 Windows-only 错误；
- 最终 E2E 辅助。

但默认不作为并行开发者，不维护第二份源码。

## 20.4 AI 汇报语言

**所有面向用户的 AI 开发汇报必须使用中文。**

包括：

```text
阶段进度
做了什么
测试结果
失败原因
未完成项
风险
Git 提交摘要
最终开发汇报
最终审计报告
```

允许保持英文的内容：

```text
代码
变量 / 类型 / API 名
命令
日志原文
Conventional Commit message
第三方项目原名
```

不要因为代码是英文，就把整份开发汇报写成英文。

## 20.5 开发完成后的独立审计

主开发结束后，应启动**新的 Codex 审计轮次 / 独立上下文**，重新读取：

```text
开发文档
AGENTS.md
TASKS.md
Git history
核心源码
测试
Windows 部署脚本
日志 / E2E 结果
```

审计时不要假设主开发实现正确，也不要只看最后一个 commit。

审计至少覆盖：

- 架构边界是否被破坏；
- Pet / Sensor 是否偷塞业务逻辑；
- 三维观察模型是否保持分离；
- ActivityWatch bucket 是否动态发现；
- localhost / Token / Privacy Gate 是否正确；
- D 盘路径是否统一；
- 是否还有 C 盘 StudyGuardian 路径残留；
- Build / Deploy 是否安全；
- 状态机 / Clock / 跨日 / Sleep / Lock 测试；
- AI fallback；
- 截图默认不永久保存；
- Windows E2E；
- Git 是否小步提交而不是一个巨型 commit；
- TODO / FIXME / dead code / 临时 mock 是否残留；
- V1 未完成项和技术债。

输出：

```text
docs/FINAL_AUDIT.md
```

并向用户提供一份中文审计结论。

# 21. Source of Truth

唯一正式源码：

```text
WSL:
~/projects/study-guardian
```

Windows：

```text
D:\StudyGuardianDev
```

只作为：

> Runtime / Deployment / Persistent Local Data Root

其中 `config / data / logs / run` 是 Windows 侧持久目录，但它们不是源码。

禁止：

> Windows runtime 改了一个 Bug，但 WSL 源码没有同步。

任何代码修复最终都必须落回 WSL repo 并进入 Git history。

# 22. 推荐代码仓库结构

为了降低 AI 多仓库协作复杂度，正式项目推荐使用：

> **单一主仓库 / Monorepo**

```text
study-guardian/
│
├── AGENTS.md
├── README.md
├── DEVELOPMENT.md
├── TASKS.md
│
├── cmd/
│   └── supervisor/
│
├── internal/
│   ├── activitywatch/
│   ├── state/
│   ├── observer/
│   ├── rules/
│   ├── reminder/
│   ├── classifier/
│   ├── storage/
│   ├── api/
│   └── platform/
│       └── windows/
│
├── pet/
│   ├── upstream/
│   ├── src/
│   ├── assets/
│   └── tests/
│
├── sensor/
│   ├── screen/
│   └── tests/
│
├── dist/
│   └── windows/                 # gitignored build artifacts
│
├── scripts/
│   ├── build-windows.sh
│   ├── deploy-windows.sh
│   ├── test-windows.ps1
│   └── bootstrap-windows.ps1
│
├── configs/
│   └── default.yaml
│
├── docs/
│   ├── ARCHITECTURE.md
│   ├── PRIVACY.md
│   ├── TEST_PLAN.md
│   ├── OPEN_SOURCE.md
│   └── FINAL_AUDIT.md
│
└── third_party/
    └── licenses/
```

---

# 23. 为什么推荐 Monorepo

原方案是：

```text
Supervisor 一个 repo
Pet 一个 fork
Screenshot 一个 fork
```

复核以后，不推荐作为第一版。

原因：

1. 纯 AI 开发时跨三个 Git 仓库上下文成本高；
2. API 改动需要多个仓库同步；
3. Windows Agent / WSL CLI 更容易操作错版本；
4. 项目主要是个人自用，没有必要过早做微服务式仓库治理。

因此：

> ActivityWatch 保持外部依赖。

桌宠代码：

> 允许从 MIT 开源项目引入基线代码，但必须保留 LICENSE 和 UPSTREAM 信息。

Screen Sensor：

> 主要依赖成熟库，不需要 Fork 一个不确定的 Screenshot Watcher。

---

# 24. Open Source 记录

必须创建：

```text
docs/OPEN_SOURCE.md
```

记录：

```text
项目
版本 / commit
许可证
使用方式
是否修改
上游仓库
资产许可证
```

特别注意：

> 桌宠“代码许可证”和“角色图片 / 动画资产许可证”可能不同。

即使个人自用，也应该保留来源。

如果未来公开发布，更必须重新审查素材版权。

---

# 25. Windows 开机启动

最终至少有：

```text
ActivityWatch
Study Supervisor
Study Pet
Screen Sensor
```

ActivityWatch 使用其官方 Windows 自启动。

V1：

StudyGuardian 组件可以分别使用 Startup Shortcut。

最终版本建议：

```text
Supervisor 作为主后台进程
```

由它启动：

```text
Pet
Screen Sensor
```

这样可以：

- 检测子进程崩溃；
- 自动重启；
- 减少 Startup 项目数量；
- 统一日志。

但此功能放在后期，不进入第一阶段。

---

# 26. 日志

每个模块独立日志，统一写入：

```text
D:\StudyGuardianDev\logs\
supervisor.log
pet.log
screen-sensor.log
```

使用 Rotating Logs。

禁止日志记录：

- API Key；
- 完整截图；
- 用户键盘内容；
- 敏感窗口内容；
- Authorization Header。

---

# 27. 健康检查

Supervisor：

```text
GET /healthz
```

检测：

```text
ActivityWatch
Database
Screen Sensor
AI Provider

Pet 是否在线可以作为附加状态，但不能让 Supervisor 的自身 `/healthz` 因 Pet 未连接而失败
```

桌宠可以显示：

```text
🟢 正常
🟡 AI 不可用
🔴 ActivityWatch 失联
```

AI 不可用不能导致整个系统变红。

---

# 28. 初始配置建议

```yaml
standby:
  first_study_active_minutes: 60
  repeat_reminder_minutes: 30

study:
  distraction_warn_minutes: 8
  distraction_strong_minutes: 15
  idle_static_warn_minutes: 20
  idle_static_strong_minutes: 30

break:
  warn_minutes: 20
  strong_minutes: 30
  repeat_minutes: 15

reminder:
  cooldown_minutes: 10

screen:
  enabled: true
  store_raw: false
  active_sample_seconds: 15
  unknown_sample_seconds: 5
  break_sample_seconds: 60

ipc:
  supervisor_host: 127.0.0.1
  supervisor_port: 17321
  sensor_host: 127.0.0.1
  sensor_port: 17322

privacy:
  sensitive_apps: []
  sensitive_domains: []

ai:
  enabled: true
  use_vision_only_when_needed: true
  min_confidence: 0.75
```

所有阈值后续根据真实使用调整。

---

# 29. 性能目标

正常后台：

```text
CPU 平均尽量 < 2%
```

Screen Sensor 不应：

```text
每秒截图
每张图调用 AI
永久保存全屏图片
```

内存没有严格极限，但整个 StudyGuardian 自有组件应尽量保持轻量。

---

# 30. 开发阶段

## Phase 0：环境与技术链 PoC

目标：

> 只证明关键技术链在当前真实 Windows 环境成立，不在这一阶段提前实现正式监督业务。

### 当前已完成（不要重复安装 / 重做）

根据 2026-09-02 当前环境实测：

```text
[✅] ActivityWatch Windows 正常采集 Window / AFK
[✅] Browser Watcher / 浏览器数据当前可用
[✅] WSL Go 可以交叉编译并在 Windows 运行
[✅] Desktop Pet 原版在 Windows 成功启动
[✅] Windows Python 3.11 可用
[✅] Screen Sensor 独立 venv 可用
[✅] mss 已安装
[✅] python-mss 可以正常识别显示器并截图
[✅] 实际 screen-poc.png 内容正常
```

### Codex 开工后先完成的剩余 PoC

1. 确认 repo / `AGENTS.md` / 基础目录 / scripts 骨架；
2. 写最小 Supervisor，仅提供 `/healthz` 与最小测试端点；
3. 写最小 Sensor `127.0.0.1:17322` hello / capture stub；
4. 写最小 Pet Supervisor client；
5. 验证：

```text
Pet
↓ HTTP localhost
Supervisor :17321
↓ HTTP localhost
Sensor :17322
```

6. 至少覆盖以下失败情况：

```text
Supervisor 未启动 → Pet 不崩溃，显示 disconnected
Sensor 未启动 → Supervisor /healthz 可表达 degraded
错误 Token → 返回 401/403，不 crash
Sensor timeout → Supervisor fail soft
```

7. 实现最小 `build-windows.sh` / `deploy-windows.sh`；
8. 连续执行至少两次 deploy，验证 D 盘持久目录不被删除或重置；
9. 全部通过后打 Git tag：

```text
phase0-passed
```

### Phase 0 明确不做

```text
正式 ActivityWatch Adapter
正式状态机业务
正式 Reminder Engine
正式 Screen diff
AI Classification
复杂 Pet 动画改造
```

### Phase 0 DoD

```text
[✅] ActivityWatch Windows 正常采集窗口 / AFK
[✅] WSL Go 可以交叉编译并在 Windows 运行
[✅] Pet 底座可以在当前 Windows 环境运行
[✅] python-mss 可以正常捕获当前 Windows 屏幕
[ ] Pet / Sensor / Supervisor 三者可以通过 localhost 通信
[ ] Build → Deploy 不覆盖 D:\StudyGuardianDev\config / data / logs / run / handoff
```

并最终证明：

```text
WSL Source
→ Build Artifact
→ Deploy 到 D:\StudyGuardianDev
→ Windows 真实运行
→ localhost health / smoke test
→ 重复 deploy 不损坏持久数据
```

完成后技术路线冻结，Codex**无需等待人工审查即可继续 Phase 1**；只有遇到第 20.1 节定义的硬阻塞才停止。

## Phase 1：无 ActivityWatch 业务依赖、无 AI 的 Supervisor MVP

目标：

> 先把 Supervisor 的确定性核心做正确。

实现：

```text
Clock abstraction
SQLite migrations
UserMode state machine
STANDBY / STUDY / BREAK / OFF
跨日规则
Supervisor API
Pet UI Shell 基础操作
Reminder Engine
Break timeout
Cooldown
```

同时定义但只使用 Fake：

```text
ActivitySource interface
FakeActivitySource
ScreenSource interface
FakeScreenSource
```

Pet：

```text
开始学习
开始休息
结束今天
修改当前任务
显示状态
```

提醒：

```text
休息太长
手动状态相关提醒
```

不要正式读取 ActivityWatch。

不要截图。

不要 AI。

### DoD

```text
go test ./... PASS
Windows E2E PASS
可连续使用一天
跨日 / restart / cooldown 基础行为正确
```

---

## Phase 2：ActivityWatch 监督

实现正式：

```text
ActivityWatchSource
bucket discovery
bounded query
window / title / URL when available / AFK
Active Time
基础 TaskRelation rules
```

Browser Watcher 数据存在时使用；不存在时优雅降级。

实现：

```text
电脑 Active Time
STUDY idle metadata detection
基础娱乐规则
STANDBY 迟迟未开始学习提醒
```

### DoD

可以识别：

```text
学习中持续 Steam
长时间 AFK
上午电脑用了很久却没开始学习
ActivityWatch restart / missing bucket 时不 crash
```

---

## Phase 3：Screen Sensor

正式加入：

```text
python-mss
screen hash
screen changed / unchanged
Privacy Gate before capture
```

实现 `InteractionState`：

```text
ACTIVE
IDLE_STATIC
IDLE_DYNAMIC
UNKNOWN
```

### DoD

可以区分：

```text
20 分钟没人操作 + 屏幕不动
```

和：

```text
20 分钟没人操作 + 视频持续播放
```

敏感应用不会触发截图。

---

## Phase 4：AI Classification

增加：

```text
task-aware TaskRelation classification
optional OCR / Vision
structured output
provider abstraction
cache
```

重点解决：

```text
Bilibili 是教程还是娱乐？
ChatGPT 是学习还是聊天？
知乎是在查资料还是刷内容？
```

### DoD

```text
AI 失败时系统仍然可用
SENSITIVE 状态绝不向 Vision 发送截图
TaskRelation 与 InteractionState 不互相覆盖
```

---

## Phase 5：桌宠体验

完善：

- 学习动画；
- 休息动画；
- 跑偏动画；
- 气泡；
- 一键反馈；
- 小型当前任务展示；
- 状态健康灯。

仍然禁止把监督逻辑重新塞回 Pet。

---

## Phase 6：稳定化

目标：

```text
连续运行 7 天
```

处理：

- crash；
- suspend/resume；
- Windows lock；
- ActivityWatch 重启；
- 网络断开；
- AI timeout；
- 多显示器变化；
- DPI；
- 开机启动；
- 日志轮转；
- Runtime redeploy 不损坏 `D:\StudyGuardianDev` 数据。

---

## Phase 7：Release

个人自用安装包。

最终目标：

```text
Windows 开机
↓
ActivityWatch
↓
Supervisor
↓
Pet + Sensor
↓
自动恢复配置 / 当日状态
```

Release 安装/升级不得覆盖用户持久数据。

# 31. 测试策略

## 31.1 Go Unit Tests

必须重点覆盖：

```text
State Machine
Clock / time advance
UserMode transition table
Reminder Cooldown
Break Timeout
Standby Active Time
Distraction accumulation
Daily reset
Classification fallback
InteractionState / TaskRelation independence
Privacy Gate before capture
```

---

## 31.2 Fake ActivityWatch

测试时不要依赖真实 ActivityWatch。

实现：

```text
ActivitySource interface
```

生产：

```text
ActivityWatchSource
```

测试：

```text
FakeActivitySource
```

AI 可以构造：

```text
VS Code 20 min
Steam 15 min
AFK 30 min
```

测试规则。

---

## 31.3 Fake Screen Sensor

同理：

```text
ScreenSource interface
```

支持：

```text
changed
unchanged
capture unavailable
```

---

## 31.4 AI Classifier Tests

使用固定输入 / 固定输出。

不要单元测试时真的调用云端 API。

---

## 31.5 Windows E2E

由 Windows Desktop Agent 执行。

测试矩阵：

```text
开始学习
开始休息
休息超时
结束今天
ActivityWatch 离线
Supervisor 重启
Pet 重启
Screen Sensor 重启
电脑锁屏
睡眠恢复
双显示器
浏览器切页
VS Code
ChatGPT
Bilibili 教程
Bilibili 娱乐
Steam
长时间不碰电脑
```

---

# 32. Windows 特殊情况

必须明确处理：

## Lock Screen

锁屏期间暂停监督计时，不计入：

```text
STUDY idle / distraction duration
BREAK timeout
STANDBY active time
Reminder cooldown 的“用户活跃时间”统计
```

解锁后以新的活动样本重新建立观察，不把锁屏前后的空档拼成一段偏离。

---

## Sleep / Hibernate

挂起时间不能被当成：

```text
休息了 5 小时
AFK 了 5 小时
偏离了 5 小时
```

恢复后重新建立计时基线。

状态机使用 `Clock`，平台层负责向 Supervisor 暴露 suspend/resume 事件或检测异常 wall-clock gap。

---

## Fullscreen Game

如果 ActivityWatch / Screen Capture 无法获得正常数据：

```text
降级到进程 / 窗口规则
```

---

## UAC Secure Desktop

禁止尝试绕过。

无法截图属于正常情况。

---

# 33. 错误处理

系统必须 Fail Soft。

例如：

### ActivityWatch 挂了

```text
继续手动 STUDY / BREAK
提醒用户 ActivityWatch unavailable
```

### Screen Sensor 挂了

```text
退化成 ActivityWatch-only
```

### AI 挂了

```text
退化成本地规则
```

### Pet 挂了

```text
Supervisor 继续记录
Windows Toast 仍可提醒
```

---

# 34. 晚间复盘系统

晚间复盘是第二个独立项目。

当前不开发。

StudyGuardian 只需要提前保留结构化数据。

未来 Review Plugin 可以读取：

```text
Study sessions
Break sessions
Distraction events
Recovery time
Reminder events
Current task history
ActivityWatch summary
```

再结合：

```text
当天 GPT 学习对话
```

生成晚间复盘。

两个系统：

```text
实时监督系统
≠
晚间复盘系统
```

禁止为了未来复盘，把当前实时系统做得过重。

---

# 35. 未来复盘接口

Supervisor 后期提供：

```text
GET /v1/review/daily?date=YYYY-MM-DD
```

输出：

```json
{
  "study_seconds": 0,
  "break_seconds": 0,
  "first_study_at": "",
  "sessions": [],
  "distractions": [],
  "average_recovery_seconds": 0,
  "reminders": []
}
```

这样复盘插件不需要直接访问 Supervisor SQLite。

---

# 36. 核心指标

不要只看：

```text
今天学习了几小时
```

重点统计：

## Study Session Completion

计划学习块完成多少。

---

## Unplanned Distraction Starts

学习状态下主动启动娱乐多少次。

---

## Recovery Time

从：

```text
DISTRACTED
```

恢复：

```text
FOCUSED
```

用了多久。

长期目标：

```text
43 min
↓
25 min
↓
12 min
↓
8 min
```

---

## First Study Start

电脑开始活动以后，到第一次 STUDY 的时间。

这个指标与“上午是否被浪费”非常相关。

---

# 37. AI 开发规则：AGENTS.md 必须写入

项目根目录必须加入：

```text
AGENTS.md
```

至少写入以下硬规则：

```text
1. 这是 Windows-first 项目。
2. WSL 是主开发环境，Windows 是真实运行环境。
3. Source of Truth 只在 WSL repo：~/projects/study-guardian。
4. Windows StudyGuardian 根目录固定为 D:\StudyGuardianDev；禁止新增 C:\StudyGuardianDev 或 %LOCALAPPDATA%\StudyGuardian 作为项目路径。
5. 不修改 ActivityWatch 主项目。
6. 开源优先，不重复造基础设施。
7. 不新增技术栈，除非有明确理由。
8. Supervisor 必须保持核心唯一性。
9. Pet 不允许包含监督业务逻辑。
10. Screen Sensor 不允许包含业务规则。
11. 不记录键盘具体内容。
12. 截图默认不永久保存。
13. 所有 API 只监听 localhost。
14. AI 不可用时基础功能必须继续运行。
15. 修改核心规则必须增加测试。
16. 每个独立功能必须有 Acceptance Criteria 或可验证完成条件。
17. Windows E2E 未验证的 Windows-only 功能不得声称完全完成。
18. 不在 D:\StudyGuardianDev 中维护正式源码。
19. 禁止重新引入单一 ObservedState；InteractionState / TaskRelation / PrivacyState 必须分离。
20. Pet 禁止实现监督规则、直接调用 LLM 或维护业务 SQLite。
21. Sensor 禁止实现 TaskRelation 或 Reminder 逻辑。
22. Build 与 Deploy 必须分离；不得直接覆盖正在运行的 Windows exe。
23. deploy 不得整棵删除 D:\StudyGuardianDev；必须保留 config / data / logs / run / handoff。
24. ActivityWatch bucket 必须动态发现，不得硬编码机器名相关 bucket ID。
25. 核心时间逻辑必须通过 Clock 抽象测试。
26. Privacy Gate 必须发生在截图之前。
27. 当前已通过的 ActivityWatch / Go EXE / Pet / mss PoC 不重复安装或推翻，除非实测证明已损坏。
28. 开工后先补 Pet → Supervisor → Sensor localhost 通信 PoC。
29. Git 必须小步多次 commit；一个独立能力/修复完成且测试通过后就提交。
30. 配置了可写远端时，每次成功 commit 后都 push 当前工作分支；禁止最后一次性 push 巨型提交。
31. push 失败不得丢弃本地 commit；记录原因后继续可安全进行的开发，最终中文汇报说明。
32. 禁止把 secret / token / API key / .env / 数据库 / 用户日志提交到 Git。
33. 面向用户的所有开发汇报、阶段总结、失败说明、最终汇报、审计报告一律使用中文。
34. 不因为需要多次 commit/push 就频繁等待用户确认；除硬阻塞外连续推进开发。
35. 开发结束必须保持工作树清晰，列出未提交改动、未完成项和技术债。
```

# 38. AI 执行任务格式

本轮用户会把整份开发文档直接交给 WSL Codex CLI，允许其连续开发，因此不再要求用户人工逐个发送每个 Task。

但 Codex 内部仍必须把工作拆成小任务，并写入 / 更新：

```text
TASKS.md
```

每个内部任务至少应有：

```text
Goal
Constraints
Acceptance Criteria
Status
Related Commit
```

示例：

```markdown
# Task: ActivityWatch Adapter V0

## Goal
Supervisor 可以读取最近 5 分钟 ActivityWatch Window / AFK 数据。

## Constraints
- 不修改 ActivityWatch
- 只访问 localhost
- 必须有 interface
- 必须支持 fake implementation

## Acceptance Criteria
- go test ./... PASS
- 可以打印当前 app/title
- ActivityWatch 离线时不会 crash
```

原则：

> **整份文档可以一次交给 Codex，但实现仍然必须拆成多个小任务、小测试、小 commit、小 push。**

禁止把“用户没有逐个审查”理解成“可以一次改几千行最后再测”。

# 39. Git 工作流：小步、多次 commit、多次 push

这是本项目本轮开发的硬要求。

## 39.1 基本原则

每完成一个**可独立说明、可独立测试、可独立回滚**的能力，就执行：

```text
实现
↓
针对性测试
↓
查看 git diff
↓
commit
↓
push
↓
继续下一小块
```

不要等待一个 Phase 全做完才提交。

典型应拆开的提交包括：

```text
chore(repo): initialize project skeleton
feat(ipc): add supervisor health endpoint
feat(sensor): add localhost sensor stub
feat(pet): add supervisor connectivity client
test(ipc): cover disconnected and timeout cases
feat(state): add user mode state machine
feat(storage): add sqlite migrations
feat(activitywatch): add bucket discovery
feat(sensor): add screen change detection
feat(rules): add task relation rules
fix(reminder): ignore sleep duration
```

## 39.2 Push 规则

如果仓库已经配置可写远端：

> **每次成功 commit 后尽快 push 当前工作分支。**

用户已经授权本轮开发进行多次 Git push，不需要每次 push 再单独询问。

但是：

- 不得擅自创建或更换陌生远端；
- 不得 force push，除非用户明确要求；
- 不得重写已经 push 的公共历史；
- 不得提交 Token、API Key、数据库、用户日志、截图等敏感文件；
- push 因网络 / 权限失败时保留本地 commit，并在中文汇报中说明。

## 39.3 提交质量

每次 commit 前至少做到：

```text
相关测试 PASS
无明显 debug 临时代码
无 secret
git diff 已检查
commit message 能准确描述改动
```

不允许：

```text
“final”
“all changes”
“big update”
```

这种无法审计的大提交作为主要开发历史。

## 39.4 阶段性汇报

Codex 不需要因为 commit/push 而停止等待用户，但可以在长任务中输出简短中文进度。

最终至少汇报：

```text
完成了哪些 Phase / 功能
总测试情况
Windows 实测情况
关键 commit 列表
push 情况
仍未完成的功能
已知风险 / 技术债
建议下一步审计重点
```

# 40. 第一轮实际开发顺序

Codex 按以下依赖顺序连续推进。**每完成一个独立能力就测试、commit、push，不等待用户逐项审查。**

## 40.1 Phase 0：补齐剩余 PoC

当前不要重复做已经人工 PASS 的 ActivityWatch / Go EXE / Pet / mss 环境验证。

```text
01. 读取文档与现有环境，确认 D:\StudyGuardianDev 当前目录
02. 创建 / 整理 WSL repo、AGENTS.md、TASKS.md、.gitignore、scripts 骨架
03. Supervisor 最小 :17321 /healthz
04. Sensor 最小 :17322 /healthz + capture stub
05. Pet 最小 Supervisor client
06. 验证 Pet → Supervisor → Sensor localhost 通信
07. 覆盖 disconnected / invalid token / timeout fail-soft 测试
08. build-windows.sh
09. deploy-windows.sh
10. 连续两次 deploy，验证 D:\StudyGuardianDev\config / data / logs / run / handoff 不被破坏
11. Phase 0 自动测试 / Windows smoke test
12. commit + push 各独立小块
13. 打 tag: phase0-passed，并 push tag（远端可用时）
```

完成后**直接继续 Phase 1**。

## 40.2 Phase 1：确定性核心

```text
14. Clock abstraction
15. SQLite migration skeleton
16. UserMode state machine + transition tests
17. 跨日 / restart / sleep / lock 时间规则
18. Supervisor /v1/status 与 mode API
19. FakeActivitySource / FakeScreenSource
20. Pet 精简为 UI Shell
21. Pet ↔ Supervisor 正式 API
22. Reminder Engine + cooldown
23. BREAK timeout
24. 一轮 Windows E2E
```

这些能力应拆成多个 commit / push，禁止一个 Phase 1 巨型 commit。

## 40.3 Phase 2：ActivityWatch 正式监督

```text
25. ActivityWatch bucket discovery
26. ActivityWatchSource real adapter
27. Window / title / URL / AFK bounded query
28. Active Time
29. 基础 TaskRelation rule engine
30. ActivityWatch offline fail-soft
31. Unit + integration tests
32. Windows smoke / E2E
```

## 40.4 Phase 3：Screen Sensor 正式接入

```text
33. mss 正式 capture implementation
34. monitor / virtual desktop handling
35. pHash / dHash / screen change detection
36. Privacy Gate before capture
37. InteractionState: ACTIVE / IDLE_STATIC / IDLE_DYNAMIC / UNKNOWN
38. Sensor timeout / crash fallback
39. 自动测试 + Windows 实测
```

## 40.5 Phase 4：AI Classification

```text
40. TaskRelationProvider interface
41. VisionProvider interface
42. 结构化 JSON schema
43. local rules first / AI fallback
44. classification cache
45. timeout / invalid response fallback
46. 敏感窗口禁止 Vision
47. provider unavailable 仍可运行
```

没有可用 API Key 时，不阻塞整个项目：完成 Provider 抽象、Fake Provider、fallback 和测试，把真实云模型接入标记为待配置。

## 40.6 Phase 5～Release

```text
48. Pet 动画 / 气泡 / feedback UX
49. Windows Toast
50. startup / resume / crash recovery
51. logs rotation
52. 7-day stability test 所需工具与 checklist
53. release packaging skeleton
54. 文档整理
55. 最终测试
56. 最终中文开发汇报
57. 为独立 Codex 审计准备 docs/FINAL_AUDIT.md 模板与 audit handoff 信息
```

原则：

> AI Vision、动画等后置能力不得反过来阻塞确定性核心完成。

# 41. 第一版明确不做

避免范围失控。

V1 不做：

- 手机监控；
- 手机推送；
- 强制锁软件；
- 浏览器插件自己重写；
- Keylogger；
- 声音录制；
- 摄像头；
- 麦克风；
- 长期全量截图；
- 人脸识别；
- 自动读 GPT 网页；
- 晚间复盘；
- 云端账户；
- 多用户；
- Web Dashboard；
- Docker；
- Kubernetes；
- 分布式；
- 复杂 Agent Framework；
- 用户反馈驱动的在线自动规则学习。

---

# 42. 主要风险与解决方案

## 风险 1：AI 误判

解决：

```text
Rules first
AI fallback
Confidence
User feedback
Cooldown
```

---

## 风险 2：截图侵犯隐私

解决：

```text
Sensitive Apps
Ephemeral screenshots
Local processing first
Cloud opt-in
```

---

## 风险 3：桌宠项目太小

解决：

> Pet 永远只做 UI，底座可替换。

---

## 风险 4：Screenshot Watcher Windows 不稳定

解决：

> 使用 `python-mss` 作为稳定底层。

---

## 风险 5：ActivityWatch API 变化

解决：

```text
ActivitySource interface
ActivityWatchAdapter
```

---

## 风险 6：WSL / Windows 双环境混乱

解决：

```text
WSL = Source
Windows = Deploy
```

禁止双向编辑。

---

## 风险 7：AI 一次开发过多导致 Bug 堆积

解决：

```text
小任务
Acceptance Criteria
自动测试
Windows E2E
```

---

## 风险 8：提醒太烦最后被关闭

解决：

```text
Cooldown
层级提醒
BREAK 合法化
OFF
用户反馈
```

---


---

## 风险 9：观察状态维度重新被混合

解决：

```text
InteractionState
TaskRelation
PrivacyState
```

保持正交，任何模块不得重新引入一个同时表达三者的万能枚举。

---

## 风险 10：开发部署覆盖用户数据

解决：

```text
D:\StudyGuardianDev\bin / pet / sensor = replaceable runtime
D:\StudyGuardianDev\config / data / logs / run = persistent data
D:\StudyGuardianDev 根目录不可整棵覆盖
Build != Deploy
```

# 43. 技术栈最终建议

## Core

```text
Go
net/http
SQLite
modernc.org/sqlite
```

---

## Pet

```text
Python 3.11+
PyQt6
Windows APIs
```

从现有 MIT Windows Desktop Pet 项目改造。

---

## Screen

```text
Python 3.11+
python-mss
Pillow / perceptual hash
Optional OCR
```

---

## Activity

```text
ActivityWatch Windows
ActivityWatch Browser Watcher
REST API
```

---

## AI

```text
Provider abstraction
Text / Vision model
Structured JSON output
```

---

## Runtime

```text
Windows 10 / 11
```

---

## Development

```text
WSL2 Ubuntu
Codex CLI
Git
Go
```

---

## Windows E2E

```text
Windows Desktop AI Agent
```

---

# 44. 最终结论

经过重新检查后，推荐方案不是：

```text
从零写一个 Windows 监控软件
```

也不是：

```text
直接魔改 ActivityWatch
```

更不是：

```text
让一个桌宠项目承担所有功能
```

正式方案应该是：

```text
ActivityWatch
负责“看到电脑行为”

python-mss Screen Sensor
负责“看到屏幕变化”

Go Supervisor
负责“理解 UserMode + InteractionState + TaskRelation + PrivacyState、规则、AI、提醒”

Desktop Pet
负责“用户交互、动画和低干扰反馈”

Windows Desktop AI
负责“真实 Windows 环境操作和 E2E”

WSL Codex CLI
负责“主要代码开发”
```

整个项目中真正需要我们原创并长期维护的核心只有：

> **Supervisor 的监督逻辑。**

其他部分都应该保持可替换。

这样才能同时满足：

- 开源优先；
- AI 全权开发；
- Windows 原生运行；
- WSL 开发体验；
- 后续容易维护；
- 低耦合；
- 低开发成本；
- 真正适合每天长期使用。

---

# 45. 当前开工技术门状态

截至 2026-09-02，以下基础环境已人工验证：

```text
[✅] ActivityWatch Windows 正常采集窗口 / AFK
[✅] WSL Go 可以交叉编译并在 Windows 运行
[✅] Pet 底座可以在当前 Windows 环境启动
[✅] python-mss 可以正常捕获 Windows 屏幕，实际图片正常
```

Codex 开工后必须优先补齐：

```text
[ ] Pet / Sensor / Supervisor 三者通过 localhost 通信
[ ] Build → Deploy 不破坏 D:\StudyGuardianDev\config / data / logs / run / handoff
```

补齐后打：

```text
phase0-passed
```

然后无需等待人工再次审查，直接继续正式开发。

如果某项 FAIL：

> 优先只修复或替换该模块底座，不推翻整个架构。


# 46. 直接交给 WSL Codex CLI 时的执行指令

用户可以直接把本开发文档交给 Codex，并附一句：

> **“按此文档继续开发 StudyGuardian。先读取现有环境和仓库，不重复已经 PASS 的环境 PoC；先补 localhost 三组件通信与 D 盘 deploy 安全检查，然后连续向后开发。除硬阻塞外不要等待我逐项确认。务必小步多次 commit、多次 push；所有面向我的开发汇报用中文。开发结束后给出完整中文汇报，我会再启动一次独立 Codex 全量审计。”**

Codex 收到后应：

```text
1. 先读完整文档
2. 检查现有 repo / git status / remote / 当前分支
3. 检查 D:\StudyGuardianDev 当前环境，不破坏已有 venv / ActivityWatch / Pet / mss
4. 建立或更新 TASKS.md
5. 从 Section 40 当前未完成项开始
6. 小步测试 → commit → push → 继续
7. 维护 .gitignore，确保用户数据与 secret 不入库
8. 尽可能自行修复普通错误
9. 最终输出中文开发汇报
10. 不把“开发完成”冒充“独立审计完成”
```

---

## 参考开源项目（2026-09-02 已复核）

- `ActivityWatch/activitywatch` — MPL-2.0，Windows / macOS / Linux，活动采集核心。
- `UIU-Developers-Hub/desktop-pet` — MIT，Windows + Python + PyQt6，作为桌宠 UI 首选候选。
- `BoboTiG/python-mss` — MIT，纯 Python、支持 Windows，多显示器截图。
- `InertialG/aw-watcher-screenshot` — MIT，截图 + perceptual hash + ActivityWatch，可作为实现参考和 Windows 候选。
- `Srakai/aw-watcher-screenshot` — ActivityWatch screenshot watcher，但当前仓库明确只实测 macOS，不作为默认底座。
- `kepptic/aw-watcher-enhanced` — MPL-2.0，提供 Windows 安装方式，OCR / diff / LLM 分类思路可参考。
- `modernc.org/sqlite` — CGo-free SQLite Go driver，适合 WSL → Windows 交叉编译。


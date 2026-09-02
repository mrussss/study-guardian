# StudyGuardian 架构设计文档

## 1. 总体架构与职责解耦

StudyGuardian 采用模块化解耦的单机分布式协作架构，各组件职责划分如下：

```text
┌─────────────────────────────────────────────────────────┐
│                       Windows 11                        │
│                                                         │
│  ┌───────────────────────────────────────────────────┐  │
│  │                   Study Pet                       │  │
│  │        PyQt6 / Desktop Pet UI Shell               │  │
│  │                                                   │  │
│  │ • 纯 UI 交互、置顶透明窗口、动画反馈              │  │
│  │ • 模式切换 (STUDY / BREAK / OFF)                  │  │
│  │ • 气泡提醒与用户反馈闭环                         │  │
│  └───────────────────────┬───────────────────────────┘  │
│                          │ HTTP localhost:17321         │
│                          ▼                              │
│  ┌───────────────────────────────────────────────────┐  │
│  │             Go Supervisor :17321                  │  │
│  │                                                   │  │
│  │ • 唯一核心状态机 (UserMode State Machine)         │  │
│  │ • 三维观察模型 (Observation Engine)               │  │
│  │ • 本地隐私门禁 (Privacy Gate)                     │  │
│  │ • 本地规则优先引擎 (Rule Engine)                  │  │
│  │ • AI 分类服务与缓存 (AI Classifier + Cache)        │  │
│  │ • 提醒策略与冷却引擎 (Reminder Engine + Cooldown) │  │
│  │ • CGO-Free SQLite 状态与事件存储                  │  │
│  └──────────────┬───────────────────────┬────────────┘  │
│                 │ REST                  │ HTTP localhost:17322
│                 ▼                       ▼               │
│  ┌──────────────────────┐   ┌────────────────────────┐  │
│  │    ActivityWatch     │   │ Screen Sensor :17322   │  │
│  │    (Port: 5600)      │   │                        │  │
│  │                      │   │ • python-mss           │  │
│  │ • Window / Title     │   │ • pHash / dHash 差异比对│ │
│  │ • URL (Web Watcher)  │   │ • 纯按需捕获，不持久化  │ │
│  │ • AFK 键鼠状态       │   │ • 敏感窗口截图阻断      │ │
│  └──────────────────────┘   └────────────────────────┘  │
│                                                         │
└─────────────────────────────────────────────────────────┘
```

## 2. 核心架构约束

1. **三维观察正交模型**：
   - `InteractionState`：`ACTIVE` / `IDLE_STATIC` / `IDLE_DYNAMIC` / `UNKNOWN`
   - `TaskRelation`：`FOCUSED` / `DISTRACTED` / `UNKNOWN`
   - `PrivacyState`：`NORMAL` / `SENSITIVE`
   严禁将三者合并为单一枚举，确保如 `IDLE_DYNAMIC + FOCUSED`（如观看课程视频）等状态能精准表达。

2. **用户意图与系统观察分离**：
   - 用户主动声明模式 `UserMode` (`STANDBY`, `STUDY`, `BREAK`, `OFF`)。
   - 系统仅在 `STUDY` 模式下执行强语义督学；在 `BREAK` 下仅做时长提醒；在 `OFF` 下完全停止提醒。

3. **隐私门禁优先 (Privacy Gate First)**：
   - 本地规则在触发任何截图或 AI 调用前，先比对敏感应用与敏感域名。
   - 若 `PrivacyState == SENSITIVE`，严禁触发截图、OCR 及云端 AI。

4. **规则优先，AI 兜底 (Rules First, AI Fallback)**：
   - 确定性黑白名单与任务关键词匹配优先判断。
   - 模糊歧义场景调用结构化 AI 分类并进行多维哈希缓存（TTL 10分钟）。

# Pet 输入架构决策与 Windows 验证记录

日期：2026-09-04，Asia/Shanghai。状态：**暂保留 Tauri 候选；完整物理压力 Gate 为 PENDING，尚未批准正式透明 Pet 的稳定性验收。**

本轮已把“单击后辅助窗口创建卡死”和“透明 Pet 拖动是否长期稳定”分开验证。已有证据支持修复同步 WebView 创建、阻塞网络请求及路由监听权限；这些修复不等于透明窗口的 50 次拖动已通过。现代 Quick Panel / Control Center / Settings 继续保留，完整 Settings、托盘和发布验收不在本记录中宣称完成。

## 源码与环境

正式源码唯一来源为 WSL Ubuntu-22.04 `/home/lls/projects/study-guardian`。调查开始时 HEAD 为 `350d88a17e1b481b0f0e48937823ea1514c70668`，已有 6 个未提交实验文件；它们已保存在本次会话 `work/preexisting-wsl-changes.patch`。没有通过重置到远端丢弃这些实验。以下候选包含本地修改，不能只用 HEAD 代替实际构建身份。

| 项目 | 本机实测值 |
| --- | --- |
| Windows | Windows 11，10.0.26200 |
| 本次系统启动时间 | 2026-09-04 21:51:21 +08:00 |
| WebView2 Runtime | 152.0.4191.62 |
| Windows cargo / rustc | 1.98.1 / 1.98.1；MSVC Windows 构建链 |
| WSL Node / npm | 22.22.1 / 10.9.4；Linux NVM 路径 |
| 已解析 Rust 依赖 | tauri 2.11.5，wry 0.55.1，tao 0.35.3 |
| 候选 Cargo.lock SHA256 | `3ED3C75106D81155036D55D60CCFFAE6A5491801B63C8AFFDC670D8FBC8884A1` |

Windows 编译在临时构建副本进行，正式运行目录 `D:\StudyGuardianDev` 不用作源代码工作区。A/B 使用相同 Rust 源码哈希 `346103D872B66095810E92607F5CE29E447CFF9602B4C68E8AB37DEAEF2EE531`、相同 Cargo.lock 与前端主 bundle 哈希 `2220BD655FAC0EA679AE49133E8C7E3EC152D914026A736F01C2D426E1BF7564`。窗口配置在 Rust 编译时嵌入；运行已编译 exe 后改变 `TAURI_CONFIG` 不会切换 A/B。

## 可复现的故障层与修复

| 问题 | 本地证据与置信度 | 本轮处理 |
| --- | --- | --- |
| 辅助 WebView 在同步 IPC 回调中创建 | 新编译旧逻辑的 PID 29808 第一次点击有 `quick-panel:open-command`，没有完成创建/显示日志；改为异步工作线程后的不透明候选 PID 5116 出现 created、show-ok、focus-ok，页面命令完成。结合官方明确的 Windows 同步创建死锁限制，对该入口故障为高置信定位。没有据此断言所有历史拖动故障都是同一原因。 | `open_quick_panel` / `open_control_center` 使用异步入口，完整辅助窗口操作在阻塞工作线程队列中串行处理；复用固定窗口 label，路由更新不在持有状态锁时调用窗口 API。 |
| Supervisor 同步 TCP 请求阻塞 UI | 原命令内使用同步 socket 连接、读写；Dashboard 一次请求还可能顺序访问多个端点。静态路径能确认阻塞风险，尚未独立量化其对历史卡顿的贡献。 | 快照、Dashboard、模式和每日目标命令通过 `spawn_blocking` 执行网络工作，保留原有超时和脱敏结果。 |
| 手动拖动固定 2 秒清状态 | 接手的手动 `setPosition` 实验从开始拖动计时，2000 ms 后直接清除仍在进行的手势；这是可从代码确认的长拖截断条件。以开始时间计算 500 ms 屏蔽也不能覆盖任意长拖后的 click。 | A/B 完全移除此类手势实现；真实 Pet 随后也移除旧手势栈、计时器及极小 alpha 命中层，改用原生 CSS 拖动与明确的“学习面板”按钮。真实 Pet 压力验收仍为 PENDING。 |
| Control Center 现有窗口路由监听不完整 | Rust 会更新/发送路由，但 control-center 原先没有所需 event listen/unlisten capability；React 首次查询与事件订阅还存在先后竞争。权限缺失和时序风险可由源码确认；页面切换必须另作实际观察。 | 增加仅限 control-center 的 listen/unlisten capability；先订阅事件再读当前路由，避免较旧查询覆盖较新事件；每次请求增加 revision，修复设置→内部历史→再次请求设置时 React 同值优化导致不切页。 |

Tauri 2.11.5 的 `WebviewWindowBuilder::new` / `from_config` 官方说明明确要求 Windows 同步命令/事件处理程序中的窗口创建改为异步命令或独立线程。微软也说明 WebView2 回调依赖 UI 线程消息循环，不支持回调内的嵌套重入。这里的结论是应用触发了已知调用约束，而不是泛称“Tauri 有问题”。[Tauri 2.11.5 API](https://docs.rs/tauri/2.11.5/tauri/webview/struct.WebviewWindowBuilder.html#method.from_config)；[WebView2 线程模型](https://learn.microsoft.com/en-us/microsoft-edge/webview2/concepts/threading-model#reentrancy)。

## A/B/C 最小实验

入口在 `pet-v3/diagnostics/input.html`，完整运行方式见 [诊断 README](../pet-v3/diagnostics/README.md)。A/B 都是 220 × 220、无边框、不可缩放、不可最大化、置顶、无阴影窗口，共用同一 HTML/JS；只切换 `main.transparent` 和辨识标题。页面不加载 Pet 动画、Supervisor 轮询或业务状态机。

拖动区域仅使用 `app-region: drag` / `-webkit-app-region: drag`，按钮使用 `no-drag`。没有 `startDragging()`、Pointer Capture、鼠标补偿、手动位置更新或手势计时器；透明实验没有加入极小 alpha 命中补丁。CSS 原生拖动区域承担标题栏语义，不能假定它同时提供普通 DOM click；产品点击入口应使用明确的 `no-drag` 热点，待 C 实测后确认。

| 实验 | 窗口与输入路径 | 已取得的证据 | 完整压力 Gate |
| --- | --- | --- | --- |
| A | 不透明最小页；原生 CSS 非客户区拖动 | 2 次真实窗口位移：原点 `(320,320) → (388,363) → (319,331)`；20 次点击计数为 20；辅助窗口创建/显示/聚焦命令完成。 | **PENDING**：尚未记录 50 次连续拖动与 10 次交替循环；20 次计数不代替全部 Gate。 |
| B | 同一最小页，真正透明背景；同一原生 CSS 路径 | PID 43916 的 2 次真实窗口位移：原点 `(160,160) → (228,203) → (159,171)`；20 次点击计数为 20。 | **PENDING**：完整 50 次拖动和 10 次交替循环未完成。 |
| C | 真实 Pet DOM/皮肤；原生 CSS 拖动与独立点击热点 | 已移除旧手势栈，Pet 画布使用 CSS 原生拖动，底部显式“学习面板”按钮使用 no-drag；PID 20900 已连接现有 Supervisor；实际 Pet 原点 `(288,288) → (357,351) → (288,289)`，重复拖动后底部按钮可打开带真实状态的 Quick Panel。 | **PENDING** |

完整 Gate 对每一候选分别要求：50 次连续拖动、20 次连续点击、10 次点击/拖动交替循环；无增长性延迟、无卡住、无丢输入、测试中无进程重启。应包含超过 3 秒的长拖、失焦后起拖、释放后立即点击，以及透明边缘与 125%/150% DPI 检查。上表中的真实桌面输入与窗口坐标证明了基础原生路径；未执行的物理压力观察仍保持 PENDING。

若 A 通过而 B 失败，才进一步定位透明合成/命中边界；若 A/B 均通过而 C 失败，优先定位真实 Pet DOM 与交互集成；若 A/B 都失败，则检查原生拖动路径或测试环境。任何一格都不能仅凭编译成功、DOM 事件、IPC Promise 返回或 `start-ok` 标记 PASS。

## 候选构建与进程身份

会话工作目录为 `C:\Users\Lenovo\Documents\Codex\2026-09-04\github-plugin-github-openai-curated-remote\work`。以下路径均相对此目录；它们是本机证据位置，不是仓库内发布产物。构建证明保存 exe 哈希、最后修改时间、Rust 源码、Cargo.lock 和前端 bundle 哈希；运行证明单独记录 PID、实际路径和启动时间。

| 候选 | 实际 exe / PID | exe SHA256 | exe LastWriteTime（+08:00） |
| --- | --- | --- | --- |
| 新编译旧逻辑基线 | `baseline-exe/studyguardian-pet-v3.exe`；PID 29808 | `31568247F2ECF1088C9F06154E8582DFB699B422726FA45A1126B2F72A3D1113` | 2026-09-04 22:06:04 |
| A 不透明 | `candidate-opaque/studyguardian-pet-v3.exe`；PID 5116 | `D25BF45CF7249F9FDF6D9DDA2FDC9DD079450414A427690A641CA9762F81FA3B` | 2026-09-04 22:09:06.2500472 |
| B 透明 | `candidate-transparent/studyguardian-pet-v3.exe`；PID 43916 | `3348A7BC3168A1843D35352BA97C038BCA3AB9A687F5B42A49E3AA84D7B61C96` | 2026-09-04 22:10:48.9510271 |

A/B 的完整记录分别为 `candidate-opaque/build-proof.json`、`candidate-opaque/runtime-proof.json`、`candidate-transparent/build-proof.json`、`candidate-transparent/runtime-proof.json`。PID 是本次运行历史，不保证阅读本文时仍在运行；任何重编译或重启必须刷新证明，不能沿用旧 PID 的 Gate 结论。

后续生命周期候选使用更新的不透明 exe，SHA256 为 `BD0C9F9D66AB69642890F7A46A76141AF5089F53A8EB020D038F7FDAAE47D439`，PID 46524。真实 Pet 候选在 22:20:02 编译，SHA256 为 `24594FCA8B16C14E1771B7496DEE6D36E41B0F25B17B0677C579CB7D45CA8EE0`，PID 20900 在 22:20:59 启动，使用现有本地 Supervisor/Sensor/AW；后续面板布局修复与重编译仍需刷新身份。会话中的 build-proof 文件可能随候选重建更新，读取时必须按 SHA256 匹配本段对应实验，不混用初次 A/B 与后续生命周期数据。

## Quick Panel 与路由的独立验收

诊断页可以独立调用打开/隐藏 Quick Panel 及 Control Center 的 settings/review 路由，无需先依赖 Pet 点击。自动 20 轮生命周期检查每轮都读取原生 `visible=true/false`，不以 IPC 完成猜测窗口显示。诊断命令有超时和 debug 构建约束。

| 检查 | 当前记录 |
| --- | --- |
| 创建、显示、聚焦与命令返回 | A 候选已观察完成 |
| 打开/原生可见/显式隐藏/原生隐藏 20 轮 | **PASS**：更新的不透明候选 PID 46524 实际运行诊断序列显示 20/20；日志 created 1、open-command 20、show-ok 20、focus-ok 20、explicit hide 20，过程中没有重启。证据在 `work/runtime-lifecycle`。 |
| Escape 隐藏及手动重复入口 | **PASS（基础操作）**：真实 Pet 点击打开后发送 Escape，窗口列表中面板消失，原生日志记录 explicit hide；完整连续点击压力仍待人工。 |
| settings/review 首次及现有实例路由 | **PASS（基础路由）**：PID 46524 的设置页及复盘页已通过真实窗口截图/可访问性标题核验。overview、重复同路由回到目标页仍待最终候选复测。 |
| 控制中心复用、不出现重复窗口 | **PASS（设置→复盘）**：同一 Control Center 窗口切页。 |

保留显式打开/隐藏、Escape 隐藏、打开 Control Center 时隐藏 Quick Panel 的策略。没有恢复无条件 `Focused(false) -> hide()`；失焦行为不能继续掩盖创建或显示问题。

## 自动化基线与测试隔离修复

| 检查 | 结果与范围 |
| --- | --- |
| `go test ./...` | 17 个包通过，1 个包无测试；工具输出有缓存结果 |
| `go test -race ./...` | exit 0；17 个包通过，1 个包无测试；有缓存结果 |
| `go vet ./...` | exit 0 |
| Pet / Sensor / Integration Python | 6 / 6 / 7 项全部通过，共 19 项 |
| 前端基线 `npm test` | 24/24，通过；这是修改后新测试加入前的基线数量 |
| 前端基线 `npm run build` | TypeScript + Vite 构建通过 |
| `scripts/test-all.sh` | 测试隔离修复后 exit 0，包含 4 个部署安全场景 |
| Rust 候选 check/test/build | check/build 通过，13 项 Rust 测试通过；按生命周期候选记录 |
| 原生 CSS Pet 迁移后的前端回归 | 20/20 通过，TypeScript/Vite 构建通过；移除旧手势的 5 项过时测试，增加重复同路由请求回归。 |

旧 `tests/test_deploy_safety.sh` 硬编码真实 `D:\StudyGuardianDev`，会先写 canary 再尝试部署。初次基线在 WSL 执行 Windows PowerShell 时以 `Exec format error` 停止，尚未停止进程或替换程序；退出前残留的 6 个 canary 与 8 个复制皮肤文件已按精确内容/SHA256 核对后清理，保留生产父目录，没有读取用户配置、令牌或日志内容。

该测试现使用临时目录及 EXIT 清理：复制同一生产部署脚本，只精确适配临时目标白名单；使用最小构建产物及 PowerShell 调用替身。目标拒绝、首次部署、重复部署、health smoke 失败后回滚 4 个场景均通过，持久目录与 Python venv 均和快照逐文件比较。生产部署脚本未改变；这里验证文件系统保护与回滚，不冒充真实 Windows health smoke。证据在本次会话 `work/baseline/summary.md` 和 `work/baseline/test-all-after-isolation.log`。

## 上游证据与推论边界

- 已解析 Wry 0.55.1 源码调用 `SetIsNonClientRegionSupportEnabled(true)`，所以该依赖支持 WebView2 非客户区；这只证明功能可用，不能替代本机透明窗口验收。[Wry 对应版本源码](https://github.com/tauri-apps/wry/blob/wry-v0.55.1/src/webview2/mod.rs#L551)。
- Microsoft 将 `app-region: drag` 视为原生标题栏区域，支持系统拖动等行为；Tauri 官方窗口文档也说明该 CSS 在 Windows 的用途。交互按钮应处于 `no-drag` 区域。[Microsoft 非客户区 API](https://learn.microsoft.com/en-us/microsoft-edge/webview2/reference/win32/icorewebview2settings9)；[Tauri Window Customization](https://tauri.app/learn/window-customization/)。
- Tauri #10767 报告了 Windows 拖动时焦点变化及鼠标事件缺失；维护者在 #14446 说明原生拖动后 WebView 可能收不到 release。它们支撑“不以 DOM release 或固定时间推定原生拖动结束”，但旧版问题报告不能直接证明本机透明合成有缺陷。[Issue #10767](https://github.com/tauri-apps/tauri/issues/10767)；[Discussion #14446](https://github.com/orgs/tauri-apps/discussions/14446)。

当前根因判断：辅助窗口创建入口的同步调用问题为高置信；同步网络阻塞、手动拖动计时和路由权限是已确认代码风险；透明 WebView2 自身是否存在剩余输入缺陷，仍缺完整 A/B/C 压力证据。没有将所有现象归因于单一框架或单一 JavaScript 缺陷。

## 架构选择与转向条件

| 方案 | 收益与代价 | 结论 |
| --- | --- | --- |
| A：Tauri Pet + Tauri/React 面板 | 保留统一 UI 技术栈；原生 CSS 路径减少手势代码。必须验证透明命中、真实皮肤和明确点击入口。 | **暂定候选**；A/B/C 完整 Gate 前不得声明正式采用或稳定通过。 |
| B：PyQt Pet + Tauri/React 面板 | 将透明桌宠输入交给现有 PyQt 壳，保留全部现代面板工作；需要有限本地 IPC 及联合打包。 | **确定的失败回退方向**；若透明原生实验或真实 Pet Gate 失败，停止继续叠加透明 WebView 补丁并转向此方案。 |
| C：原生 Rust/Win32 Pet + Tauri/React 面板 | 可以脱离 WebView2 的 Pet 输入表面，但新增绘制、窗口和输入维护成本。 | 留作后续版本；当前没有证明其实现成本足够低。 |

本轮决策是先保留已经有基础原生输入证据的 Tauri 候选，并修复可独立确认的调用缺陷；这是一项受 Gate 约束的暂定决策。若 A/B 及真实 Pet 50 次拖动、20 次点击、10 次交替循环均通过，再批准正式 Pet 原生 CSS 方案。任一必要稳定性 Gate 失败则采用 Hybrid v1，停止扩展旧 Pointer/Mouse/timer/透明 alpha 组合。

Hybrid 的 PyQt 仅承担宠物绘制、拖动、点击/右击、小状态和提醒气泡；Quick Panel、Control Center、Settings 继续由 Tauri/React 承担。桥接只接受有限本地 UI 命令，不接受任意 URL、shell 执行或秘密参数；Supervisor 保持唯一业务状态机。尚未在本轮实现或验证该备用桥接，不将方案设计称为交付完成。

下一次更新必须补全 A/B 的完整压力 Gate、C 候选、Escape 及真实页面路由结果，并刷新对应构建/进程证明。已完成的 20 轮生命周期检查不代替这些缺项；只有必要证据完成后才能推进完整 Settings/Tray 与发布验收。

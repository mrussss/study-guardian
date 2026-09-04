# StudyGuardian UI 产品架构

## 当前阶段

2026-09-04 完成 Stage B 基础、Stage C native 编排骨架，并开始 Stage D canonical
数据接入：Pet v3 继续保持
轻量 TypeScript/canvas 桌宠；新增独立 `quick-panel.html` 与 `control-center.html`
React 入口，以及 Control Center Dashboard 静态壳。当前 Dashboard 使用受控 fixture，
尚未把它当作完整真实数据验收；native dashboard 已接入 status、motivation、history、
achievements、missions、rewards 和 AI status 的脱敏 DTO。Stage C 的 Windows 原生窗口
视觉/交互仍需人工 Gate。

## 窗口职责

| 窗口 | 目标 | 当前状态 |
| --- | --- | --- |
| Pet (`main`) | 动画、状态/任务短文案、click/drag、提醒、托盘入口 | 已有 native transport；Interaction Fix Pack 自动验证通过，Windows 手动 Gate PENDING |
| Quick Panel (`quick-panel`) | 当前模式、任务、专注进度、Study/Break/Off、学习中心、设置 | React UI、Tauri 延迟创建/单实例/定位/隐藏与 native status/motivation/control 已接通；原生窗口人工 Gate 待完成 |
| Control Center (`control-center`) | 总览、任务、成就、奖励、复盘、历史、设置、诊断 | React Dashboard/sidebar、Tauri 窗口与受限 status/motivation/history/achievements/missions/rewards/AI DTO 已接通；其余 parity 待后续阶段 |

## 边界不变

- Supervisor 是唯一业务状态源；React 不复制 mode/session 状态机。
- Sensor 负责采集与 diff，ActivityWatch 是外部事实源，Pet 只是 UI shell。
- Pet 的 `supervisor_snapshot` / `supervisor_set_mode` 仍通过 Rust loopback
  transport，WebView 不读取 `auth.token`。
- Review evidence exclusion、Collector 和现有 Task 92–100 逻辑不在本阶段重做。
- 最终桌宠艺术资产仍待用户选择；不加入 Chiikawa/吉伊素材。

## 后续顺序

1. 完成 Stage C Windows 原生窗口视觉/交互 Gate（当前自动化和浏览器页面复查已通过）。
2. 完成 Stage D/E：补齐 review、Study Center parity 和真实页面 actions，并保持
   受限 native client 的固定路径/脱敏边界。
3. Stage E/F：完成 Control Center parity，再实现 typed settings API、原子配置
   保存与 secret configured-only DTO。
4. Stage H/I/J：托盘统一、生产清理、全量回归和 Windows 视觉验收。

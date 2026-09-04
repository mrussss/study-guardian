# Pet v3 当前实现与验收

更新于 2026-09-04。正式源码位于 WSL `pet-v3/`；Windows 构建副本只用于编译和验证。

## 本轮输入与窗口修复

- 透明、无边框、置顶的 220px Pet 使用 WebView2 原生 CSS `app-region: drag`。按住猫身体拖动，点击底部“学习面板”打开 Quick Panel。
- 已移除自定义 Pointer Capture、鼠标补偿、`startDragging()`、`setPosition()`、两秒手势计时器和极低 alpha 命中层。原生拖动区域使用标题栏语义，因此打开面板使用独立的 `no-drag` 按钮。
- Quick Panel 和 Control Center 在后台工作线程内创建与操作，避免 Windows 同步 WebView 创建死锁。辅助窗口操作串行执行，不承诺并发请求的 FIFO 顺序。
- Supervisor 网络请求移出 UI 线程，令牌读取、固定 localhost API 和脱敏 DTO 边界继续保留。
- Quick Panel 显式关闭/Escape 隐藏；失焦不自动隐藏。Control Center 固定单实例，支持受限路由与重复相同路由请求。
- 托盘提供快捷面板、控制中心、设置及鼠标穿透恢复入口。
- `Cargo.lock` 跟踪已验证的 tauri 2.11.5 / wry 0.55.1 / tao 0.35.3。

## 验证边界

本轮已在新编译的旧逻辑上重现首次点击卡住；修复后的独立面板 20 次显示/隐藏全部通过，原生状态每次均被核验。实际 A/B 窗口分别有重复位移与 20 次点击计数证据。真实 Pet 的完整物理压力验收仍待用户反馈，不能仅凭编译、命令返回或基础自动输入标记稳定通过。

完整环境、构建身份、测试证据、架构备选与待验收项目见 [输入架构决策](PET_INPUT_ARCH_DECISION.md)。最小复现与独立面板检查见 [诊断说明](../pet-v3/diagnostics/README.md)。

现代 React Quick Panel / Control Center 及现有数据接入保留。完整 Settings、统一生产托盘、安装交付、锁屏/休眠/多显示器 E2E 和七天试用仍未完成；本轮没有把这些事项标成已交付。

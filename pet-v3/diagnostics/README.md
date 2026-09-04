# Windows 原生输入最小诊断页

此页用同一套 220 × 220 HTML 对照不透明窗口 A 与透明窗口 B，分别检查系统拖动、普通 DOM 单击及辅助窗口创建。它不加载 Pet 动画、Supervisor 轮询或业务状态机。

顶部深色区域仅使用 `app-region: drag` / `-webkit-app-region: drag`；下方按钮显式使用 `no-drag`。页面没有 `data-tauri-drag-region`、`startDragging()`、指针捕获、手动窗口移动或手势计时器。计数按钮完全在 DOM 内运行；面板和路由按钮直接调用原生命令，并显示请求序号与返回状态。因此，单击计数成功不能替代辅助窗口成功的验证，CSS 拖动也不以 DOM `mouseup` 为成功依据。

“自动开关面板 20 次”逐次执行打开、原生可见断言、隐藏、原生隐藏断言，全部通过才显示 `20/20`。每个诊断命令有 6 秒超时；超时会显示可能卡死并停止序列，不会宣称通过。诊断过程中原生命令按钮禁用，独立单击计数仍可使用。可见性来自与窗口操作共用序列化队列的 `pet_window_diagnostics`，不是根据 IPC Promise 成功猜测。

“检查设置”和“检查复盘”确认控制中心原生可见及 Rust 路由状态。它们明确不证明 React 已显示对应页面；必须另看实际页面的活动导航和标题。该诊断命令仅在 Rust debug 构建中可用；发布构建拒绝请求。常规构建仍包含静态诊断页，但它不增加发布版原生诊断权限。

## 启动

正式源码位于 WSL `/home/lls/projects/study-guardian`。在 Windows 的临时构建副本中安装 Windows Node/Rust 依赖并运行；不要在 `D:\StudyGuardianDev` 的运行时源码副本直接开发。以下命令的当前目录均为构建副本的 `pet-v3`。

```powershell
# A：不透明原生窗口
npm run tauri -- dev --no-watch --config ./diagnostics/tauri.input-opaque.conf.json
```

退出 A 后，以同一份构建运行 B：

```powershell
# B：透明原生窗口
npm run tauri -- dev --no-watch --config ./diagnostics/tauri.input-transparent.conf.json
```

两个配置的窗口大小、HTML、置顶、无边框、阴影及按钮完全相同；仅 `main.transparent` 和用于识别实验的窗口标题不同。`maximizable: false` 防止 CSS 标题栏双击改变尺寸。配置合并会替换 `app.windows` 数组，因此配置同时保留了 Quick Panel 和 Control Center 的定义；修改正式辅助窗口配置时也应同步这两份诊断配置。[Tauri CLI 配置参数](https://v2.tauri.app/reference/cli/#dev)

需要直接通过 Cargo 构建、不启动 Vite 时：

```powershell
npm run build
$previousDiagnosticConfig = $env:TAURI_CONFIG
try {
    $env:TAURI_CONFIG = Get-Content ./diagnostics/tauri.input-opaque.conf.json -Raw
    cargo build --manifest-path ./src-tauri/Cargo.toml --features custom-protocol
} finally {
    $env:TAURI_CONFIG = $previousDiagnosticConfig
}
# 把已构建 exe 作为诊断程序启动；B 使用 transparent 配置重新构建。
# 如果设置了 CARGO_TARGET_DIR，exe 在该目录，否则在 src-tauri/target/debug/。
```

`TAURI_CONFIG` 必须在 Rust 构建时提供，因为窗口配置会编入 exe。启动已经构建的 exe 时再改变该变量不会切换 A/B。本页也纳入常规 `npm run build` 的 Vite 多页面入口，生产 Pet 的默认入口保持 `index.html`。

## 建议记录

先分别测 A、B 的基础输入，再点击辅助窗口按钮，避免辅助窗口创建卡住后污染基础输入的判定。

| 操作 | 观察与记录 |
| --- | --- |
| 单击计数 50 次 | 初值 0、终值 50；每次真实单击只增加 1 |
| 先点击另一应用，再单击计数 | 第一次单击就增加 1 |
| 拖动顶部区域至少 3 秒 | 原生窗口位置持续变化；松开后停止，不跳回 |
| 连续拖动 10 次，含无焦点起拖 | 无卡住、误点或失去输入 |
| 只拖动顶部、不按计数按钮 | 计数保持不变 |
| 拖动结束后立即单击计数 | 正常增加 1 |
| 打开面板，再点诊断页隐藏面板 | 请求显示已完成，面板真实显示/隐藏；重复操作复用窗口 |
| 自动开关面板 20 次 | 每轮打开后原生 visible=true，隐藏后 visible=false，最终显示 20/20 |
| 检查设置，再检查复盘 | 原生路由通过后，实际页面活动导航及标题分别对应设置/学习复盘 |
| 测试透明边缘及 125%/150% DPI | 记录原生矩形、缩放、实际命中与释放；边缘透明度不由 DOM 计数证明 |

记录 Windows 版本、WebView2 版本、Rust `Cargo.lock`、A/B 模式、exe 构建时间、实际计数及原生位置。浏览器预览、构建通过及 IPC 返回只证明各自阶段，不等于 Windows 原生拖动通过。诊断页中的失败状态不输出未经筛选的原生命令错误；详细定位使用现有本地原生日志。

## 已核实的上游证据

- 2026-09-04 检查既有 Windows 构建副本的 `Cargo.lock`：`tauri 2.11.5`、`wry 0.55.1`、`tao 0.35.3`、`webview2-com 0.38.2`。当时 WSL 源码目录没有 `Cargo.lock`，因此这些是该构建副本的解析版本，不能从 `Cargo.toml` 中的 `version = "2"` 推定以后构建必然相同。
- Wry 0.55.1 的 `src/webview2/mod.rs` 第 551–553 行启用 `ICoreWebView2Settings9::SetIsNonClientRegionSupportEnabled(true)`；此支持最初在 Wry 0.40.0 发布。这确认该解析版本支持 CSS 原生拖动，但不证明本机透明窗口命中已通过。[对应版本源码](https://github.com/tauri-apps/wry/blob/wry-v0.55.1/src/webview2/mod.rs#L551) · [合并记录 #1262](https://github.com/tauri-apps/wry/pull/1262)
- Microsoft 说明 `app-region: drag` 区域按原生标题栏处理，支持窗口拖动、右键系统菜单及双击最大化/还原。页面按钮需要置于 `no-drag` 区域。[WebView2 非客户区 API](https://learn.microsoft.com/en-us/microsoft-edge/webview2/reference/win32/icorewebview2settings9?view=webview2-1.0.2478.35)
- Tauri 官方窗口定制文档明确列出 Windows `app-region: drag` 的用法，适用于无需自定义交互的拖动区域。[Window Customization](https://tauri.app/learn/window-customization/)
- 上游 #10767 报告 Windows 拖动区域导致焦点变化及丢失 `mouseup`；该报告来自较早的 Tauri 版本，不能直接证明本机发生同一故障。2025-11 的维护者答复也说明 `startDragging()` 后 WebView 可能收不到释放事件。因此不能把 DOM release 或任意固定超时作为原生拖动已结束的可靠证据。[问题 #10767](https://github.com/tauri-apps/tauri/issues/10767) · [维护者答复 #14446](https://github.com/orgs/tauri-apps/discussions/14446)

检查原实现时发现两个可从代码确定的手势缺陷：拖动开始后固定 2000 ms 清除状态会截断长拖；以拖动开始时间计算 500 ms 单击屏蔽，会在较长拖动产生后续 `click` 时误开面板。它们与辅助 WebView 创建卡死是不同问题，应通过此页分别验证。

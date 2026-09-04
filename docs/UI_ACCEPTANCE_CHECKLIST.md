# StudyGuardian UI 验收清单

## Stage B 当前 Gate

- [x] Vite 具备 Pet、Quick Panel、Control Center 三个 HTML 入口。
- [x] Quick Panel 有清晰的 mode/task/progress/action/footer 层级。
- [x] Control Center 有侧栏、Dashboard hero、metrics、7 日趋势、任务、成就和复盘区域。
- [x] Light token、Dark token、reduced-motion 和键盘 focus-visible 已建立。
- [x] 主要产品图标使用 Lucide，趋势图使用 Recharts。
- [x] Pet 原有 canvas/behavior/native transport 未被 React 化或重写。
- [x] `npm test` 与 `npm run build` 通过。

## Stage C 自动化 Gate

- [x] Quick Panel（380×430，隐藏创建、置顶、跳过任务栏）与 Control Center（1040×720，隐藏创建、可调整大小）的 Tauri 配置已建立。
- [x] 辅助窗口按 label 延迟创建并复用；关闭请求转为隐藏，Quick Panel 失焦自动隐藏。
- [x] Quick Panel 优先贴靠 Pet，并在正/负坐标工作区内进行有界定位；Rust `cargo check` 与 7 项单测通过。
- [x] Tauri dev 在 NTFS 验证副本成功启动，主窗口标题为 `StudyGuardian Pet v3`；本地浏览器已复查两个 React 页面。

## Stage C 尚未通过的人工 Gate

- [ ] Windows 原生 Quick Panel / Control Center 的真实显示、Escape、失焦隐藏、重复打开复用与 Pet 相邻定位。

## Stage D 自动化 Gate

- [x] Rust native client 只访问固定 Supervisor GET 路径，并对白名单字段、字符串长度、数值范围和数组数量做验证。
- [x] Quick Panel native runtime 接入 canonical status/motivation；mode/task、专注计时、目标、streak、AP 使用 Supervisor 返回值，模式控制继续走 Rust 固定 POST 路径。
- [x] Control Center native runtime 接入 status、motivation、7 天 history、achievements、missions、rewards 和 AI status；空/异常可选数据以空态显示。
- [x] 源 WSL 与 NTFS 验证副本 Node 测试均为 21/21，Rust `cargo check` 与单测均为 8/8，Vite 三入口构建通过。

## 尚未通过的后续 Gate

- [ ] Control Center Review、History actions 与 Study Center parity。
- [ ] Settings typed API、验证、atomic persistence、secret masking。
- [ ] Tray 与 Pet/Quick Panel/Control Center 统一。
- [ ] Windows 原生窗口的视觉/交互验收，包括 Light/Dark 主题。
- [ ] PyQt retirement 前的 feature parity 与 rollback path。

## 视觉拒绝条件

出现默认表单、随机渐变、emoji 主图标、核心文字小于 12px、原始错误码、过度
卡片化、过重阴影或过多颜色时，不能标记 UI PASS。

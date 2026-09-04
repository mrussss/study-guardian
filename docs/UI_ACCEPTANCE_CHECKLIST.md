# StudyGuardian UI 验收清单

## Stage B 当前 Gate

- [x] Vite 具备 Pet、Quick Panel、Control Center 三个 HTML 入口。
- [x] Quick Panel 有清晰的 mode/task/progress/action/footer 层级。
- [x] Control Center 有侧栏、Dashboard hero、metrics、7 日趋势、任务、成就和复盘区域。
- [x] Light token、Dark token、reduced-motion 和键盘 focus-visible 已建立。
- [x] 主要产品图标使用 Lucide，趋势图使用 Recharts。
- [x] Pet 原有 canvas/behavior/native transport 未被 React 化或重写。
- [x] `npm test` 与 `npm run build` 通过。

## 尚未通过的后续 Gate

- [ ] Quick Panel / Control Center 的 Tauri 窗口单实例、定位、隐藏和 Escape 行为。
- [ ] Quick Panel 真实 Supervisor 状态、Study/Break/Off、专注进度、streak、AP。
- [ ] Control Center 真实 Dashboard、Missions、Achievements、Rewards、Review、History。
- [ ] Settings typed API、验证、atomic persistence、secret masking。
- [ ] Tray 与 Pet/Quick Panel/Control Center 统一。
- [ ] Windows 原生窗口的视觉/交互验收，包括 Light/Dark 主题。
- [ ] PyQt retirement 前的 feature parity 与 rollback path。

## 视觉拒绝条件

出现默认表单、随机渐变、emoji 主图标、核心文字小于 12px、原始错误码、过度
卡片化、过重阴影或过多颜色时，不能标记 UI PASS。

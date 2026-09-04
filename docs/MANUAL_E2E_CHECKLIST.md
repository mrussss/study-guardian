# StudyGuardian v1.0 手动验收清单

这份清单只记录必须在 Windows 用户会话中完成的真实动作。自动化测试、
fake display context、localhost fixture 和部署 smoke 不能替代下列项目。

## 前置条件

- [ ] 源码真源仍为 WSL `~/projects/study-guardian`，先确认 `git status`
  干净且 `HEAD` 与 `origin/main` 一致。
- [ ] `D:\StudyGuardianDev` 已由最新源码构建、部署；Supervisor、Sensor、
  ActivityWatch、Pet 和 watchdog 均已启动。
- [ ] 若验收 Tauri Pet，先安装并验证 Rustup/Rust/Cargo、Visual Studio Build
  Tools 的 Desktop development with C++、MSVC 和 Windows SDK；未满足时只将
  Tauri compile/runtime 标为 BLOCKED。

## A. 真实 Chrome / Collector（Task 89）

- [ ] 在 Chrome `chrome://extensions` 开启 Developer mode，加载
  `D:\StudyGuardianDev\browser\chatgpt-collector` unpacked extension。
- [ ] 只在本地扩展设置中配置 Collector token；不要把 token 写入仓库、截图
  或聊天记录。
- [ ] 打开既有 ChatGPT conversation，确认 baseline 不会重复上报。
- [ ] 发送一轮新的 user/assistant turn，确认 streaming 最终只形成一个
  canonical turn/message。
- [ ] reload 页面，切换 conversation，再发送一轮；检查 offline queue、
  dedupe、wrong-day 字段和 Supervisor collector API 结果。
- [ ] 记录 DevTools / Supervisor API 的脱敏证据；若仍出现 Debugger
  unattached，记录 BLOCKED 原因，不伪造 PASS。

## B. 锁屏与 Sleep/Resume（Tasks 17 / 50 / 68 / 101）

- [ ] STUDY 中记录操作前 `raw_time`、`credited_time` 和 session 状态。
- [ ] 执行 `Win+L`，等待后解锁；确认 lock 期间不计入用户专注时间，解锁后
  仍能恢复服务。
- [ ] 让 Windows Sleep，再 Resume；确认没有把挂起时长计入 session/raw/
  credited time，下一次正常 tick 可以继续累计。
- [ ] 检查 Supervisor `/v1/status`、`/healthz`、日志和 SQLite 中没有巨大时间
  跳变；记录 resume 前后时间戳。

## C. 多显示器真实热插拔（Task 39）

- [ ] 在双显示器状态调用 `/v1/monitors`，记录 virtual desktop、每个 physical
  monitor 的 `left/top/width/height`，特别保留负坐标。
- [ ] 物理拔出或禁用一台显示器，等待至少一个 monitor listing 周期；确认
  Sensor 不崩溃、context 被重新发现、几何信息更新。
- [ ] 恢复显示器，再次确认 listing 恢复，Supervisor 仍健康，截图策略未绕过
  Privacy Gate。

## D. 崩溃恢复 / watchdog（Task 50）

- [ ] 在确认当前服务可恢复且没有未保存操作后，按现有 watchdog 验收方案只
  终止一个受监管子进程。
- [ ] 记录 watchdog 重新拉起进程的时间、PID 变化、healthz 恢复和日志脱敏。
- [ ] 确认不会重复启动、不会触碰 `config/data/logs/run/handoff` 或 venv。

## E. Tauri Pet native transport and interaction（Task 101）

- [ ] `pet-v3` 执行 `npm ci`、`npm test`、`npm run build`，再执行 `npm run
  tauri dev` 或构建后的 Tauri binary。
- [ ] 在线时确认 `supervisor_snapshot` 返回 `connected=true`、完整
  `CurrentActivityView` 和 `last_success_at`，响应中没有 token、路径或任意
  native error 原文。
- [ ] 关闭 Supervisor / 使用错误 token / 制造超时 / 返回坏 JSON，确认前端只
  看到 `unauthorized`、`timeout`、`unavailable` 或 `invalid_response`。
- [ ] 单击小猫（移动距离小于 5 CSS px）打开生产控制面板；重复 10 次确认窗口
  不移动且不会被 native drag 吞掉。
- [ ] 在面板输入任务并点击“开始学习”，确认真实 Supervisor 状态变为
  `STUDY`，任务值正确；点击“开始休息”，确认状态变为 `BREAK` 或显示有界的
  `当前状态不允许该操作`。
- [ ] 明显拖动小猫 100px 以上，确认调用 native drag、窗口移动且不打开面板；
  交替执行 click / drag / click / drag，确认没有误触。
- [ ] 点击面板 input、Study、Break、关闭按钮并输入文字，确认窗口不跟随移动。
- [ ] 关闭 Supervisor 后打开面板并点击模式按钮，确认 Pet 不崩溃且只显示有界
  的 `unavailable` / `timeout` / `unauthorized` / `invalid_response` 文案。
- [ ] 通过托盘菜单恢复 click-through；确认 Pet 仍保持 always-on-top、透明背景、
  shadow=false 和可拖动。Study Center / Review 入口仍记为 PENDING，除非单独
  验证 legacy PyQt Pet 的安全入口。

## F. Final Art / Review Provider

- [ ] 最终桌宠美术由用户选择后再进入仓库；不加入 Chiikawa/吉伊素材。
- [ ] 仅当用户已配置真实 Review Provider credential 时，在隔离测试数据上做
  一次 smoke；否则记录 fallback PASS，不打印或提交 credential。

## 证据记录

每项记录日期、源码 HEAD、Windows 部署版本、动作时间、观察结果和日志/API
脱敏片段。没有真实动作或受环境阻塞的项目必须标为 `BLOCKED` / `PENDING`，
不能用单元测试替代。

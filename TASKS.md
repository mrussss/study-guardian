# TASKS.md — StudyGuardian 任务跟踪表

## Phase 0: 补齐剩余 PoC 与开发骨架
- [x] 01. 读取文档与现有环境，确认 D:\StudyGuardianDev 当前目录与已验证 PoC
- [x] 02. 创建 / 整理 WSL repo、AGENTS.md、TASKS.md、.gitignore、scripts 骨架
- [x] 03. Supervisor 最小 :17321 /healthz
- [x] 04. Sensor 最小 :17322 /healthz + capture stub
- [x] 05. Pet 最小 Supervisor client
- [x] 06. 验证 Pet → Supervisor → Sensor localhost 通信
- [x] 07. 覆盖 disconnected / invalid token / timeout fail-soft 测试
- [x] 08. build-windows.sh
- [x] 09. deploy-windows.sh
- [x] 10. 连续两次 deploy，验证 D:\StudyGuardianDev\config / data / logs / run / handoff 不被破坏
- [x] 11. Phase 0 自动测试 / Windows smoke test
- [x] 12. commit + push 各独立小块
- [x] 13. 打 tag: phase0-passed 并 push

## Phase 1: 确定性核心
- [x] 14. Clock abstraction (RealClock / FakeClock)
- [x] 15. SQLite migration skeleton (modernc.org/sqlite, CGO-free)
- [x] 16. UserMode state machine + transition tests
- [~] 17. 跨日 / restart / sleep / lock 时间规则与会话管理（长 resume gap 已 fail-closed 且有 FakeClock 回归测试；真实 lock/sleep 仍待手动验收）
- [x] 18. Supervisor /v1/status 与 mode API (/v1/mode/study, /v1/mode/break, /v1/mode/off, /v1/task, /v1/feedback)
- [x] 19. FakeActivitySource / FakeScreenSource 驱动单测与集成测试
- [x] 20. Pet 精简为 UI Shell (去除独立业务逻辑，保留透明置顶、动画、托盘、气泡与菜单)
- [x] 21. Pet ↔ Supervisor 正式 API 通信与轮询
- [x] 22. Reminder Engine + cooldown
- [x] 23. BREAK timeout 处理
- [x] 24. Phase 1 Windows E2E 验证（restart、OFF 保持、Pet 离线 GUI、Toast 触发/通知中心记录已通过；lock/sleep 按本轮决定延后）

## Phase 2: ActivityWatch 正式监督
- [x] 25. ActivityWatch bucket 动态发现 (window / afk buckets)
- [x] 26. ActivityWatchSource 真实适配层与 HTTP 客户端
- [x] 27. Window / title / URL / AFK 有界查询与数据解析
- [x] 28. Active Time 累计计算 (基于 AFK 状态)
- [x] 29. 基础 TaskRelation 规则引擎 (白名单/黑名单/学习任务关键词匹配)
- [x] 30. ActivityWatch offline fail-soft 处理
- [x] 31. Phase 2 单元测试与集成测试
- [~] 32. Phase 2 Windows 验证（ActivityWatch fresh/stale、临时停止恢复、动态 rediscovery 已自动覆盖；完整真实 Windows 矩阵待完成）

## Phase 3: Screen Sensor 正式接入
- [x] 33. mss 正式 capture 实现与双端口服务
- [x] 34. monitor / virtual desktop 处理
- [x] 35. pHash / dHash 屏幕变化检测
- [x] 36. Privacy Gate 截图前门禁校验 (sensitive apps / domains)
- [x] 37. InteractionState 综合判定 (ACTIVE / IDLE_STATIC / IDLE_DYNAMIC / UNKNOWN)
- [x] 38. Sensor timeout / crash fallback fail-soft
- [~] 39. Phase 3 自动测试与 Windows 实测（MSS/真实捕获、上下文 rediscovery 与几何回归已覆盖；真实多显示器热插拔待完成）

## Phase 4: AI Classification
- [x] 40. TaskRelationProvider 接口定义与模型适配
- [x] 41. VisionProvider 接口与低分辨率/裁剪支持
- [x] 42. 结构化 JSON Schema 输出校验
- [x] 43. 本地规则优先 / AI fallback 机制
- [x] 44. Classification cache (app/title/domain/task/hash)
- [x] 45. AI timeout / invalid response fallback
- [x] 46. 敏感窗口禁止 Vision 门禁与元数据过滤
- [x] 47. Provider unavailable fail-soft 保证系统正常运行

## Phase 5: 交互优化与发布交付
- [x] 48. Pet 动画 / 气泡 / feedback UX 完善
- [x] 49. Windows 本地通知集成 (Toast)（Windows 11 实机触发并确认通知中心记录）
- [~] 50. Startup / resume / crash recovery（启动脚本已实现；resume 与 crash 自动拉起待完成）
- [x] 51. 日志轮转 (Rotating logs)
- [x] 52. 稳定性检查清单与测试工具
- [x] 53. Release packaging 骨架
- [x] 54. 整理 ARCHITECTURE.md, PRIVACY.md, TEST_PLAN.md, OPEN_SOURCE.md
- [~] 55. 全量自动化测试回归与 Windows smoke test（自动化与本轮 Pet/Toast smoke 已通过；多显示器热插拔、崩溃自动拉起仍待后续）
- [x] 56. 中文开发完成总结汇报
- [x] 57. 准备 docs/FINAL_AUDIT.md 模板与 handoff 数据供独立审计

## Phase 6: 产品化（v0.6）
- [x] 58. AI Provider V2：文本/视觉分离、国内 profile、JSON fallback、cooldown、状态 API
- [x] 59. Legacy AI 配置内存迁移与安全写入脚本（`migrate-config.ps1` / `configure-ai.ps1`）
- [x] 60. Study Forest：有效专注单一 TickOutcome、每日 FP/AP、milli-AP ledger、打卡、连续天数
- [x] 61. Mission / Achievement / Reward 本地 SQLite 模型、幂等操作与 Supervisor API
- [x] 62. PyQt 懒加载 Study Center，业务状态以 Supervisor API 为唯一来源
- [x] 63. Manifest 皮肤系统、用户皮肤目录、持久偏好、像素缩放、celebrate 状态与 fail-soft
- [x] 64. 原创 Phase 6 占位皮肤素材与代码/素材许可证说明
- [x] 65. Python requirements 与 venv 重建脚本
- [x] 66. staging 部署与程序路径精确替换，保护 config/data/logs/run/handoff/venv
- [x] 67. Phase 6 Go/Python/部署安全回归与 Windows 运行检查
- [~] 68. 1～3 天日常试运行已启动（2026-09-02，D:\StudyGuardianDev 保持运行）；锁屏、Sleep/Resume、双显示器热插拔和崩溃 watchdog 延后

## Phase 6 v0.7 复审修正
- [x] 69. Credited Focus：单一 TickOutcome、静态阅读 grace、Raw Study Time 与奖励时间分离
- [x] 70. motivation_settings、canonical status、事件表与 after_id cursor API
- [x] 71. Study Center 全部 HTTP 移出 Qt GUI Thread，并覆盖离线/超时 fail-soft
- [x] 72. AI：Qwen 共享默认地址、可选 temperature、文本/视觉独立 provider 与缓存 key
- [x] 73. Pet：pet.json 唯一偏好真源、事件 cursor 持久化、重复事件防护
- [x] 74. Runtime：requirements/dependency audit、venv.new 安全回滚、staging 精确替换
- [ ] 75. Final Visual Asset Pending：最终桌宠美术资产仍待确定；当前仅使用原创 placeholder
- [~] 76. v0.7 Windows E2E 与 1～3 天日常试运行（锁屏/Sleep/Resume 按用户决定延期）
- [x] 77. 复审修正：TickOutcome.Now 单一时钟、COMEBACK 连续专注、DAILY_120 固定阈值
- [x] 78. 复审修正：Rules → Text AI → Vision fallback、按端点 timeout、model 配置校验
- [x] 79. 复审修正：Deploy 停进程、ephemeral rollback、health smoke、真实文件统计口径

## Phase 7: Daily Review Foundations

- [x] 80. Extract generic OpenAI-compatible AI transport without classifier regression
- [x] 81. Chat conversation / turn / message migrations
- [x] 82. Semantic snapshot / review exclusion / daily review migrations
- [x] 83. Scoped collector token + collector context API
- [x] 84. ChatGPT Collector baseline POC
- [x] 85. STUDY turn eligibility freeze
- [~] 86. Streaming / reload / regenerate / edit / branch dedupe（已完成普通 Assistant streaming canonicalization、reload、SPA conversation switch、MV3 Worker restart、candidate dedupe；Regenerate/Edit/复杂 Branch 待后续）
- [x] 87. Offline queue + observed_at local-date correctness
- [x] 88. Semantic snapshots without extra Vision/capture
- [ ] 89. Phase 7 unit/integration/Windows E2E（v1.2 自动测试、打包与部署 smoke 完成；真实 Chrome + ChatGPT E2E 仍受 Debugger unattached 阻塞）

## Phase 8: Daily Review

- [x] 90. Daily Evidence Aggregator over existing + new data
- [x] 91. Deterministic fallback report
- [x] 92. Conversation compaction
- [x] 93. Generic ReviewProvider + JSON schema
- [x] 94. Evidence-ref / accomplishment validator
- [x] 95. Cloud sanitizer
- [x] 96. Canonical daily review persistence + input hash
- [x] 97. OFF debounce / stale / restart / next-day backfill
- [x] 98. Markdown atomic output
- [x] 99. DAILY_REVIEW_READY ui_event
- [x] 100. Pet / Study Center Review UI（含安全 evidence exclusion）
- [x] Retention cleanup（启动时及每日 worker 原子清理；保留 daily reviews、sessions 与 Study Forest 数据）
- [~] 101. Full Windows E2E（Tauri native transport、生产 Study/Break 控制面板、production EXE 与部署、single-instance UI shell、Node 20 项/Rust 14 项已通过；原生窗口视觉/输入、真实 mode/error 和完整 Windows E2E 待人工验收）

## Phase 9: Trial

- [ ] 102. 7-day daily-use trial
- [ ] 103. Collector miss/duplicate/wrong-day audit
- [ ] 104. Review factuality audit
- [ ] 105. Optional Git evidence

## Phase 10: Modern UI productization

- [x] 106. Stage B React multi-page foundation：独立 Quick Panel / Control Center、
  design tokens、Lucide、Recharts、静态 Dashboard 与 UI acceptance/design docs
- [~] 107. Stage C Quick Panel / Control Center Tauri window orchestration（窗口配置、
  延迟创建/单实例复用、Pet 相邻定位、Escape/显式 hide 生命周期、Quick Panel →
  Control Center 的 overview/settings/review bounded route 与多显示器边界自动验证已完成；
  focus loss 只记录诊断、不再隐式销毁可交互状态，原生 Windows 窗口视觉/交互仍待人工 Gate）
- [~] 108. Stage D/E 已接入受限 native Supervisor dashboard DTO（status、motivation、history、
  achievements、missions、rewards、AI status）与 Quick Panel 实时 mode/task/进度/模式控制；
  Control Center 其余页面、Review/Study Center parity 与原生窗口人工 Gate 待完成
- [~] 109. Stage F/G 已完成每日目标、quiet periods、AI provider/model/URL/secret/test、
  configured-only DTO、运行时重载和 autostart native toggle；其余 Screen/Privacy 配置仍待完成
- [~] 110. Stage H/J 已完成 tray 入口、single-instance、production EXE、稳定 Launcher 与部署；
  Tauri Pet 默认切换和现代 UI Windows visual acceptance 待人工 Gate


### 2026-09-04 重启后输入修复补充

- Task 107：已定位并修复同步 WebView 创建死锁、UI 线程网络阻塞、路由监听权限和重复路由请求；原生 Quick Panel 20 次显示/隐藏验证通过。
- Pet 候选改为原生 CSS 拖动 + 独立“学习面板”按钮；完整真人 50 次拖动 / 20 次点击 / 10 次交替仍待验收，详见 `docs/PET_INPUT_ARCH_DECISION.md`。
- Task 110 已增加托盘 UI 入口，但生产统一切换和完整视觉验收仍未完成。
- Task 109 的完整设置范围没有在本轮宣称完成。

## Phase 11: Product completion pack

- [x] 111. Task presets、置顶与 recent study tasks
- [x] 112. Active-session task persistence 与 restart recovery
- [x] 113. Reminder quiet periods、24:00 校验与 no-debt behavior
- [x] 114. AI Settings、脱敏 API 与原子 secret 管理
- [~] 115. Provider connection test 与 runtime status（真实 provider credential E2E 未运行）
- [x] 116. Offline Daily Review v2 与事实型 fallback
- [x] 117. OFF-state Review UX、immediate generate 与 debounce generate
- [~] 118. Feature-pack Windows E2E（自动化与进程级启动通过；设置 UI 人工流程待验收）
- [~] 119. Unified Windows Launcher 与 single-instance activation（进程级重复激活通过；窗口焦点待人工确认）
- [~] 120. Windows autostart integration（真实 Startup enable/state/disable 通过并恢复关闭；重启/登录 Gate 未运行）
- [~] 121. Desktop shortcut integration（真实快捷方式和冷/重复启动通过；窗口焦点待人工确认）
- [~] 122. Tauri v3 Windows production packaging/deploy/runtime selection（build/deploy PASS；cutover PENDING USER GATE）
- [~] 123. Windows startup integration E2E（1 个逻辑 PyQt Pet、Supervisor、Sensor、watchdog 和 Tauri UI shell；reboot/sign-in 未运行）

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
- [~] 17. 跨日 / restart / sleep / lock 时间规则与会话管理（自动测试与 restart 已通过；按本轮验收决定 lock/sleep 延后且不阻塞）
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
- [~] 32. Phase 2 Windows 验证（ActivityWatch fresh/stale 已通过；完整矩阵待完成）

## Phase 3: Screen Sensor 正式接入
- [x] 33. mss 正式 capture 实现与双端口服务
- [x] 34. monitor / virtual desktop 处理
- [x] 35. pHash / dHash 屏幕变化检测
- [x] 36. Privacy Gate 截图前门禁校验 (sensitive apps / domains)
- [x] 37. InteractionState 综合判定 (ACTIVE / IDLE_STATIC / IDLE_DYNAMIC / UNKNOWN)
- [x] 38. Sensor timeout / crash fallback fail-soft
- [~] 39. Phase 3 自动测试与 Windows 实测（MSS/真实捕获已通过；多显示器热插拔待完成）

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

# Windows E2E Report

## 2026-09-04 接力复核

- 当前 WSL `main` 与 `origin/main` 已核对为 `0 0`，工作区干净。
- `bash scripts/test-all.sh` 通过：Go、Python 单元、7 个 Phase 0–4 集成测试和两次 deploy safety 均 PASS；`go vet ./...` 与部署后 PowerShell 脚本语法检查也 PASS。
- 最新 `build-windows.sh` 与 `deploy-windows.sh` 已成功执行；Supervisor `:17321` 与 Sensor `:17322` 健康，ActivityWatch 与 MSS 均报告可用。
- 轻量 watchdog 已包含在最新部署中，停止脚本先停 watchdog 再停 Supervisor/Sensor/Pet；本轮已分别终止 D 盘 Supervisor、Screen Sensor、生产 Pet 的精确进程目标，watchdog 按 5/10/20 秒退避自动拉起，最终 Supervisor/Sensor health 均为 `ok`、Pet 进程恢复。
- Daily Review 92–100 已完成；retention worker 仅清理过期 raw chat / semantic snapshots，不删除 daily reviews、sessions、Study Forest 或配置。
- Collector 41 项测试在 NTFS 验证副本中 41/41 通过；Pet v3 在 WSL 原生 Node 22 与 NTFS 验证副本中均完成测试/TypeScript 检查，NTFS 副本用于 Tauri Windows runtime 验收。
- Tauri `supervisor_snapshot` 已补齐：复用 `D:\StudyGuardianDev\config\auth.token` 与 `config.yaml`，仅允许 loopback Supervisor、2 秒有界 HTTP、白名单 semantic 字段和四类脱敏错误；NTFS 验证副本的 `npm ci`、`npm test`（17/17）、`npm run build`、`cargo check`、`cargo test`（6 tests）均通过，`npm run tauri dev` 已启动并保持 `StudyGuardian Pet v3` 响应。Fix Pack 的 native `startDragging()`、shadow 关闭、开发面板 gating 与 CSS hit-target 已编码并完成自动验证。当前 CUA 未枚举该原生窗口，视觉/UI 交互验收仍需人工完成。
- Interaction Fix Pack v1 已完成自动验证：生产面板与 dev mock panel 分离；小于 5px 的左键手势切换面板，达到阈值后仅调用一次 Tauri native `startDragging()`；Study/Break 通过 Rust `supervisor_set_mode` 访问固定 loopback Supervisor 路径，任务 JSON 由 Rust 编码，token 不进入 WebView。源 WSL 与 NTFS 副本的 Node 测试统一为 17/17，NTFS `cargo check` 与 `cargo test` 为 6/6，重新启动的 `StudyGuardian Pet v3` 临时进程响应正常。当前 CUA 未枚举该原生窗口，因此真实 click/drag/mode 仍为人工 PENDING，未宣称 UI PASS。
- Modern UI Stage B/C 已完成自动复核：源 WSL 与 NTFS 验证副本的 Node 测试均为 20/20，TypeScript 检查与 Vite 三入口构建均 PASS；Quick Panel 与 Control Center 的本地浏览器页面已完成视觉复查。NTFS 副本的 Tauri 配置包含延迟创建、单实例复用、Quick Panel Pet 相邻定位、Escape/focus-loss hide 和多显示器边界夹紧；`cargo check` 与 Rust 7/7 单测 PASS，`npm run tauri dev` 成功启动并保持 `StudyGuardian Pet v3` 响应。当前 CUA 仍未枚举 Tauri 原生辅助窗口，因此原生窗口显示/交互仍为人工 PENDING，未宣称 UI PASS。
- Modern UI Stage D 已完成自动复核：native dashboard 固定路径 transport 对 canonical status、motivation、history、achievements、missions、rewards、AI status 做白名单/边界脱敏；Quick Panel 的 mode/task/计时/目标/streak/AP 与模式控制接入 canonical Supervisor，Control Center 接入可选数据空态。源 WSL 与 NTFS 验证副本 Node 测试均为 21/21，Rust `cargo check` 与单测均为 8/8，Vite 三入口构建 PASS；原生辅助窗口仍未被当前 CUA 枚举，视觉/交互继续人工 PENDING，未宣称 UI PASS。
- Modern UI Stage F/G 已完成每日目标设置的自动复核：固定 `/v1/motivation/settings` typed native control、1–1440 范围校验、Supervisor atomic storage 和 bounded result 均通过；设置页不接触 token/AI secret。完整配置 parity 与原生窗口交互仍待后续 Gate。
- 传感器 monitor listing 新增 fake MSS context rediscovery/负坐标/几何变化回归；真实物理显示器热插拔仍保留为人工验收。

## Environment

- Windows: Windows 11 desktop environment
- WSL: Ubuntu-22.04
- Go: WSL Go toolchain
- Python: Windows Python 3.11; Sensor venv with `mss` and Pillow; Pet venv with PyQt6
- ActivityWatch: 0.13.2 at `127.0.0.1:5600`
- Displays: MSS reported virtual desktop plus one physical monitor (`monitor: 0`)
- Git Commit: 当前 `HEAD`（每次复核以 `git log -n 1` 与 `origin/main` 重新核对）
- Runtime: `D:\StudyGuardianDev`

## PASS

- Go unit tests: `go test ./...`
- Python Pet and Sensor unit tests via `unittest`
- Python integration tests: Phase 0 through Phase 4
- ActivityWatch dynamic bucket discovery and real event timestamp parsing
- Real ActivityWatch fresh event: VS Code window became current and Supervisor recovered to `activitywatch_ok=true`
- Real ActivityWatch stale window event: Supervisor reported `activitywatch_ok=false` and `UNKNOWN`
- Screen Sensor real MSS health: `mss_available=true`
- Screen Sensor real capture: HTTP 200, `is_stub=false`, `monitor=0`
- Supervisor localhost status and invalid Sensor token: HTTP 401
- Same-day STUDY restart recovery: mode/task/study total preserved
- Same-day OFF restart recovery: mode remained OFF
- Two consecutive deployments: persistent config, token, data, logs, run and handoff directories preserved
- Pet process startup after moving HTTP polling to a worker thread
- Startup and stop scripts: idempotent port checks and reliable Python process cleanup
- Supervisor stopped while Pet remained running: Pet window stayed responsive through multiple HTTP timeout polls
- Pet GUI drag: real window moved from `(2291, 1271)` to `(2336, 1286)` as expected
- Pet context menu: right-click opened the native menu with status and mode actions while Supervisor was offline
- Windows 11 Toast: isolated BREAK test produced `current_reminder.level=TOAST`, reason `BREAK_TOO_LONG_STRONG`; the Windows notification database contains the corresponding `StudyGuardian 提醒 / 休息已满 1 分钟` record and the `StudyGuardian` notification registration timestamp advanced
- Normal runtime restored after the isolated Toast test; Supervisor, Sensor, ActivityWatch, and Pet were left running for daily trial
- Phase 6 motivation status, missions, rewards, and AI status APIs returned successfully on the deployed Windows runtime
- Phase 6 manifest skin directories and both isolated Python requirements files were present after deployment
- Phase 6 staging deployment passed twice while preserving persistent directories, token, and existing virtual environments
- Phase 6 Pet process stayed alive with the manifest skin renderer and lazy Study Center modules deployed
- Daily trial started after the final deployment on 2026-09-02; runtime was left running for the requested 1–3 day observation window

## Phase 6 v0.7 复审补充

- WSL source of truth remains `~/projects/study-guardian`; Windows runtime remains `D:\StudyGuardianDev`.
- Credited Focus boundary tests passed: ACTIVE and IDLE_DYNAMIC credit; IDLE_STATIC + FOCUSED credits only within the 300-second grace; IDLE_STATIC + UNKNOWN and over-grace static reading do not credit; DISTRACTED and UNKNOWN interaction do not credit.
- Motivation settings persistence passed in SQLite and API tests; changing the target updates the current day's target while YAML remains only the initial default.
- Canonical status fields now distinguish credited focus, today's earned/spent milli-AP, deterministic balance and daily target progress.
- `GET /v1/events?after_id=<cursor>&limit=20` and Pet `config/pet.json.last_event_id` cursor behavior are covered by storage/service tests; important events are consumed in ID order and not replayed after acknowledgement.
- Study Center operations use a `QThread` worker for all Supervisor HTTP calls, including refresh and mutations; timeout/500 results are rendered as an error state and do not block window navigation or close.
- OpenAI-compatible request tests passed: default requests omit `temperature`; explicit configuration sends it; JSON fallback is restricted to explicit HTTP 400/422 compatibility errors; provider/model and text/vision kind are part of cache identity.
- Qwen profile uses the shared DashScope Beijing compatibility endpoint by default. Unknown providers remain rules-only and Fake remains developer-only.
- `pip check` passed for the deployed Pet and Sensor environments; direct requirements contain only the runtime dependencies audited in `docs/RUNTIME_AUDIT.md`.
- Safe venv rebuild now installs and smoke-checks `.venv.new` before swapping, and restores `.venv.backup` if a post-swap smoke check fails.
- Deploy safety passed across two deployments with persistent canaries and no stale files under the exact ephemeral replacement paths.
- Built-in skins are source-controlled; user skins remain under `config/pet-skins`; `Final Visual Asset Pending` remains explicit for the placeholder art.
- `TickOutcome.Now` is now populated by State Manager and Motivation rejects a zero timestamp; FakeClock tests confirm credited focus is stored on the outcome date rather than the machine wall-clock date.
- `DAILY_120` remains a fixed 7,200-second achievement threshold regardless of the editable daily target; `COMEBACK` uses persisted post-distraction continuous credited focus and does not reuse lifetime focus.
- Supervisor now captures a non-image screen sample first, runs Rules/Text classification, and requests an analysis image only for a configured Vision fallback after UNKNOWN/low-confidence text results. The selected endpoint timeout is preserved (default Text 6 seconds / Vision 8 seconds).
- Provider registry marks a real endpoint unconfigured when its model is missing and reports a clear model warning before any request.
- Deploy now invokes the runtime stop script before replacing files, retains a recoverable backup of exact ephemeral paths until health smoke passes, and restores those paths on replacement/smoke failure.

## v0.7 Runtime Measurements

Captured on 2026-09-02 after the v0.7 deployment. Working Set is a live process measurement, not a disk-size estimate.

| Measurement | Result |
|---|---:|
| Pet venv disk | 241,260,482 bytes / 5,073 files |
| Sensor venv disk | 40,230,413 bytes / 1,764 files |
| Pet direct package count | 3 |
| Sensor direct package count | 2 |
| Pet Working Set (venv launcher + interpreter) | 112,087,040 bytes |
| Sensor Working Set (venv launcher + interpreter) | 34,906,112 bytes |
| Supervisor Working Set | 21,929,984 bytes |
| Runtime tree files excluding persistent data and venvs | 6,916 |
| StudyGuardian program-owned files (`bin/pet/scripts/sensor`, excluding bundled ActivityWatch and legacy `poc`) | 67 |
| Startup smoke observation | all four components reported started; healthz PASS after 5 seconds |

The Pet launcher/interpreter pair and Sensor launcher/interpreter pair are expected on this Windows Python setup; they are not duplicate business services. The runtime-tree count includes 1,421 bundled ActivityWatch files and 5,428 legacy `poc` files; the 67-file program-owned count is the relevant StudyGuardian deployment footprint.

## FAIL

- None observed in the automated or executed Windows checks above.

## Fixed During This Pass

- ActivityWatch snapshots now use actual watcher event timestamps and expose per-watcher timestamps.
- Stale or unavailable ActivityWatch data now forces `UNKNOWN`, pauses active-time accumulation, and skips capture/AI.
- Restart recovery now loads only interrupted open sessions, closes all stale open rows, and avoids session ID collisions.
- Session durations now use persisted tick time instead of wall-clock downtime; lock-screen ticks pause all mode timers.
- BREAK reminder input uses the current BREAK session duration and BREAK no longer performs task-relevance AI classification.
- Production defaults disable Fake AI; invalid AI schema, confidence, and oversized text fail soft to rules.
- Screen sampling supports configurable monitor selection and reports unusable MSS instances as unhealthy.
- Pet Supervisor polling runs outside the Qt GUI thread.
- Added unified test runner and per-user startup install/uninstall scripts.
- Added AI Provider V2 profiles, secure config helper/migration scripts, motivation ledger/API, lazy Study Center, manifest skins, isolated requirements, and staging deployment.

## Known Limitations

- The installed real machine has one physical monitor; negative-coordinate dual-monitor and unplug/replug behavior were not available to validate.
- Lightweight bounded crash recovery/watchdog is implemented in `scripts/watchdog.ps1`; full seven-day observation and every recovery edge case remain outside this report's current automated run.
- Per the current acceptance decision, Windows lock-screen and Sleep/Hibernate/Resume remain explicitly deferred and are not blockers for this 1–3 day daily trial. Core lock/long-gap rules pass with deterministic clocks.
- The existing user config intentionally enables `provider: fake`; the repository default is now `enabled: false, provider: none`.
- Final visual artwork is still pending; placeholder skin assets are not presented as final product art.
- Tauri runtime process startup is verified in the NTFS copy, but the current
  CUA surface did not expose its native window for visual/UI interaction
  evidence.
- Pet v3 Interaction Fix Pack automatic checks pass, but the native window is
  still not exposed through the current CUA surface; production panel click,
  native drag, Supervisor STUDY/BREAK, and bounded offline/error behavior are
  therefore not yet Windows-manually accepted.

## Not Tested

- Windows lock/unlock timing on the Secure Desktop
- Sleep/Hibernate/Resume timing on physical hardware
- Dual physical monitor arrangement and hot unplug/replug
- Toast banner animation timing and cooldown behavior beyond the triggered notification-center record
- Automatic child-process crash recovery
- Visual pixel-skin artwork comparison beyond process startup; the original placeholder skin is loaded from the manifest path and no user skin is installed on this machine.
- Pet v3 production panel and click/drag/mode interaction on the native window.

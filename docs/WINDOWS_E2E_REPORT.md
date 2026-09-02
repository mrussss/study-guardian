# Windows E2E Report

## Environment

- Windows: Windows 11 desktop environment
- WSL: Ubuntu-22.04
- Go: WSL Go toolchain
- Python: Windows Python 3.11; Sensor venv with `mss` and Pillow; Pet venv with PyQt6
- ActivityWatch: 0.13.2 at `127.0.0.1:5600`
- Displays: MSS reported virtual desktop plus one physical monitor (`monitor: 0`)
- Git Commit: `2caf7c0` (`feat(product): deliver phase 6 study guardian productization`)
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
- Crash auto-restart/watchdog is not implemented; `start-all.ps1` is a reliable one-shot/idempotent launcher only.
- Per the current acceptance decision, Windows lock-screen and Sleep/Hibernate/Resume remain explicitly deferred and are not blockers for this 1–3 day daily trial. Core lock/long-gap rules pass with deterministic clocks.
- The existing user config intentionally enables `provider: fake`; the repository default is now `enabled: false, provider: none`.

## Not Tested

- Windows lock/unlock timing on the Secure Desktop
- Sleep/Hibernate/Resume timing on physical hardware
- Dual physical monitor arrangement and hot unplug/replug
- Toast banner animation timing and cooldown behavior beyond the triggered notification-center record
- Automatic child-process crash recovery
- Visual pixel-skin artwork comparison beyond process startup; the original placeholder skin is loaded from the manifest path and no user skin is installed on this machine.

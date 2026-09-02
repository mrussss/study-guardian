# Windows E2E Report

## Environment

- Windows: Windows 11 desktop environment
- WSL: Ubuntu-22.04
- Go: WSL Go toolchain
- Python: Windows Python 3.11; Sensor venv with `mss` and Pillow; Pet venv with PyQt6
- ActivityWatch: 0.13.2 at `127.0.0.1:5600`
- Displays: MSS reported virtual desktop plus one physical monitor (`monitor: 0`)
- Git Commit: see `git log -1` (working tree contains the v0.4 audit changes)
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

## Known Limitations

- The installed real machine has one physical monitor; negative-coordinate dual-monitor and unplug/replug behavior were not available to validate.
- Crash auto-restart/watchdog is not implemented; `start-all.ps1` is a reliable one-shot/idempotent launcher only.
- `Win + L` lock-screen and Sleep/Hibernate/Resume were not executed automatically because doing so can leave the interactive desktop locked or suspended. Core lock/long-gap tests pass with deterministic clocks, but these remain real-machine pending checks.
- Windows Toast was not visually verified in the notification center during this pass.
- Pet process startup was verified; drag/menu responsiveness under a deliberately stopped Supervisor was not visually exercised.
- The existing user config intentionally enables `provider: fake`; the repository default is now `enabled: false, provider: none`.

## Not Tested

- Windows lock/unlock timing on the Secure Desktop
- Sleep/Hibernate/Resume timing on physical hardware
- Dual physical monitor arrangement and hot unplug/replug
- Visual Windows Toast rendering and cooldown behavior
- Automatic child-process crash recovery

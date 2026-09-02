# Windows E2E Report

## Environment

- Windows: Windows 11 desktop environment
- WSL: Ubuntu-22.04
- Go: WSL Go toolchain
- Python: Windows Python 3.11; Sensor venv with `mss` and Pillow; Pet venv with PyQt6
- ActivityWatch: 0.13.2 at `127.0.0.1:5600`
- Displays: MSS reported virtual desktop plus one physical monitor (`monitor: 0`)
- Git Commit: `87a7a41` (`fix(ai): allow independent vision fallback`)
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
- Crash auto-restart/watchdog is not implemented; `start-all.ps1` is a reliable one-shot/idempotent launcher only.
- Per the current acceptance decision, Windows lock-screen and Sleep/Hibernate/Resume remain explicitly deferred and are not blockers for this 1–3 day daily trial. Core lock/long-gap rules pass with deterministic clocks.
- The existing user config intentionally enables `provider: fake`; the repository default is now `enabled: false, provider: none`.
- Final visual artwork is still pending; placeholder skin assets are not presented as final product art.

## Not Tested

- Windows lock/unlock timing on the Secure Desktop
- Sleep/Hibernate/Resume timing on physical hardware
- Dual physical monitor arrangement and hot unplug/replug
- Toast banner animation timing and cooldown behavior beyond the triggered notification-center record
- Automatic child-process crash recovery
- Visual pixel-skin artwork comparison beyond process startup; the original placeholder skin is loaded from the manifest path and no user skin is installed on this machine.

# StudyGuardian Integrated Product Completion Report

记录日期：2026-09-05（Asia/Shanghai）

## Git

- starting branch: `codex/fix-pet-input-stability`
- starting SHA: `8dbb9cb`
- final branch: `codex/fix-pet-input-stability`
- implementation SHA before docs: `1a587f6`
- origin relation: synchronized through `1a587f6`; the final documentation push is verified at handoff
- working tree: expected clean after the commit containing this report

## Implemented product behavior

- Task presets are stored in SQLite with normalized duplicate handling, pinned/recent ordering, CRUD API, Quick Panel and Control Center pickers. The current open session receives task changes and restart recovery does not restore a stale closed task.
- Reminder settings persist canonical quiet periods. Defaults are 12:00–14:00, 17:30–19:00 and 21:00–24:00; `24:00` is valid only as an end. Quiet exit resets baselines so no reminder debt is emitted.
- AI Settings exposes sanitized provider/model/URL state, stores secrets atomically outside SQLite, performs a minimal real connection request and hot-applies provider changes. Rules-first, Privacy Gate and Vision-default-OFF remain intact.
- Offline Review v2 ranks real task evidence and produces factual `FALLBACK` output with explicit confirmed/unconfirmed statements and a task-related next priority. Immediate and OFF-debounce generation share canonical storage.
- `launch-studyguardian.ps1` is the stable Windows entry. Tauri single-instance IPC activates the existing Quick Panel or Control Center. Desktop and Startup shortcuts call this launcher rather than a versioned executable path.
- Windows production builds create `dist/windows/pet-v3/StudyGuardian.exe`; deploy stages and rolls back this artifact together with the existing runtime while preserving persistent directories and PyQt fallback.

## Windows deployment evidence

- deploy path: `D:\StudyGuardianDev`
- production artifact: `D:\StudyGuardianDev\pet-v3\StudyGuardian.exe`
- artifact size: 10,287,616 bytes
- SHA-256: `469322f4a12430f6c4678390f5781c6327053f44ffc0a2e65261700e503e0af2`
- runtime selection: `{"pet_runtime":"pyqt"}`
- desktop shortcut: `C:\Users\Lenovo\Desktop\StudyGuardian.lnk`, created
- actual autostart state: disabled; a real enable/state/disable cycle created exactly one valid Startup shortcut and restored the original absent state
- preserved config evidence: `auth.token` and `config.yaml` hashes unchanged
- database evidence: the existing database remained in place and grew from 5,713,920 to 5,726,208 bytes when the updated Supervisor ran migrations/health smoke
- repeated desktop activation: one Supervisor, healthy ports 17321/17322, one logical PyQt Pet, one watchdog and one Tauri UI shell after the second launch

The Windows virtual environment launches PyQt through a small interpreter parent and its base Python child. Two `python.exe` processes therefore represent one logical Pet; their parent-child relationship and identical Pet script path were verified.

## Automated verification

- `go test ./...`: PASS
- `go test -race ./...`: PASS
- `go vet ./...`: PASS
- `scripts/test-all.sh`: PASS
- Python Pet tests: 6 PASS
- Python Sensor tests: 6 PASS
- Python integration tests: 7 PASS
- Collector tests: 41 PASS
- Pet frontend tests: 20 PASS
- Pet frontend production build: PASS
- Windows Rust/MSVC `cargo test`: 14 PASS
- Windows PowerShell integration/path/shortcut tests: PASS
- isolated deploy safety: 4 scenarios PASS, including failed-health rollback
- `build-windows.sh`: PASS
- Tauri Windows release build: PASS
- `deploy-windows.sh` and Supervisor health smoke: PASS

## Final Windows acceptance matrix

| Item | Result | Evidence limit |
|---|---|---|
| Task presets | PASS | automated storage/API/UI-contract and restart tests |
| Active-session task persistence | PASS | automated persistence/restart tests |
| Quiet periods | PASS | parser, persistence and no-debt tests |
| AI settings save | PASS | automated sanitized storage and runtime apply tests |
| AI Test Connection | NOT RUN | no real provider credential was supplied |
| AI runtime apply | PASS | automated provider rebuild and race tests |
| No-AI review | PASS | automated immediate/debounce deterministic fallback tests |
| Desktop shortcut cold launch | PASS | real process/port-level launch; visual focus pending |
| Desktop shortcut repeat activation | PASS | real single-stack process result; visual focus pending |
| Autostart install | PASS | real Startup folder contained one valid background-launch shortcut after enable |
| Autostart reboot/sign-in | NOT RUN | requires real sign-out or reboot |
| Autostart disable | PASS | real disable removed the shortcut and state query returned disabled |
| Tauri production artifact | PASS | native Windows/MSVC release artifact built, hashed and deployed |
| Tauri runtime cutover | PENDING USER GATE | default remains PyQt |
| Legacy fallback | PASS | real deployed PyQt runtime started with modern UI shell |
| Full automated tests | PASS | suites listed above |

## Remaining external gates

- Real provider credential connection and ambiguous-rule invocation.
- Control Center visual focus/restore inspection; computer-use exposed no native app window in this session.
- Tauri Pet: 50 drags, 20 clicks, 10 alternating click/drag cycles, tray/click-through and panel repetitions.
- Autostart enable, sign-out/reboot, background behavior, disable and second sign-in.
- Existing unrelated gates: real Chrome Collector, physical lock/sleep, monitor hotplug, final art, seven-day trial and full factuality audit.

No pending external gate is reported as PASS. PyQt remains the selected runtime until the Tauri Pet gate is explicitly accepted.

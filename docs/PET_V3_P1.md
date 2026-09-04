# Pet 3.0 P1 Foundation

`pet-v3/` is an isolated desktop-shell foundation. The production PyQt Pet,
the Supervisor API, `scripts/build-windows.sh`, `scripts/deploy-windows.sh`,
`start-all.ps1`, and the old `pet/` directory are unchanged.

## P1 scope delivered

- Tauri 2 / Rust shell with a transparent, undecorated, always-on-top,
  non-resizable 220px window bounded to 160–260px;
- explicit native left-button `startDragging()` handling after a bounded 5px
  gesture threshold for the pet, state, and task surfaces, with
  interactive-control exclusion and a tray menu;
- transparent CSS/reset and native `shadow: false`/`set_shadow(false)`;
- development mock controls gated behind `DEV + VITE_PET_DEV_PANEL=1`;
- reversible click-through command, with the tray menu as the recovery path;
- exact `CurrentActivityView` TypeScript contract and local mock controls;
  transport `connected` is separate from semantic `fresh`;
- native `supervisor_snapshot` command that reads the existing runtime token,
  calls localhost Supervisor `/v1/activity/current`, validates the bounded
  response, and exposes only the sanitized contract plus a bounded error kind;
- production click-open control panel for real STUDY/BREAK mode actions;
- native `supervisor_set_mode` command with fixed loopback paths, bounded
  STUDY task input, JSON encoding, and `rejected`/transport error kinds; the
  auth token remains entirely inside the Rust boundary;
- pure behavior engine with event/offline/break/distraction/semantic priority,
  normal hysteresis, quick distraction transition, and UI-only THINKING;
- horizontal sprite-sheet frame splitter, FPS animation loop/one-shot/fallback
  completion API, and pixel-rendering settings;
- Skin manifest schema v1 validation plus explicit missing-state fallback;
  the existing PyQt Skin v1 manifest (numeric `frame_size`/`display_size`,
  and lowercase `idle`/`study`/`distracted`/`rest`/`talk`/`celebrate` keys) is
  accepted without changing the old `pet/` assets;
- runtime sprite path: manifest -> state resolution -> cached image load ->
  horizontal sheet split -> `AnimationEngine` -> `drawPixelFrame` -> Canvas;
  `fillRect` is only the final emergency fallback;
- offline visual state and placeholder pixel canvas.

When explicitly enabled, the dev panel uses local mocks. The native shell now
owns the real Supervisor transport; ActivityWatch, Sensor, Text AI, and Vision
AI remain behind Supervisor and are not called directly by the Pet.

## Verification

```text
npm ci                                PASS (existing NTFS verification copy)
npm test                              PASS (17 tests; WSL and NTFS)
npm run build                         PASS (strict tsc + Vite)
tauri CLI --version                   PASS (2.11.4)
native transport tests                PASS (cargo check; cargo test; 6 Rust tests)
tauri info                            PASS (Windows Rust/MSVC/SDK environment)
```

The Rust unit tests and the full Node checks pass in the NTFS verification
copy. `npm run tauri dev` reaches the Vite dev server and a responding
`StudyGuardian Pet v3` Windows process after the Rust executable starts. The
current CUA surface did not enumerate that native window, so visual/UI
interaction acceptance remains a manual Windows check; no visual PASS is
claimed from process liveness alone.

The Pet v3 Interaction Fix Pack is coded and automatically verified. The
production panel opens on a click gesture and submits mode changes through
Rust to the real Supervisor; a successful write is followed by an immediate
snapshot poll, so Supervisor remains the only mode source of truth. Windows
visual/click/drag/mode acceptance remains PENDING because the current CUA
surface did not enumerate the native window. Study Center and Review also
remain PENDING because the existing safe entry is owned by the legacy PyQt
Pet; no second Review UI was introduced.

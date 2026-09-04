# Pet 3.0 P1 Foundation

`pet-v3/` is an isolated desktop-shell foundation. The production PyQt Pet,
the Supervisor API, `scripts/build-windows.sh`, `scripts/deploy-windows.sh`,
`start-all.ps1`, and the old `pet/` directory are unchanged.

## P1 scope delivered

- Tauri 2 / Rust shell with a transparent, undecorated, always-on-top,
  non-resizable 220px window bounded to 160–260px;
- explicit native left-button `startDragging()` handling for the pet, state,
  and task surfaces, with interactive-control exclusion and a tray menu;
- transparent CSS/reset and native `shadow: false`/`set_shadow(false)`;
- development mock controls gated behind `DEV + VITE_PET_DEV_PANEL=1`;
- reversible click-through command, with the tray menu as the recovery path;
- exact `CurrentActivityView` TypeScript contract and local mock controls;
  transport `connected` is separate from semantic `fresh`;
- native `supervisor_snapshot` command that reads the existing runtime token,
  calls localhost Supervisor `/v1/activity/current`, validates the bounded
  response, and exposes only the sanitized contract plus a bounded error kind;
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
npm ci                                PASS
npm test                              PASS (15 tests)
npm run build                         PASS (strict tsc + Vite)
tauri CLI --version                   PASS (2.11.4)
native supervisor_snapshot tests      PASS (cargo check/test; 5 Rust tests)
tauri info                            PASS (Windows Rust/MSVC/SDK environment)
```

The Rust unit tests and the full Node checks pass in the NTFS verification
copy. `npm run tauri dev` reaches the Vite dev server and a responding
`StudyGuardian Pet v3` Windows process after the Rust executable starts. The
current CUA surface did not enumerate that native window, so visual/UI
interaction acceptance remains a manual Windows check; no visual PASS is
claimed from process liveness alone.

The current Fix Pack is coded and automatically verified. Study Center and
Review remain PENDING because the existing safe entry is owned by the legacy
PyQt Pet; no second Review UI was introduced.

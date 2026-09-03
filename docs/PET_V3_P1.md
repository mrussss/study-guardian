# Pet 3.0 P1 Foundation

`pet-v3/` is an isolated desktop-shell foundation. The production PyQt Pet,
the Supervisor API, `scripts/build-windows.sh`, `scripts/deploy-windows.sh`,
`start-all.ps1`, and the old `pet/` directory are unchanged.

## P1 scope delivered

- Tauri 2 / Rust shell with a transparent, undecorated, always-on-top,
  non-resizable 220px window bounded to 160–260px;
- draggable window region and tray menu;
- reversible click-through command, with the tray menu as the recovery path;
- exact `CurrentActivityView` TypeScript contract and local mock controls;
  transport `connected` is separate from semantic `fresh`;
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

No real Supervisor HTTP, main token, ActivityWatch, Sensor, Text AI, or Vision
AI is used in the Pet dev panel. Real transport and production integration
remain later scope items.

## Verification

```text
npm ci                                PASS
npm test                              PASS (updated count in final report)
npm run build                         PASS (strict tsc + Vite)
tauri CLI --version                   PASS (2.11.4)
tauri info                            BLOCKED: rustc/Cargo and MSVC/SDK absent
```

The Tauri startup smoke was not run because this Windows machine has WebView2
and Node but no Rust toolchain or Visual Studio Build Tools. No startup PASS
is claimed until those dependencies are installed.

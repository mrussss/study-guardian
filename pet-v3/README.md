# StudyGuardian Pet 3.0 — P1 Foundation

This directory is an isolated Tauri 2 / Rust / Vite / TypeScript foundation
for the next Pet. The production PyQt Pet in `../pet/` is unchanged.

P1 deliberately uses `src/mock/semantic.ts` and does not call Supervisor,
ActivityWatch, Sensor, Text AI, or Vision AI. The mock has the same fields as
Supervisor's `CurrentActivityView`, so the behavior and animation layers can
be tested before the real transport is introduced in a later phase.

The desktop shell is configured as a transparent, undecorated, always-on-top,
non-resizable 220px window bounded to 160–260px. It exposes a tray menu for
the reversible click-through toggle; the tray remains the recovery path when
the window is click-through.

## Local verification

From this directory:

```text
npm install
npm test
npm run build
npm run tauri dev   # requires Rust/Cargo and Windows WebView2
```

The current development machine has Node/npm but no Rust/Cargo, so the first
three commands are the available P1 checks and the Tauri startup check must
be run after the Rust toolchain is installed.

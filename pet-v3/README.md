# StudyGuardian Pet 3.0 — P1 Foundation

This directory is an isolated Tauri 2 / Rust / Vite / TypeScript foundation
for the next Pet. The production PyQt Pet in `../pet/` is unchanged.

The development panel still exposes `src/mock/semantic.ts` controls, but the
native Tauri shell now also exposes `supervisor_snapshot`. That command reads
the existing runtime `config/auth.token`, calls the localhost Supervisor
`/v1/activity/current` endpoint, and returns only the sanitized semantic
contract. ActivityWatch, Sensor, Text AI, and Vision AI remain behind
Supervisor and are not called by the Pet.

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

The Windows toolchain is now available. In an NTFS verification copy,
`npm ci`, `npm test`, `npm run build`, `cargo check`, and `cargo test` pass;
`npm run tauri dev` also reaches a responding `StudyGuardian Pet v3` process.
The CUA surface used in this environment did not enumerate the native window,
so visual/UI interaction acceptance remains a manual Windows check.

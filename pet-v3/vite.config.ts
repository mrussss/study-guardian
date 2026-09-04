import { defineConfig } from "vite";

export default defineConfig({
  clearScreen: false,
  server: {
    port: 1420,
    strictPort: true,
    // Tauri locks the Windows DLLs under src-tauri/target while running.
    // Keeping that build tree out of Vite's watcher makes `tauri dev`
    // reliable on NTFS without weakening source-file HMR.
    watch: { ignored: ["**/src-tauri/**"] },
  },
  envPrefix: ["VITE_", "TAURI_"],
  build: { target: "es2021", sourcemap: false },
});

import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import { resolve } from "node:path";

export default defineConfig({
  plugins: [react()],
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
  build: {
    target: "es2021",
    sourcemap: false,
    rollupOptions: {
      input: {
        pet: resolve(__dirname, "index.html"),
        quickPanel: resolve(__dirname, "quick-panel.html"),
        controlCenter: resolve(__dirname, "control-center.html"),
      },
    },
  },
});

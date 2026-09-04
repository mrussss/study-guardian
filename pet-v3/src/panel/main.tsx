import { createRoot } from "react-dom/client";
import { invoke } from "@tauri-apps/api/core";
import { QuickPanel } from "./QuickPanel";
import "../shared/theme/tokens.css";
import "./panel.css";

const root = document.querySelector<HTMLElement>("#quick-panel");
if (!root) throw new Error("Quick Panel root is missing");

const isTauriRuntime = typeof window !== "undefined"
  && Object.prototype.hasOwnProperty.call(window, "__TAURI_INTERNALS__");
const invokeWindowCommand = (command: string): void => {
  if (isTauriRuntime) void invoke(command).catch(() => { /* bounded window command failure */ });
};
window.addEventListener("keydown", event => {
  if (event.key === "Escape") invokeWindowCommand("hide_quick_panel");
});

createRoot(root).render(<QuickPanel onOpenCenter={() => invokeWindowCommand("open_control_center")} onOpenSettings={() => invokeWindowCommand("open_control_center")} />);

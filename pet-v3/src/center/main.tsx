import { createRoot } from "react-dom/client";
import { ControlCenter } from "./App";
import "../shared/theme/tokens.css";
import "./center.css";

const root = document.querySelector<HTMLElement>("#control-center");
if (!root) throw new Error("Control Center root is missing");

createRoot(root).render(<ControlCenter />);

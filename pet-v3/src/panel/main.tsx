import { createRoot } from "react-dom/client";
import { QuickPanel } from "./QuickPanel";
import "../shared/theme/tokens.css";
import "./panel.css";

const root = document.querySelector<HTMLElement>("#quick-panel");
if (!root) throw new Error("Quick Panel root is missing");

createRoot(root).render(<QuickPanel />);

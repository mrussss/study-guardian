import "./ui/style.css";
import { mountApp } from "./ui/app";

const root = document.querySelector<HTMLElement>("#app");
if (!root) throw new Error("Pet root element is missing");
mountApp(root);

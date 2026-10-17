import { createRoot } from "react-dom/client";
import { ReminderApp } from "./ReminderApp";
import "./reminder.css";

createRoot(document.getElementById("eye-care-reminder")!).render(<ReminderApp />);

import { useEffect, useState, type ReactElement } from "react";
import { createRoot } from "react-dom/client";
import { ControlCenter } from "./App";
import { NativeSupervisorDashboardAdapter, type SupervisorDashboardSnapshot } from "../transport/supervisor";
import "../shared/theme/tokens.css";
import "./center.css";

const root = document.querySelector<HTMLElement>("#control-center");
if (!root) throw new Error("Control Center root is missing");

const isTauriRuntime = typeof window !== "undefined"
  && Object.prototype.hasOwnProperty.call(window, "__TAURI_INTERNALS__");

function RuntimeControlCenter(): ReactElement {
  const [snapshot, setSnapshot] = useState<SupervisorDashboardSnapshot>();

  useEffect(() => {
    let stopped = false;
    let inFlight = false;
    const adapter = new NativeSupervisorDashboardAdapter();
    const poll = async (): Promise<void> => {
      if (stopped || inFlight) return;
      inFlight = true;
      try {
        const next = await adapter.poll();
        if (!stopped) setSnapshot(next);
      } finally {
        inFlight = false;
      }
    };
    void poll();
    const timer = window.setInterval(() => void poll(), 2500);
    return () => {
      stopped = true;
      window.clearInterval(timer);
    };
  }, []);

  return <ControlCenter snapshot={snapshot} live />;
}

createRoot(root).render(isTauriRuntime ? <RuntimeControlCenter /> : <ControlCenter />);

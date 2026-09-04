import { useEffect, useState, type ReactElement } from "react";
import { createRoot } from "react-dom/client";
import { listen } from "@tauri-apps/api/event";
import { invoke } from "@tauri-apps/api/core";
import { ControlCenter } from "./App";
import { CONTROL_CENTER_ROUTE_EVENT, isControlCenterRoute, type ControlCenterRoute } from "./route";
import { NativeSupervisorDashboardAdapter, type SupervisorDashboardSnapshot } from "../transport/supervisor";
import "../shared/theme/tokens.css";
import "./center.css";

const root = document.querySelector<HTMLElement>("#control-center");
if (!root) throw new Error("Control Center root is missing");

const isTauriRuntime = typeof window !== "undefined"
  && Object.prototype.hasOwnProperty.call(window, "__TAURI_INTERNALS__");

function RuntimeControlCenter(): ReactElement {
  const [snapshot, setSnapshot] = useState<SupervisorDashboardSnapshot>();
  const [route, setRoute] = useState<ControlCenterRoute>("overview");

  useEffect(() => {
    let stopped = false;
    let unlisten: (() => void) | undefined;
    const loadRoute = async (): Promise<void> => {
      try {
        const next = await invoke<unknown>("control_center_route");
        if (!stopped && isControlCenterRoute(next)) setRoute(next);
      } catch {
        // Browser/older native builds can keep the overview fallback.
      }
    };
    void loadRoute();
    void listen<unknown>(CONTROL_CENTER_ROUTE_EVENT, event => {
      if (isControlCenterRoute(event.payload)) setRoute(event.payload);
    }).then(nextUnlisten => {
      if (stopped) nextUnlisten();
      else unlisten = nextUnlisten;
    }).catch(() => {
      // Route state is still recovered through the native command on mount.
    });
    return () => {
      stopped = true;
      unlisten?.();
    };
  }, []);

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

  return <ControlCenter snapshot={snapshot} live initialActive={route} />;
}

createRoot(root).render(isTauriRuntime ? <RuntimeControlCenter /> : <ControlCenter />);

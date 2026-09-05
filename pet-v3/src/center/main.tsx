import { useEffect, useState, type ReactElement } from "react";
import { createRoot } from "react-dom/client";
import { listen } from "@tauri-apps/api/event";
import { invoke } from "@tauri-apps/api/core";
import { ControlCenter } from "./App";
import { applyControlCenterRouteRequest, CONTROL_CENTER_ROUTE_EVENT, isControlCenterRoute, type ControlCenterRouteRequest } from "./route";
import { NativeSupervisorDashboardAdapter, type SupervisorDashboardSnapshot } from "../transport/supervisor";
import "../shared/theme/tokens.css";
import "../shared/task-picker.css";
import "./center.css";

const root = document.querySelector<HTMLElement>("#control-center");
if (!root) throw new Error("Control Center root is missing");

const isTauriRuntime = typeof window !== "undefined"
  && Object.prototype.hasOwnProperty.call(window, "__TAURI_INTERNALS__");

function RuntimeControlCenter(): ReactElement {
  const [snapshot, setSnapshot] = useState<SupervisorDashboardSnapshot>();
  const [routeRequest, setRouteRequest] = useState<ControlCenterRouteRequest>({ route: "overview", revision: 0 });

  useEffect(() => {
    let stopped = false;
    let unlisten: (() => void) | undefined;
    let routeRevision = 0;
    const subscribeAndLoadRoute = async (): Promise<void> => {
      try {
        const nextUnlisten = await listen<unknown>(CONTROL_CENTER_ROUTE_EVENT, event => {
          if (!stopped && isControlCenterRoute(event.payload)) {
            routeRevision += 1;
            setRouteRequest(current => applyControlCenterRouteRequest(current, event.payload));
          }
        });
        if (stopped) {
          nextUnlisten();
          return;
        }
        unlisten = nextUnlisten;
      } catch {
        // Older native builds can still recover the route through the query.
      }
      if (stopped) return;
      const revisionBeforeRead = routeRevision;
      try {
        const next = await invoke<unknown>("control_center_route");
        // A route event received during this query is newer than its snapshot.
        if (!stopped && routeRevision === revisionBeforeRead) {
          setRouteRequest(current => applyControlCenterRouteRequest(current, next));
        }
      } catch {
        // Keep the latest event route or the overview fallback.
      }
    };
    void subscribeAndLoadRoute();
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

  return <ControlCenter snapshot={snapshot} live initialActive={routeRequest.route} routeRevision={routeRequest.revision} />;
}

createRoot(root).render(isTauriRuntime ? <RuntimeControlCenter /> : <ControlCenter />);

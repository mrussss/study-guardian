import { useEffect, useRef, useState, type ReactElement } from "react";
import { createRoot } from "react-dom/client";
import { listen } from "@tauri-apps/api/event";
import { invoke } from "@tauri-apps/api/core";
import { ControlCenter } from "./App";
import { applyControlCenterRouteRequest, CONTROL_CENTER_ROUTE_EVENT, isControlCenterRoute, type ControlCenterRouteRequest } from "./route";
import { SupervisorDashboardPollLoop, type SupervisorDashboardSnapshot } from "../transport/supervisor";
import { getMockScenario, getSupervisorDashboardAdapter, isTauriRuntime } from "../runtime/adapters";
import { MockScenarioToolbar } from "../mock/MockScenarioToolbar";
import "../shared/theme/tokens.css";
import "../shared/task-picker.css";
import "../shared/task-wheel/task-wheel.css";
import "../shared/help-drawer.css";
import "./center.css";
import "./focus-clock.css";

const root = document.querySelector<HTMLElement>("#control-center");
if (!root) throw new Error("Control Center root is missing");

function RuntimeControlCenter(): ReactElement {
  const [snapshot, setSnapshot] = useState<SupervisorDashboardSnapshot>();
  const [routeRequest, setRouteRequest] = useState<ControlCenterRouteRequest>({ route: "overview", revision: 0 });
  const pollerRef = useRef<SupervisorDashboardPollLoop | undefined>(undefined);

  useEffect(() => {
    if (!isTauriRuntime) {
      const requested = new URLSearchParams(window.location.search).get("route");
      if (isControlCenterRoute(requested)) setRouteRequest({ route: requested, revision: 1 });
      return;
    }
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
    const adapter = getSupervisorDashboardAdapter();
    const poller = new SupervisorDashboardPollLoop(adapter, 2500);
    pollerRef.current = poller;
    poller.start(next => { if (!stopped) setSnapshot(next); });
    return () => {
      stopped = true;
      poller.stop();
      pollerRef.current = undefined;
    };
  }, []);

  return <ControlCenter snapshot={snapshot} live initialActive={routeRequest.route} routeRevision={routeRequest.revision}
    onTaskChanged={() => pollerRef.current?.refresh()}
    onTaskMutationStarted={() => pollerRef.current?.markMutation()}
    onRefresh={() => pollerRef.current?.refresh() ?? Promise.resolve()}
  />;
}

const mockScenario = getMockScenario();
createRoot(root).render(<>{mockScenario && <MockScenarioToolbar scenario={mockScenario} />}<RuntimeControlCenter /></>);

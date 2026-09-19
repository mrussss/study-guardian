import { JSDOM } from "jsdom";
import { strict as assert } from "node:assert";
import test, { afterEach } from "node:test";
import { Dashboard } from "./App";
import type { NativeEyeCareSettings, NativeEyeCareStatus, NativeSupervisorStatus, SupervisorDashboardSnapshot } from "../transport/supervisor";
import { getSupervisorControlAdapter } from "../runtime/adapters";

const dom = new JSDOM("<!doctype html><html><body></body></html>", { url: "http://localhost" });
Object.defineProperty(globalThis, "window", { value: dom.window, configurable: true });
Object.defineProperty(globalThis, "document", { value: dom.window.document, configurable: true });
Object.defineProperty(globalThis, "navigator", { value: dom.window.navigator, configurable: true });
Object.defineProperty(globalThis, "HTMLElement", { value: dom.window.HTMLElement, configurable: true });
Object.defineProperty(globalThis, "Node", { value: dom.window.Node, configurable: true });
Object.defineProperty(globalThis, "MutationObserver", { value: dom.window.MutationObserver, configurable: true });
Object.defineProperty(globalThis, "getComputedStyle", { value: dom.window.getComputedStyle.bind(dom.window), configurable: true });
(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
const { cleanup, fireEvent, render, screen, waitFor } = await import("@testing-library/react");
afterEach(() => cleanup());

const status: NativeSupervisorStatus = {
  user_mode: "BREAK", interaction_state: "ACTIVE", task_relation: "UNKNOWN", privacy_state: "NORMAL",
  confidence: 1, task: "Go", study_seconds: 2400, break_seconds: 30, active_seconds: 2400,
  activitywatch_ok: true, screen_sensor_ok: true, mode_origin: "EYE_CARE", pause_reason: "EYE_CARE_SHORT",
};
const settings: NativeEyeCareSettings = {
  enabled: true, focus_minutes: 40, short_break_minutes: 5, long_break_after_focus_minutes: 120,
  long_break_minutes: 20, snooze_minutes: 5, max_snoozes: 2,
};
const eyeStatus: NativeEyeCareStatus = {
  enabled: true, phase: "WAITING_RETURN", local_date: "2026-09-13", focus_segment_seconds: 0,
  focus_since_long_break_seconds: 2400, snooze_count: 0, completed_short_breaks: 1,
  completed_long_breaks: 0, retry_focus_after_seconds: 0, revision: 9,
  updated_at: "2026-09-13T10:00:00Z", notification_suppressed: false,
};

test("Dashboard prominent continue action uses RESUME_STUDY for eye-care waiting return", async () => {
  let eyeAction = "";
  let regularStudyRequests = 0;
  const control = {
    eyeCareAction: async (action: string) => { eyeAction = action; return { ok: true }; },
    setModeStudy: async () => { regularStudyRequests++; return { ok: true }; },
  } as unknown as ReturnType<typeof getSupervisorControlAdapter>;
  const snapshot: SupervisorDashboardSnapshot = { connected: true, status, eye_care_settings: settings, eye_care_status: eyeStatus };
  const view = render(<Dashboard live snapshot={snapshot} control={control} onRefresh={async () => snapshot} />);
  const button = view.container.querySelector<HTMLButtonElement>(".hero-actions .primary-button");
  assert.ok(button);
  assert.equal(button.textContent, "继续学习");
  fireEvent.click(button);
  await waitFor(() => assert.equal(eyeAction, "RESUME_STUDY"));
  assert.equal(regularStudyRequests, 0);
  await waitFor(() => assert.ok(screen.queryByText("已继续学习")));
});

test("Dashboard labels and routes an active eye-care countdown through FINISH_EARLY", async () => {
  let eyeAction = "";
  const control = {
    eyeCareAction: async (action: string) => { eyeAction = action; return { ok: true }; },
  } as unknown as ReturnType<typeof getSupervisorControlAdapter>;
  const snapshot: SupervisorDashboardSnapshot = {
    connected: true, status,
    eye_care_settings: settings,
    eye_care_status: { ...eyeStatus, phase: "SHORT_BREAK", break_started_at: "2026-09-13T10:00:00Z", planned_break_end_at: "2026-09-13T10:05:00Z" },
  };
  const view = render(<Dashboard live snapshot={snapshot} control={control} onRefresh={async () => snapshot} />);
  const button = view.container.querySelector<HTMLButtonElement>(".hero-actions .primary-button");
  assert.ok(button);
  assert.equal(button.textContent, "提前结束并继续");
  fireEvent.click(button);
  await waitFor(() => assert.equal(eyeAction, "FINISH_EARLY"));
});

test("Dashboard rapid continue clicks submit only one eye-care action", async () => {
  let actionCount = 0;
  let releaseAction!: (result: { ok: boolean }) => void;
  const pendingAction = new Promise<{ ok: boolean }>(resolve => { releaseAction = resolve; });
  const control = {
    eyeCareAction: async () => { actionCount++; return pendingAction; },
  } as unknown as ReturnType<typeof getSupervisorControlAdapter>;
  const snapshot: SupervisorDashboardSnapshot = { connected: true, status, eye_care_settings: settings, eye_care_status: eyeStatus };
  const view = render(<Dashboard live snapshot={snapshot} control={control} onRefresh={async () => snapshot} />);
  const button = view.container.querySelector<HTMLButtonElement>(".hero-actions .primary-button");
  assert.ok(button);
  fireEvent.click(button);
  fireEvent.click(button);
  assert.equal(actionCount, 1);
  releaseAction({ ok: true });
  await waitFor(() => assert.ok(screen.queryByText("已继续学习")));
});

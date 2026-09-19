import { JSDOM } from "jsdom";
import { strict as assert } from "node:assert";
import test, { afterEach } from "node:test";
import { QuickPanel } from "./QuickPanel";
import type { NativeEyeCareStatus, NativeSupervisorStatus } from "../transport/supervisor";

const dom = new JSDOM("<!doctype html><html><body></body></html>", { url: "http://localhost" });
Object.defineProperty(globalThis, "window", { value: dom.window, configurable: true });
Object.defineProperty(globalThis, "document", { value: dom.window.document, configurable: true });
Object.defineProperty(globalThis, "navigator", { value: dom.window.navigator, configurable: true });
Object.defineProperty(globalThis, "HTMLElement", { value: dom.window.HTMLElement, configurable: true });
Object.defineProperty(globalThis, "Node", { value: dom.window.Node, configurable: true });
Object.defineProperty(globalThis, "MutationObserver", { value: dom.window.MutationObserver, configurable: true });
(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
const { cleanup, fireEvent, render, screen } = await import("@testing-library/react");
afterEach(() => cleanup());

const row = (origin: "EYE_CARE" | "MANUAL"): NativeSupervisorStatus => ({
  user_mode: "BREAK", interaction_state: "ACTIVE", task_relation: "UNKNOWN", privacy_state: "NORMAL",
  confidence: 1, task: "Go", study_seconds: 0, break_seconds: 30, active_seconds: 30, afk_seconds: 0,
  activitywatch_ok: true, screen_sensor_ok: true, mode_origin: origin,
});
const eyeStatus = (phase: NativeEyeCareStatus["phase"]): NativeEyeCareStatus => ({
  enabled: true, phase, local_date: "2026-09-13", focus_segment_seconds: 0,
  focus_since_long_break_seconds: 0, snooze_count: 0, completed_short_breaks: 0,
  completed_long_breaks: 0, retry_focus_after_seconds: 0, revision: 7,
  updated_at: "2026-09-13T10:00:00Z", notification_suppressed: false,
});

test("Quick Panel continue routes WAITING_RETURN through RESUME_STUDY", () => {
  let eyeAction = "";
  let regularAction = "";
  render(<QuickPanel mode="BREAK" connected status={row("EYE_CARE")} eyeCareStatus={eyeStatus("WAITING_RETURN")}
    onEyeCareResumeAction={action => { eyeAction = action; }} onModeAction={mode => { regularAction = mode; }} />);
  fireEvent.click(screen.getByRole("button", { name: /继续学习/ }));
  assert.equal(eyeAction, "RESUME_STUDY");
  assert.equal(regularAction, "");
});

test("Quick Panel active eye-care break routes through FINISH_EARLY", () => {
  let eyeAction = "";
  render(<QuickPanel mode="BREAK" connected status={row("EYE_CARE")} eyeCareStatus={eyeStatus("LONG_BREAK")}
    onEyeCareResumeAction={action => { eyeAction = action; }} />);
  const button = screen.getByRole("button", { name: /提前结束并继续/ });
  fireEvent.click(button);
  assert.equal(eyeAction, "FINISH_EARLY");
});

test("Quick Panel manual break still uses the normal study mode action", () => {
  let eyeAction = "";
  let regularAction = "";
  render(<QuickPanel mode="BREAK" connected status={row("MANUAL")} eyeCareStatus={eyeStatus("WAITING_RETURN")}
    onEyeCareResumeAction={action => { eyeAction = action; }} onModeAction={mode => { regularAction = mode; }} />);
  fireEvent.click(screen.getByRole("button", { name: /继续学习/ }));
  assert.equal(eyeAction, "");
  assert.equal(regularAction, "STUDY");
});

test("Quick Panel rapid continue clicks submit only one eye-care action", async () => {
  let actionCount = 0;
  let releaseAction!: () => void;
  const pendingAction = new Promise<void>(resolve => { releaseAction = resolve; });
  render(<QuickPanel mode="BREAK" connected status={row("EYE_CARE")} eyeCareStatus={eyeStatus("WAITING_RETURN")}
    onEyeCareResumeAction={async () => { actionCount++; await pendingAction; }} />);
  const button = screen.getByRole("button", { name: /继续学习/ });
  fireEvent.click(button);
  fireEvent.click(button);
  assert.equal(actionCount, 1);
  releaseAction();
  await pendingAction;
});

test("Quick Panel shows bounded automatic-start grace progress", () => {
  const standby: NativeSupervisorStatus = {
    ...row("MANUAL"),
    user_mode: "STANDBY",
    automation_diagnostics: {
      state: "GRACE", signal_kind: "STRONG_FOCUS", accumulated_seconds: 42,
      required_seconds: 90, grace_remaining_seconds: 8, blocker: "", updated_at: "2026-09-19T10:00:00Z",
    },
  };
  render(<QuickPanel mode="STANDBY" connected status={standby} />);
  assert.ok(screen.getByText("自动开始：短暂切换中，保留进度（42 / 90 秒，容错剩余 8 秒）"));
  assert.equal(screen.queryByText(/0 \/ 0 秒/), null);
});

test("Quick Panel explains a manual override", () => {
  const standby: NativeSupervisorStatus = {
    ...row("MANUAL"),
    user_mode: "STANDBY",
    automation_diagnostics: {
      state: "BLOCKED", signal_kind: "", accumulated_seconds: 0,
      required_seconds: 90, grace_remaining_seconds: 20, blocker: "MANUAL_OVERRIDE",
      manual_override_until: "2026-09-19T10:30:00Z", updated_at: "2026-09-19T10:00:00Z",
    },
  };
  render(<QuickPanel mode="STANDBY" connected status={standby} />);
  assert.ok(screen.getByText("自动开始暂不可用：手动操作暂时覆盖自动开始"));
});

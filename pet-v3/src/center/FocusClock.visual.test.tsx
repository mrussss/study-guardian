import { JSDOM } from "jsdom";
import { strict as assert } from "node:assert";
import test, { afterEach } from "node:test";
import { FocusClock } from "./FocusClock";
import type { NativeSupervisorStatus } from "../transport/supervisor";

const dom = new JSDOM("<!doctype html><html><body></body></html>", { url: "http://localhost" });
Object.defineProperty(globalThis, "window", { value: dom.window, configurable: true });
Object.defineProperty(globalThis, "document", { value: dom.window.document, configurable: true });
Object.defineProperty(globalThis, "navigator", { value: dom.window.navigator, configurable: true });
Object.defineProperty(globalThis, "HTMLElement", { value: dom.window.HTMLElement, configurable: true });
Object.defineProperty(globalThis, "Node", { value: dom.window.Node, configurable: true });
Object.defineProperty(globalThis, "MutationObserver", { value: dom.window.MutationObserver, configurable: true });
Object.defineProperty(globalThis, "getComputedStyle", { value: dom.window.getComputedStyle.bind(dom.window), configurable: true });
(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

const { cleanup, render, screen } = await import("@testing-library/react");

afterEach(() => cleanup());

const status: NativeSupervisorStatus = {
  user_mode: "STUDY", interaction_state: "ACTIVE", task_relation: "FOCUSED", privacy_state: "NORMAL",
  confidence: 1, task: "Go", study_seconds: 545, break_seconds: 0, active_seconds: 545,
  activitywatch_ok: true, screen_sensor_ok: true,
};

test("focus clock keeps only time and daily total in its visible copy", () => {
  render(<FocusClock connected status={status} motivation={{
    today_credited_focus_minutes: 127,
    total_credited_focus_minutes: 127,
    today_earned_ap_milli: 0,
    today_spent_ap_milli: 0,
    balance_ap_milli: 0,
    checkin_completed: false,
    daily_target_minutes: 120,
    target_progress: 1,
    streak_days: 1,
  }} />);
  assert.ok(screen.getByLabelText("专注秒表，00:09:05，运行中"));
  assert.ok(screen.getByText("本次专注"));
  assert.ok(screen.getByText("今日累计"));
  assert.equal(screen.queryByText("正在记录有效专注"), null);
  assert.equal(screen.queryByText("秒表已暂停"), null);
});

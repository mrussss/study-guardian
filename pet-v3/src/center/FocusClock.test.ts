import assert from "node:assert/strict";
import test from "node:test";
import { formatSessionClock, getAnchoredDisplaySeconds, isEffectiveFocus, secondProgress, syncFocusClockAnchor } from "./FocusClock";
import type { NativeSupervisorStatus } from "../transport/supervisor";

const activeStatus: NativeSupervisorStatus = {
  user_mode: "STUDY", interaction_state: "ACTIVE", task_relation: "FOCUSED", privacy_state: "NORMAL",
  confidence: 1, task: "Go", study_seconds: 545, break_seconds: 0, active_seconds: 545,
  activitywatch_ok: true, screen_sensor_ok: true,
};

test("session clock always uses a stable two-digit hour format", () => {
  assert.equal(formatSessionClock(0), "00:00:00");
  assert.equal(formatSessionClock(545), "00:09:05");
  assert.equal(formatSessionClock(7238), "02:00:38");
});

test("effective focus is the only state that locally advances the stopwatch", () => {
  assert.equal(isEffectiveFocus(true, activeStatus), true);
  assert.equal(isEffectiveFocus(true, { ...activeStatus, user_mode: "BREAK" }), false);
  assert.equal(isEffectiveFocus(false, activeStatus), false);
  const paused = syncFocusClockAnchor(undefined, 545, false, 1000);
  assert.equal(getAnchoredDisplaySeconds(paused, 9000), 545);
});

test("second progress completes at 59 seconds and wraps on the next minute", () => {
  assert.equal(secondProgress(30), .5);
  assert.equal(secondProgress(59), 59 / 60);
  assert.equal(secondProgress(60), 0);
});

test("snapshot anchors advance locally and small corrections never make time go backwards", () => {
  const first = syncFocusClockAnchor(undefined, 100, true, 0);
  assert.equal(getAnchoredDisplaySeconds(first, 2500), 102.5);
  const recalibrated = syncFocusClockAnchor(first, 101, true, 2500);
  assert.ok(getAnchoredDisplaySeconds(recalibrated, 2500) >= getAnchoredDisplaySeconds(first, 2500));
  assert.ok(getAnchoredDisplaySeconds(recalibrated, 3500) >= getAnchoredDisplaySeconds(recalibrated, 2500));
  const largeCorrection = syncFocusClockAnchor(first, 300, true, 2500);
  assert.equal(getAnchoredDisplaySeconds(largeCorrection, 2500), 300);
});

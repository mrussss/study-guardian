import { strict as assert } from "node:assert";
import test from "node:test";
import { automationPauseStatusText } from "./AutomationPauseStatus";
import type { NativeAutomationSettings, NativeSupervisorStatus } from "../transport/supervisor";

const settings: NativeAutomationSettings = {
  enabled: true,
  auto_start: { enabled: true, focused_stable_seconds: 90, min_confidence: .8, allow_unclassified: true, confirm: false },
  auto_pause: { enabled: true, idle_static_seconds: 300, idle_dynamic_seconds: 900, locked_seconds: 15, confirm: false },
  auto_resume: { enabled: true, focused_stable_seconds: 45 },
  transition_cooldown_seconds: 30,
  manual_override_minutes: 30,
};
const status = (changes: Partial<NativeSupervisorStatus>): NativeSupervisorStatus => ({
  user_mode: "STUDY", interaction_state: "ACTIVE", task_relation: "FOCUSED", privacy_state: "NORMAL", confidence: .9,
  task: "Go", study_seconds: 100, break_seconds: 0, active_seconds: 100, activitywatch_ok: true, screen_sensor_ok: true,
  ...changes,
});

test("shows the remaining pause time for a dynamic AFK interval", () => {
  assert.equal(automationPauseStatusText(status({ interaction_state: "IDLE_DYNAMIC", afk_seconds: 642 }), settings), "将在 04:18 后自动暂停");
});

test("shows the active threshold and disabled reason without inventing a pause", () => {
  assert.equal(automationPauseStatusText(status({ interaction_state: "IDLE_STATIC", afk_seconds: 0 }), settings), "自动暂停等待中 · 静态屏幕阈值：5分钟");
  assert.equal(automationPauseStatusText(status({ interaction_state: "ACTIVE" }), { ...settings, enabled: false }), "自动暂停未启用");
});

test("does not duplicate a pending pause prompt", () => {
  assert.equal(automationPauseStatusText(status({ interaction_state: "IDLE_STATIC", afk_seconds: 300, pending_automation_intent: {
    intent_id: "intent-1", transition: "AUTO_PAUSE", reason: "IDLE", task: "Go", created_at: "2026-09-09T00:00:00Z", expires_at: "2026-09-09T00:00:05Z", requires_confirmation: true,
  } }), settings), undefined);
});

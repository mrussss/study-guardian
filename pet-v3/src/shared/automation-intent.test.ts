import { strict as assert } from "node:assert";
import test from "node:test";
import { automationDecisionNotice } from "./automation-intent";
import type { NativeAutomationIntent, SupervisorDashboardSnapshot } from "../transport/supervisor";

function intent(transition: NativeAutomationIntent["transition"]): NativeAutomationIntent {
  return {
    intent_id: "intent-race",
    transition,
    reason: transition === "AUTO_PAUSE" ? "IDLE" : "NONE",
    task: "Go",
    created_at: "2026-09-09T10:00:00.000Z",
    expires_at: "2026-09-09T10:00:15.000Z",
    requires_confirmation: true,
  };
}

function snapshot(mode: "STANDBY" | "STUDY" | "BREAK" | "OFF", pending?: NativeAutomationIntent): SupervisorDashboardSnapshot {
  return { connected: true, status: { user_mode: mode, pending_automation_intent: pending } as SupervisorDashboardSnapshot["status"] };
}

test("canonical BREAK state wins over a late failed AUTO_PAUSE decision", () => {
  const result = automationDecisionNotice(intent("AUTO_PAUSE"), false, { ok: false, error_kind: "rejected" }, snapshot("BREAK"));
  assert.equal(result, "已自动暂停");
});

test("canonical cleared AUTO_START intent is not shown as a failed response", () => {
  const result = automationDecisionNotice(intent("AUTO_START"), true, { ok: false, error_kind: "rejected" }, snapshot("STANDBY"));
  assert.equal(result, "开始请求已处理");
});

test("a failed decision remains an error while the same intent is still canonical", () => {
  const pending = intent("AUTO_PAUSE");
  const result = automationDecisionNotice(pending, true, { ok: false, error_kind: "rejected" }, snapshot("STUDY", pending));
  assert.equal(result, "自动转场响应失败");
});

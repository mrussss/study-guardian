import assert from "node:assert/strict";
import test from "node:test";
import { MockSupervisorRuntime, parseMockScenario } from "./supervisor";

test("mock scenarios parse only supported values", () => {
  assert.equal(parseMockScenario("?mock=slow"), "slow");
  assert.equal(parseMockScenario("?mock=offline"), "offline");
  assert.equal(parseMockScenario("?mock=sensor-failure"), "sensor-failure");
  assert.equal(parseMockScenario("?mock=activitywatch-failure"), "activitywatch-failure");
  assert.equal(parseMockScenario("?mock=unknown"), "normal");
});
test("normal mock mutates task, mode, target, and settings through production interfaces", async () => {
  const runtime = new MockSupervisorRuntime("normal");
  assert.deepEqual(await runtime.selectTaskPreset("go"), { ok: true, task: "Go" });
  assert.deepEqual(await runtime.setModeBreak(), { ok: true });
  assert.deepEqual(await runtime.setDailyTarget(90), { ok: true });
  assert.deepEqual(await runtime.setReminderSettings(15, [{ start: "12:00", end: "13:00" }]), { ok: true });
  const snapshot = await runtime.poll();
  assert.equal(snapshot.status?.task, "Go");
  assert.equal(snapshot.status?.user_mode, "BREAK");
  assert.equal(snapshot.motivation?.daily_target_minutes, 90);
  assert.equal(snapshot.reminder_settings?.cooldown_minutes, 15);
});
test("updating a preset moves it between pinned and recent collections", async () => {
  const runtime = new MockSupervisorRuntime("normal");
  assert.deepEqual(await runtime.updateTaskPreset("english", "英语听力", true, 1), { ok: true });
  const snapshot = await runtime.poll();
  assert.equal(snapshot.task_presets?.recent.some(item => item.id === "english"), false);
  assert.equal(snapshot.task_presets?.pinned.find(item => item.id === "english")?.name, "英语听力");
});
test("mock missions support zero-AP linking, completion, and cancellation", async () => {
  const runtime = new MockSupervisorRuntime("normal");
  assert.deepEqual(await runtime.createMission("复习 Go", "整理 context", undefined, "Go", "go"), { ok: true });
  let snapshot = await runtime.poll();
  const created = snapshot.missions?.find(item => item.title === "复习 Go");
  assert.ok(created);
  assert.equal(created.reward_milli_ap, 0);
  assert.equal(created.linked_task_name, "Go");
  assert.equal(created.linked_task_preset_id, "go");
  assert.equal(created.link_source, "MANUAL");
  assert.deepEqual(await runtime.completeMission(created.id), { ok: true });
  assert.deepEqual(await runtime.cancelMission("missing"), { ok: false, error_kind: "rejected" });
  snapshot = await runtime.poll();
  assert.equal(snapshot.missions?.find(item => item.id === created.id)?.status, "COMPLETED");
});
test("offline and failure scenarios return bounded failures", async () => {
  const offline = new MockSupervisorRuntime("offline");
  assert.deepEqual(await offline.poll(), { connected: false, last_error_kind: "unavailable" });
  assert.deepEqual(await offline.setTask("Go"), { ok: false, error_kind: "unavailable" });
  const failure = new MockSupervisorRuntime("failure");
  assert.deepEqual(await failure.setTask("Go"), { ok: false, error_kind: "rejected" });
  assert.equal((await failure.poll()).connected, true);
});
test("progress and reminder scenarios expose deterministic visual states", async () => {
  const empty = await new MockSupervisorRuntime("progress-empty").poll();
  const complete = await new MockSupervisorRuntime("progress-complete").poll();
  const reminder = await new MockSupervisorRuntime("reminder").poll();
  assert.equal(empty.motivation?.target_progress, 0);
  assert.equal(complete.motivation?.target_progress, 1);
  assert.equal(complete.motivation?.checkin_completed, true);
  assert.equal(reminder.status?.task_relation, "DISTRACTED");
  assert.equal(reminder.motivation?.last_event?.type, "DISTRACTION");
});

test("service failure scenarios expose independent health flags", async () => {
  const sensorFailure = await new MockSupervisorRuntime("sensor-failure").poll();
  const activityWatchFailure = await new MockSupervisorRuntime("activitywatch-failure").poll();
  assert.equal(sensorFailure.status?.screen_sensor_ok, false);
  assert.equal(sensorFailure.status?.activitywatch_ok, true);
  assert.equal(activityWatchFailure.status?.screen_sensor_ok, true);
  assert.equal(activityWatchFailure.status?.activitywatch_ok, false);
});

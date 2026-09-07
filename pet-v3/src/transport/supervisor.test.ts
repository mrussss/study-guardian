import { strict as assert } from "node:assert";
import test from "node:test";
import { mockSemantic } from "../mock/semantic";
import { NativeSupervisorControlAdapter, normalizeControlResult, normalizeNativeDashboardSnapshot, normalizeNativeSnapshot, SupervisorDashboardPollLoop, SupervisorPollLoop, type PetTransportSnapshot, type SupervisorAdapter, type SupervisorDashboardPollScheduler, type SupervisorDashboardSnapshot } from "./supervisor";

function deferred<T>() {
  let resolve!: (value: T | PromiseLike<T>) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise;
    reject = rejectPromise;
  });
  return { promise, resolve, reject };
}

async function flushMicrotasks(): Promise<void> {
  for (let index = 0; index < 8; index += 1) await Promise.resolve();
}

function createFakeScheduler(): SupervisorDashboardPollScheduler & { pendingCount(): number; runNext(): void } {
  let nextId = 0;
  const callbacks = new Map<number, () => void>();
  return {
    setTimeout(callback) {
      const id = ++nextId;
      callbacks.set(id, callback);
      return id as unknown as ReturnType<typeof setTimeout>;
    },
    clearTimeout(timer) {
      callbacks.delete(timer as unknown as number);
    },
    pendingCount() {
      return callbacks.size;
    },
    runNext() {
      const next = callbacks.entries().next();
      if (next.done) return;
      callbacks.delete(next.value[0]);
      next.value[1]();
    },
  };
}

test("control results expose only bounded success or error kinds", () => {
  assert.deepEqual(normalizeControlResult({ ok: true, token: "ignored", path: "ignored" }), { ok: true });
  assert.deepEqual(normalizeControlResult({ ok: false, error_kind: "rejected", detail: "ignored" }), { ok: false, error_kind: "rejected" });
  assert.deepEqual(normalizeControlResult({ ok: false, error_kind: "timeout" }), { ok: false, error_kind: "timeout" });
  assert.deepEqual(normalizeControlResult({ ok: false, error_kind: "raw-supervisor-error" }), { ok: false, error_kind: "invalid_response" });
  assert.deepEqual(normalizeControlResult({ ok: false, token: "never returned" }), { ok: false, error_kind: "invalid_response" });
  assert.deepEqual(normalizeControlResult(null), { ok: false, error_kind: "invalid_response" });
});

test("daily target control rejects values outside the typed range before native invoke", async () => {
  const control = new NativeSupervisorControlAdapter();
  assert.deepEqual(await control.setDailyTarget(0), { ok: false, error_kind: "rejected" });
  assert.deepEqual(await control.setDailyTarget(1441), { ok: false, error_kind: "rejected" });
});

test("native snapshot accepts only a complete sanitized semantic contract", () => {
  const semantic = mockSemantic({}, "2026-09-03T00:00:00.000Z");
  const snapshot = normalizeNativeSnapshot({ connected: true, semantic, last_success_at: "2026-09-03T00:00:01.000Z", secret: "must be ignored" });
  assert.deepEqual(snapshot, { connected: true, semantic, last_success_at: "2026-09-03T00:00:01.000Z" });
  assert.equal(normalizeNativeSnapshot({ connected: true, semantic: { ...semantic, confidence: 2 } }).connected, false);
  assert.equal(normalizeNativeSnapshot({ connected: false, last_error_kind: "unauthorized", token: "never returned" }).last_error_kind, "unauthorized");
  assert.equal(normalizeNativeSnapshot({ connected: false, last_error_kind: "token-value" }).last_error_kind, "unavailable");
});

test("dashboard snapshot accepts canonical data and drops invalid optional sections", () => {
  const status = {
    user_mode: "STUDY",
    interaction_state: "ACTIVE",
    task_relation: "FOCUSED",
    privacy_state: "NORMAL",
    confidence: 0.9,
    task: "Go context",
    study_seconds: 2520,
    break_seconds: 0,
    active_seconds: 2520,
    activitywatch_ok: true,
    screen_sensor_ok: true,
  } as const;
  const motivation = {
    today_credited_focus_minutes: 42,
    total_credited_focus_minutes: 420,
    today_earned_ap_milli: 700,
    today_spent_ap_milli: 0,
    balance_ap_milli: 7000,
    checkin_completed: true,
    daily_target_minutes: 120,
    target_progress: 0.35,
    streak_days: 5,
  } as const;
  const snapshot = normalizeNativeDashboardSnapshot({
    connected: true,
    status,
    motivation,
    history: [{ date: "2026-09-04", focus_minutes: 42, target_minutes: 120, checkin_completed: true, target_completed: false }],
    achievements: [{ achievement_id: "FIRST_30", name: "初次专注", description: "累计有效专注 30 分钟", progress: 1, unlocked: true }],
    missions: [{ id: "m-1", title: "Read", description: "", reward_milli_ap: 100, status: "OPEN", created_at: "2026-09-04T00:00:00Z" }],
    rewards: [{ id: "r-1", name: "Break", type: "TIME", cost_milli_ap: 100, description: "", enabled: true }],
    ai: { enabled: false, text_provider: "none", text_configured: false, vision_enabled: false },
    review: {
      schema_version: 1,
      date: "2026-09-04",
      headline: "完成了一小步",
      topics: [{ name: "Go", summary: "复习 context", confidence: 0.8, evidence_refs: ["private"] }],
      accomplishments: [{ text: "完成练习", confidence: 0.9, evidence_refs: ["private"] }],
      unfinished: ["整理笔记"],
      difficulties: [],
      behavior: { distraction_count: 1, largest_distraction_seconds: 30, average_recovery_seconds: 20 },
      tomorrow_priority: "继续练习",
      warnings: [],
      raw_chat: "must be ignored",
    },
    secret: "must be ignored",
  });
  assert.equal(snapshot.connected, true);
  assert.deepEqual(snapshot.status, status);
  assert.deepEqual(snapshot.motivation, motivation);
  assert.equal(snapshot.history?.length, 1);
  assert.equal("secret" in snapshot, false);
  assert.equal(snapshot.review?.topics[0].name, "Go");
  assert.equal("raw_chat" in (snapshot.review ?? {}), false);
  assert.equal("evidence_refs" in (snapshot.review?.topics[0] ?? {}), false);
  assert.equal(normalizeNativeDashboardSnapshot({ connected: true, status: { ...status, confidence: 2 } }).connected, false);
  assert.deepEqual(normalizeNativeDashboardSnapshot({ connected: true, status, missions: [{ ...snapshot.missions?.[0], status: "INVALID" }] }).missions, undefined);
});

test("legacy dashboard AI settings without proxy normalize to environment mode", () => {
  const endpoint = { enabled: false, provider: "none", model: "", fallback_models: [], base_url: "", api_key_configured: false, timeout_seconds: 6, json_mode: "auto" } as const;
  const snapshot = normalizeNativeDashboardSnapshot({
    connected: true,
    status: { user_mode: "STANDBY", interaction_state: "UNKNOWN", task_relation: "UNKNOWN", privacy_state: "NORMAL", confidence: 0, task: "", study_seconds: 0, break_seconds: 0, active_seconds: 0, activitywatch_ok: true, screen_sensor_ok: true },
    ai_settings: { enabled: false, min_confidence: 0.75, text: endpoint, vision: { ...endpoint, timeout_seconds: 8 } },
  });
  assert.deepEqual(snapshot.ai_settings?.proxy, { mode: "environment", url: "" });
});

test("poll loop prevents overlap and stops cleanly", async () => {
  let resolvePoll: (() => void) | undefined;
  let calls = 0;
  let active = 0;
  let maxActive = 0;
  const result: PetTransportSnapshot = normalizeNativeSnapshot({ connected: false });
  const adapter: SupervisorAdapter = {
    poll: async () => {
      calls += 1;
      active += 1;
      maxActive = Math.max(maxActive, active);
      await new Promise<void>(resolve => { resolvePoll = resolve; });
      active -= 1;
      return result;
    },
  };
  const loop = new SupervisorPollLoop(adapter, 1500);
  let snapshots = 0;
  loop.start(() => { snapshots += 1; });
  await new Promise(resolve => setTimeout(resolve, 5));
  assert.equal(loop.isInFlight(), true);
  assert.equal(calls, 1);
  resolvePoll?.();
  await new Promise(resolve => setTimeout(resolve, 5));
  loop.stop();
  await new Promise(resolve => setTimeout(resolve, 10));
  assert.equal(maxActive, 1);
  assert.equal(snapshots, 1);
  assert.equal(loop.isInFlight(), false);
});

test("dashboard refresh queues behind an in-flight poll and drops stale snapshots", async () => {
  let releaseFirst!: () => void;
  const first = new Promise<void>(resolve => { releaseFirst = resolve; });
  let calls = 0;
  const stale = { connected: true } as SupervisorDashboardSnapshot;
  const fresh = { connected: true } as SupervisorDashboardSnapshot;
  const snapshots: SupervisorDashboardSnapshot[] = [];
  const loop = new SupervisorDashboardPollLoop({
    poll: async () => {
      calls += 1;
      if (calls === 1) await first;
      return calls === 1 ? stale : fresh;
    },
  }, 1000);

  loop.start(snapshot => snapshots.push(snapshot));
  await Promise.resolve();
  assert.equal(loop.isInFlight(), true);
  loop.markMutation();
  let finished = false;
  const refresh = loop.refresh().then(() => { finished = true; });
  await Promise.resolve();
  assert.equal(calls, 1);
  assert.equal(finished, false);
  releaseFirst();
  await refresh;
  assert.equal(calls, 2);
  assert.deepEqual(snapshots, [fresh]);
  loop.stop();
});

test("dashboard coalesces repeated refreshes and resolves every waiter after one follow-up", async () => {
  const first = deferred<SupervisorDashboardSnapshot>();
  const second = deferred<SupervisorDashboardSnapshot>();
  const scheduler = createFakeScheduler();
  let calls = 0;
  const stale = { connected: true } as SupervisorDashboardSnapshot;
  const fresh = { connected: true } as SupervisorDashboardSnapshot;
  const snapshots: SupervisorDashboardSnapshot[] = [];
  const loop = new SupervisorDashboardPollLoop({
    poll: () => {
      calls += 1;
      return calls === 1 ? first.promise : second.promise;
    },
  }, 1000, scheduler);

  loop.start(snapshot => snapshots.push(snapshot));
  await flushMicrotasks();
  assert.equal(calls, 1);
  loop.markMutation();
  let firstWaiterDone = false;
  let secondWaiterDone = false;
  const refreshA = loop.refresh().then(() => { firstWaiterDone = true; });
  const refreshB = loop.refresh().then(() => { secondWaiterDone = true; });
  first.resolve(stale);
  await flushMicrotasks();
  assert.equal(calls, 2);
  assert.equal(firstWaiterDone, false);
  assert.equal(secondWaiterDone, false);
  second.resolve(fresh);
  await Promise.all([refreshA, refreshB]);
  assert.deepEqual(snapshots, [fresh]);
  assert.equal(calls, 2);
  loop.stop();
});

test("dashboard stop resolves refresh waiters and blocks stale publish or timer re-arm", async () => {
  const first = deferred<SupervisorDashboardSnapshot>();
  const scheduler = createFakeScheduler();
  let calls = 0;
  const snapshots: SupervisorDashboardSnapshot[] = [];
  const loop = new SupervisorDashboardPollLoop({
    poll: async () => {
      calls += 1;
      return first.promise;
    },
  }, 1000, scheduler);

  loop.start(snapshot => snapshots.push(snapshot));
  await flushMicrotasks();
  loop.markMutation();
  let refreshDone = false;
  const refresh = loop.refresh().then(() => { refreshDone = true; });
  loop.stop();
  await refresh;
  assert.equal(refreshDone, true);
  first.resolve({ connected: true } as SupervisorDashboardSnapshot);
  await flushMicrotasks();
  scheduler.runNext();
  await flushMicrotasks();
  assert.equal(calls, 1);
  assert.deepEqual(snapshots, []);
  assert.equal(scheduler.pendingCount(), 0);
});

test("dashboard recovers from a polling exception on the next manual refresh", async () => {
  const second = deferred<SupervisorDashboardSnapshot>();
  const scheduler = createFakeScheduler();
  const snapshots: SupervisorDashboardSnapshot[] = [];
  let calls = 0;
  const loop = new SupervisorDashboardPollLoop({
    poll: () => {
      calls += 1;
      if (calls === 1) throw new Error("temporary failure");
      return second.promise;
    },
  }, 1000, scheduler);

  loop.start(snapshot => snapshots.push(snapshot));
  await flushMicrotasks();
  assert.equal(calls, 1);
  const refresh = loop.refresh();
  await flushMicrotasks();
  assert.equal(calls, 2);
  second.resolve({ connected: true } as SupervisorDashboardSnapshot);
  await refresh;
  assert.equal(snapshots.length, 1);
  loop.stop();
});

test("dashboard start is idempotent and stop cancels the only scheduled timer", async () => {
  const scheduler = createFakeScheduler();
  let calls = 0;
  const loop = new SupervisorDashboardPollLoop({
    poll: async () => {
      calls += 1;
      return { connected: true } as SupervisorDashboardSnapshot;
    },
  }, 1000, scheduler);
  const snapshots: SupervisorDashboardSnapshot[] = [];

  loop.start(snapshot => snapshots.push(snapshot));
  loop.start(snapshot => snapshots.push(snapshot));
  await flushMicrotasks();
  assert.equal(calls, 1);
  assert.equal(scheduler.pendingCount(), 1);
  loop.stop();
  assert.equal(scheduler.pendingCount(), 0);
  scheduler.runNext();
  await flushMicrotasks();
  assert.equal(calls, 1);
  assert.equal(snapshots.length, 1);
});

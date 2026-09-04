import { strict as assert } from "node:assert";
import test from "node:test";
import { mockSemantic } from "../mock/semantic";
import { normalizeControlResult, normalizeNativeSnapshot, SupervisorPollLoop, type PetTransportSnapshot, type SupervisorAdapter } from "./supervisor";

test("control results expose only bounded success or error kinds", () => {
  assert.deepEqual(normalizeControlResult({ ok: true, token: "ignored", path: "ignored" }), { ok: true });
  assert.deepEqual(normalizeControlResult({ ok: false, error_kind: "rejected", detail: "ignored" }), { ok: false, error_kind: "rejected" });
  assert.deepEqual(normalizeControlResult({ ok: false, error_kind: "timeout" }), { ok: false, error_kind: "timeout" });
  assert.deepEqual(normalizeControlResult({ ok: false, error_kind: "raw-supervisor-error" }), { ok: false, error_kind: "invalid_response" });
  assert.deepEqual(normalizeControlResult({ ok: false, token: "never returned" }), { ok: false, error_kind: "invalid_response" });
  assert.deepEqual(normalizeControlResult(null), { ok: false, error_kind: "invalid_response" });
});

test("native snapshot accepts only a complete sanitized semantic contract", () => {
  const semantic = mockSemantic({}, "2026-09-03T00:00:00.000Z");
  const snapshot = normalizeNativeSnapshot({ connected: true, semantic, last_success_at: "2026-09-03T00:00:01.000Z", secret: "must be ignored" });
  assert.deepEqual(snapshot, { connected: true, semantic, last_success_at: "2026-09-03T00:00:01.000Z" });
  assert.equal(normalizeNativeSnapshot({ connected: true, semantic: { ...semantic, confidence: 2 } }).connected, false);
  assert.equal(normalizeNativeSnapshot({ connected: false, last_error_kind: "unauthorized", token: "never returned" }).last_error_kind, "unauthorized");
  assert.equal(normalizeNativeSnapshot({ connected: false, last_error_kind: "token-value" }).last_error_kind, "unavailable");
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

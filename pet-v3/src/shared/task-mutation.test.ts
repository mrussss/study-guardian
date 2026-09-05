import { strict as assert } from "node:assert";
import test from "node:test";
import { TaskMutationQueue, settleTaskPickerResult, type TaskPickerActionResult } from "./task-mutation";

function deferred<T>(): { promise: Promise<T>; resolve: (value: T) => void } {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>(nextResolve => { resolve = nextResolve; });
  return { promise, resolve };
}

test("keeps only the latest pending ordinary task selection", async () => {
  const queue = new TaskMutationQueue();
  const gate = deferred<void>();
  const calls: string[] = [];
  const active = queue.enqueue(async () => { calls.push("Go"); await gate.promise; return { ok: true } satisfies TaskPickerActionResult; }, { coalesceKey: "task-selection" });
  const dropped = queue.enqueue(async () => { calls.push("八股"); return { ok: true }; }, { coalesceKey: "task-selection" });
  const latest = queue.enqueue(async () => { calls.push("算法"); return { ok: true }; }, { coalesceKey: "task-selection" });
  assert.deepEqual(await dropped, { status: "superseded" });
  assert.deepEqual(calls, ["Go"]);
  gate.resolve();
  await Promise.all([active, latest]);
  assert.deepEqual(calls, ["Go", "算法"]);
});

test("does not drop save operations across a coalescing boundary", async () => {
  const queue = new TaskMutationQueue();
  const gate = deferred<void>();
  const calls: string[] = [];
  const firstSave = queue.enqueue(async () => { calls.push("save-1"); await gate.promise; });
  const intermediate = queue.enqueue(async () => { calls.push("intermediate"); }, { coalesceKey: "task-selection" });
  const secondSave = queue.enqueue(async () => { calls.push("save-2"); });
  const finalSelection = queue.enqueue(async () => { calls.push("final-selection"); }, { coalesceKey: "task-selection" });
  gate.resolve();
  await Promise.all([firstSave, intermediate, secondSave, finalSelection]);
  assert.deepEqual(calls, ["save-1", "intermediate", "save-2", "final-selection"]);
});

test("stale mutation results are ignored without fabricating task data", () => {
  const stale = settleTaskPickerResult(4, 3, { ok: true });
  assert.equal(stale.applied, false);
  assert.deepEqual(stale.result, { ok: true });
  const latest = settleTaskPickerResult(4, 4, { ok: true });
  assert.equal(latest.applied, true);
  assert.deepEqual(latest.result, { ok: true });
});

test("a failed mutation does not poison the next queued operation", async () => {
  const queue = new TaskMutationQueue();
  const failed = await queue.enqueue(async () => { throw new Error("transport failure"); });
  assert.equal(failed.status, "failed");
  const next = await queue.enqueue(async () => "next");
  assert.deepEqual(next, { status: "completed", value: "next" });
});

test("ten rapid ordinary selections execute at most the active and latest request", async () => {
  const queue = new TaskMutationQueue();
  const gate = deferred<void>();
  const calls: string[] = [];
  const selections = ["任务1", "任务2", "任务3", "任务4", "任务5", "任务6", "任务7", "任务8", "任务9", "任务10"];
  const results = selections.map((name, index) => queue.enqueue(async () => {
    calls.push(name);
    if (index === 0) await gate.promise;
    return { ok: true } satisfies TaskPickerActionResult;
  }, { coalesceKey: "task-selection" }));

  await Promise.resolve();
  assert.deepEqual(calls, ["任务1"]);
  gate.resolve();
  await Promise.all(results);
  assert.deepEqual(calls, ["任务1", "任务10"]);
});

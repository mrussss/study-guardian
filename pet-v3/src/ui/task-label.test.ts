import { strict as assert } from "node:assert";
import test from "node:test";
import { getPetTaskLabel } from "./task-label";

test("pet task label keeps the durable task when activity data is stale", () => {
  assert.equal(getPetTaskLabel(true, "Go"), "Go");
  assert.equal(getPetTaskLabel(true, "算法"), "算法");
  assert.equal(getPetTaskLabel(true, "  "), "未设置任务");
  assert.equal(getPetTaskLabel(true, undefined), "未设置任务");
  assert.equal(getPetTaskLabel(false, "Go"), "监督离线");
});

import assert from "node:assert/strict";
import test from "node:test";
import { getTaskWheelSectorPath, getTaskWheelSlotPosition, TASK_WHEEL_ADD_SLOT, TASK_WHEEL_TASK_SLOTS } from "./TaskWheelDialog";

test("task wheel uses equal angular slots with the plus action at 135 degrees", () => {
  assert.equal(TASK_WHEEL_ADD_SLOT, 5);
  assert.equal(TASK_WHEEL_TASK_SLOTS.includes(TASK_WHEEL_ADD_SLOT), false);
  const top = getTaskWheelSlotPosition(0);
  const bottom = getTaskWheelSlotPosition(4);
  const add = getTaskWheelSlotPosition(TASK_WHEEL_ADD_SLOT);
  assert.equal(top.x, bottom.x);
  assert.ok(top.y < 50 && bottom.y > 50);
  assert.ok(add.x < 50 && add.y > 50);
});

test("task wheel sector paths are generated from the same symmetric geometry", () => {
  const paths = Array.from({ length: 8 }, (_, index) => getTaskWheelSectorPath(index));
  assert.equal(new Set(paths).size, 8);
  assert.ok(paths.every(path => path.startsWith("M ") && !path.startsWith("M 50 50") && path.includes("A 48 48") && path.includes("A 18 18") && path.endsWith(" Z")));
});

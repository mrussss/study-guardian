import { strict as assert } from "node:assert";
import test from "node:test";
import { isClickGesture, isInteractiveTargetShape, movementDistance, shouldBeginNativeDrag, shouldStartDragging, type DragTargetShape } from "./drag";

const target = (tagName: string, options: Partial<DragTargetShape> = {}): DragTargetShape => ({ tagName, ...options });

test("pet canvas, state, and task surfaces are draggable in native runtime", () => {
  for (const tagName of ["canvas", "div", "span"]) {
    assert.equal(shouldStartDragging(0, true, isInteractiveTargetShape(target(tagName))), true, tagName);
  }
});

test("interactive controls and descendants never start dragging", () => {
  for (const tagName of ["button", "select", "input", "textarea", "a"]) {
    assert.equal(isInteractiveTargetShape(target(tagName)), true, tagName);
    assert.equal(shouldStartDragging(0, true, isInteractiveTargetShape(target(tagName))), false, tagName);
  }
  assert.equal(isInteractiveTargetShape(target("span", { ancestorTags: ["BUTTON"] })), true);
  assert.equal(isInteractiveTargetShape(target("span", { inDevPanel: true })), true);
  assert.equal(isInteractiveTargetShape(target("span", { hasNoDrag: true })), true);
});

test("only a native left-button press starts dragging", () => {
  assert.equal(shouldStartDragging(1, true, false), false);
  assert.equal(shouldStartDragging(2, true, false), false);
  assert.equal(shouldStartDragging(0, false, false), false);
});

test("click and drag gestures use the five pixel boundary", () => {
  const start = { x: 10, y: 10 };
  assert.equal(movementDistance(start, { x: 13, y: 14 }), 5);
  assert.equal(isClickGesture(start, { x: 13, y: 13 }), true);
  assert.equal(isClickGesture(start, { x: 13, y: 14 }), false);
  assert.equal(shouldBeginNativeDrag(start, { x: 13, y: 14 }), true);
});

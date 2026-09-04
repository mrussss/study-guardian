import { strict as assert } from "node:assert";
import test from "node:test";
import { isInteractiveTargetShape, shouldStartDragging, type DragTargetShape } from "./drag";

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

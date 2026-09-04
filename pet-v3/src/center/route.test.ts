import { strict as assert } from "node:assert";
import test from "node:test";
import { isControlCenterRoute } from "./route";

test("control center route contract accepts only bounded navigation ids", () => {
  assert.equal(isControlCenterRoute("overview"), true);
  assert.equal(isControlCenterRoute("settings"), true);
  assert.equal(isControlCenterRoute("review"), true);
  assert.equal(isControlCenterRoute("javascript:alert(1)"), false);
  assert.equal(isControlCenterRoute(undefined), false);
});

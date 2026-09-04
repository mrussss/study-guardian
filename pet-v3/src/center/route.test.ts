import { strict as assert } from "node:assert";
import test from "node:test";
import { applyControlCenterRouteRequest, isControlCenterRoute, type ControlCenterRouteRequest } from "./route";

test("control center route contract accepts only bounded navigation ids", () => {
  assert.equal(isControlCenterRoute("overview"), true);
  assert.equal(isControlCenterRoute("settings"), true);
  assert.equal(isControlCenterRoute("review"), true);
  assert.equal(isControlCenterRoute("javascript:alert(1)"), false);
  assert.equal(isControlCenterRoute(undefined), false);
});

test("reopening the same native route remains a new navigation request", () => {
  const initial: ControlCenterRouteRequest = { route: "overview", revision: 0 };
  const first = applyControlCenterRouteRequest(initial, "settings");
  // The center can navigate locally to history while the last native request
  // stays settings. A second settings request must still trigger its effect.
  const repeated = applyControlCenterRouteRequest(first, "settings");
  assert.equal(repeated.route, "settings");
  assert.notEqual(repeated.revision, first.revision);
  assert.notStrictEqual(repeated, first);
  const changed = applyControlCenterRouteRequest(repeated, "review");
  assert.equal(changed.route, "review");
  assert.ok(changed.revision > repeated.revision);
  assert.strictEqual(applyControlCenterRouteRequest(changed, "invalid-route"), changed);
});

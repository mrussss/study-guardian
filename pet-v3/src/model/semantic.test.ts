import { strict as assert } from "node:assert";
import test from "node:test";
import { isCurrentActivityView } from "./semantic";
import { mockSemantic } from "../mock/semantic";

test("CurrentActivityView validator accepts the complete v1 contract", () => {
  assert.equal(isCurrentActivityView(mockSemantic({}, "2026-09-03T00:00:00.000Z")), true);
});

test("CurrentActivityView validator rejects invalid enums, dates, and confidence", () => {
  const base = mockSemantic();
  assert.equal(isCurrentActivityView({ ...base, schema_version: 2 }), false);
  assert.equal(isCurrentActivityView({ ...base, user_mode: "RUNNING" }), false);
  assert.equal(isCurrentActivityView({ ...base, activity: "OTHER" }), false);
  assert.equal(isCurrentActivityView({ ...base, observed_at: "" }), false);
  assert.equal(isCurrentActivityView({ ...base, observed_at: "not-a-date" }), false);
  for (const confidence of [-0.01, 1.01, Number.NaN, Number.POSITIVE_INFINITY]) {
    assert.equal(isCurrentActivityView({ ...base, confidence }), false);
  }
});

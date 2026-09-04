import { strict as assert } from "node:assert";
import test from "node:test";
import { clampProgress, formatFocusMinutes, totalFocusMinutes, type FocusDay } from "./dashboard";

test("dashboard progress is bounded for incomplete and completed goals", () => {
  assert.equal(clampProgress(-1), 0);
  assert.equal(clampProgress(.72), .72);
  assert.equal(clampProgress(2), 1);
  assert.equal(clampProgress(Number.NaN), 0);
});

test("focus minutes use readable hour and minute labels", () => {
  assert.equal(formatFocusMinutes(0), "0m");
  assert.equal(formatFocusMinutes(86), "1h 26m");
  assert.equal(formatFocusMinutes(-4), "0m");
});

test("weekly total only sums non-negative focus minutes", () => {
  const days: FocusDay[] = [
    { label: "一", minutes: 20, target: 120, completed: false },
    { label: "二", minutes: -1, target: 120, completed: false },
    { label: "三", minutes: 66, target: 120, completed: false },
  ];
  assert.equal(totalFocusMinutes(days), 86);
});

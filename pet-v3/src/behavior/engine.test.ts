import { strict as assert } from "node:assert";
import test from "node:test";
import { BehaviorEngine, targetState } from "./engine";
import { mockSemantic } from "../mock/semantic";

test("priority selects celebration, reminder, offline and resting first", () => {
  const base = mockSemantic({ activity: "CODING" });
  assert.equal(targetState({ semantic: base, nowMs: 0, event: { kind: "celebrate" } }), "CELEBRATE");
  assert.equal(targetState({ semantic: base, nowMs: 0, event: { kind: "reminder" } }), "TALKING");
  assert.equal(targetState({ semantic: { ...base, fresh: false }, nowMs: 0 }), "OFFLINE");
  assert.equal(targetState({ semantic: { ...base, user_mode: "BREAK" }, nowMs: 0 }), "RESTING");
});

test("semantic activity maps independently from relation", () => {
  assert.equal(targetState({ semantic: mockSemantic({ activity: "CODING", relation: "DISTRACTED" }), nowMs: 0 }), "DISTRACTED");
  assert.equal(targetState({ semantic: mockSemantic({ activity: "CODING", relation: "FOCUSED" }), nowMs: 0 }), "CODING");
  assert.equal(targetState({ semantic: mockSemantic({ activity: "BROWSING", relation: "FOCUSED" }), nowMs: 0 }), "LEARNING");
  assert.equal(targetState({ semantic: mockSemantic({ activity: "AI_ASSISTED" }), nowMs: 0 }), "LEARNING");
});

test("normal, distracted and thinking hysteresis are explicit", () => {
  const engine = new BehaviorEngine({ normalHysteresisMs: 1000, distractedHysteresisMs: 100, thinkingStableMs: 800 });
  const coding = mockSemantic({ activity: "CODING" });
  assert.equal(engine.update({ semantic: coding, nowMs: 0 }), "IDLE");
  assert.equal(engine.update({ semantic: coding, nowMs: 999 }), "IDLE");
  assert.equal(engine.update({ semantic: coding, nowMs: 1000 }), "CODING");
  const distracted = mockSemantic({ relation: "DISTRACTED", activity: "CODING" });
  assert.equal(engine.update({ semantic: distracted, nowMs: 1050 }), "CODING");
  assert.equal(engine.update({ semantic: distracted, nowMs: 1150 }), "DISTRACTED");
  const thinking = mockSemantic({ activity: "UNKNOWN", interaction: "IDLE_STATIC" });
  assert.equal(engine.update({ semantic: thinking, nowMs: 1200 }), "DISTRACTED");
  assert.equal(engine.update({ semantic: thinking, nowMs: 1999 }), "DISTRACTED");
  assert.equal(engine.update({ semantic: thinking, nowMs: 2000 }), "THINKING");
});

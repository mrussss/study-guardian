import { strict as assert } from "node:assert";
import test from "node:test";
import { loadSkinManifest, resolveState } from "./skin";

const skin = {
  schema_version: 1 as const,
  id: "placeholder-v1",
  name: "StudyGuardian Placeholder",
  license: "Original placeholder; CC0-style project asset",
  frame_size: { width: 16, height: 16 },
  display_size: { width: 176, height: 176 },
  fps: 8,
  pixel_art: true,
  states: { IDLE: "idle.png", CODING: "coding.png" },
};

test("skin v1 validates and falls back missing states", () => {
  const loaded = loadSkinManifest(skin);
  assert.deepEqual(resolveState(loaded, "CODING"), { state: "CODING", source: "coding.png" });
  assert.deepEqual(resolveState(loaded, "THINKING"), { state: "IDLE", source: "idle.png" });
  assert.deepEqual(resolveState(loaded, "ALGORITHM"), { state: "CODING", source: "coding.png" });
});

test("invalid skin schema is rejected", () => {
  assert.throws(() => loadSkinManifest({ ...skin, schema_version: 2 }));
  assert.throws(() => loadSkinManifest({ ...skin, frame_size: { width: 0, height: 16 } }));
});

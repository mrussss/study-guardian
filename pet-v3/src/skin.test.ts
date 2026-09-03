import { strict as assert } from "node:assert";
import { readFileSync } from "node:fs";
import test from "node:test";
import { loadSkinManifest, resolveState } from "./skin";

const existingManifest = JSON.parse(readFileSync(new URL("../../pet/assets/skins/studyguardian-pixel/manifest.json", import.meta.url), "utf8"));

test("real existing PyQt skin manifest is accepted as v1", () => {
  const loaded = loadSkinManifest(existingManifest);
  assert.equal(loaded.frame_size, 64);
  assert.equal(loaded.display_size, 128);
  assert.equal(loaded.fps, 7);
  for (const state of ["idle", "study", "distracted", "rest", "talk", "celebrate"]) assert.equal(typeof loaded.states[state], "string");
  assert.deepEqual(resolveState(loaded, "CODING"), { state: "study", source: "sprites/study.png" });
  assert.deepEqual(resolveState(loaded, "DISTRACTED"), { state: "distracted", source: "sprites/distracted.png" });
  assert.deepEqual(resolveState(loaded, "CELEBRATE"), { state: "celebrate", source: "sprites/celebrate.png" });
  assert.deepEqual(resolveState(loaded, "OFFLINE"), { state: "idle", source: "sprites/idle.png" });
});

test("skin v1 requires numeric dimensions and an idle hard fallback", () => {
  assert.throws(() => loadSkinManifest({ ...existingManifest, frame_size: { width: 64, height: 64 } }));
  assert.throws(() => loadSkinManifest({ ...existingManifest, states: { ...existingManifest.states, idle: "" } }));
  assert.throws(() => loadSkinManifest({ ...existingManifest, fps: Number.NaN }));
});

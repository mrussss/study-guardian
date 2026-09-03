import { strict as assert } from "node:assert";
import test from "node:test";
import { AnimationEngine } from "./engine";
import { splitHorizontal } from "./sprite";

const frames = splitHorizontal(32, 8, 8);

test("horizontal spritesheets split into pixel frames", () => {
  assert.equal(frames.length, 4);
  assert.deepEqual(frames[2], { sx: 16, sy: 0, width: 8, height: 8 });
  assert.deepEqual(splitHorizontal(7, 8, 8), []);
});

test("animation timing loops and one-shot completion", () => {
  const engine = new AnimationEngine();
  engine.loop({ name: "walk", frames, fps: 2, loop: true });
  engine.update(500);
  assert.equal(engine.frame()?.sx, 8);
  engine.update(1500);
  assert.equal(engine.frame()?.sx, 0);
  let completed = 0;
  engine.oneShot({ name: "celebrate", frames, fps: 4, loop: false }, () => completed++);
  engine.update(1000);
  assert.equal(engine.frame()?.sx, 24);
  assert.equal(completed, 1);
  engine.stop();
  assert.equal(engine.frame(), null);
});

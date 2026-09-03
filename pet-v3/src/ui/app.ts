import { invoke } from "@tauri-apps/api/core";
import { BehaviorEngine, type VisualState } from "../behavior/engine";
import { AnimationEngine } from "../animation/engine";
import { splitHorizontal } from "../animation/sprite";
import { mockSemantic } from "../mock/semantic";
import type { Activity, CurrentActivityView, Relation, UserMode } from "../model/semantic";

const colors: Record<VisualState, string> = {
  IDLE: "#8aa0b8", CODING: "#50d890", ALGORITHM: "#5fc6ff", READING: "#f1d36b", WRITING: "#d39aff",
  WATCHING: "#ff9e72", LEARNING: "#7fe7df", DISTRACTED: "#ff6978", RESTING: "#b4a7d6", OFFLINE: "#66717e",
  THINKING: "#b4d7ff", CELEBRATE: "#ffe36e", TALKING: "#ffa8df",
};

export function mountApp(root: HTMLElement): void {
  const engine = new BehaviorEngine();
  const animation = new AnimationEngine();
  const placeholderFrames = splitHorizontal(96, 96, 24, 96);
  let semantic = mockSemantic({});
  let clickThrough = false;
  let lastFrame = performance.now();
  root.innerHTML = `<section class="pet-shell" data-tauri-drag-region>
    <canvas class="pet-canvas" width="96" height="96" aria-label="StudyGuardian Pet"></canvas>
    <div class="pet-state" data-state>LEARNING</div>
    <div class="pet-task" data-task></div>
    <div class="pet-controls" data-dev-panel>
      <select data-mode aria-label="mock mode"><option>STUDY</option><option>BREAK</option><option>STANDBY</option><option>OFF</option></select>
      <select data-activity aria-label="mock activity"><option>GENERAL_STUDY</option><option>CODING</option><option>ALGORITHM</option><option>READING</option><option>WRITING</option><option>WATCHING</option><option>AI_ASSISTED</option><option>BROWSING</option><option>UNKNOWN</option></select>
      <button data-distracted type="button">分心</button><button data-offline type="button">离线</button>
      <button data-clickthrough type="button">穿透:关</button>
    </div>
  </section>`;
  const canvas = root.querySelector("canvas")!;
  const stateLabel = root.querySelector<HTMLElement>("[data-state]")!;
  const taskLabel = root.querySelector<HTMLElement>("[data-task]")!;
  const mode = root.querySelector<HTMLSelectElement>("[data-mode]")!;
  const activity = root.querySelector<HTMLSelectElement>("[data-activity]")!;
  const draw = (state: VisualState): void => {
    const ctx = canvas.getContext("2d")!;
    ctx.imageSmoothingEnabled = false;
    ctx.clearRect(0, 0, canvas.width, canvas.height);
    ctx.fillStyle = colors[state];
    const frameOffset = animation.frame()?.sx ?? 0;
    const bob = Math.floor(frameOffset / 24) % 2;
    ctx.fillRect(28, 20 + bob, 40, 48);
    ctx.fillRect(20, 34 + bob, 56, 28);
    ctx.fillStyle = "#18202b";
    ctx.fillRect(36, 34, 6, 6); ctx.fillRect(54, 34, 6, 6);
    ctx.fillRect(38, 52, 20, 4);
    ctx.fillStyle = colors[state];
    ctx.fillRect(14, 72, 68, 8);
  };
  const refresh = (now: number): void => {
    const state = engine.update({ semantic, nowMs: now });
    if (animation.currentName() !== state) animation.loop({ name: state, frames: placeholderFrames, fps: 8, loop: true });
    animation.update(now - lastFrame);
    stateLabel.textContent = state;
    stateLabel.style.color = colors[state];
    taskLabel.textContent = semantic.fresh ? semantic.task : "ActivityWatch offline / stale";
    draw(state);
    lastFrame = now;
    requestAnimationFrame(refresh);
  };
  const updateMock = (overrides: Parameters<typeof mockSemantic>[0]): void => {
    semantic = mockSemantic(overrides);
  };
  mode.addEventListener("change", () => updateMock({ user_mode: mode.value as UserMode }));
  activity.addEventListener("change", () => updateMock({ activity: activity.value as Activity }));
  root.querySelector("[data-distracted]")!.addEventListener("click", () => updateMock({ relation: "DISTRACTED" as Relation }));
  root.querySelector("[data-offline]")!.addEventListener("click", () => { semantic = mockSemantic({ fresh: false }); });
  root.querySelector("[data-clickthrough]")!.addEventListener("click", async () => {
    clickThrough = !clickThrough;
    try { await invoke("set_click_through", { enabled: clickThrough }); } catch { /* browser dev mode has no Tauri host */ }
    root.querySelector<HTMLButtonElement>("[data-clickthrough]")!.textContent = `穿透:${clickThrough ? "开" : "关"}`;
  });
  void lastFrame;
  requestAnimationFrame(refresh);
}

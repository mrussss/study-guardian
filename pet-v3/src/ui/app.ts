import { invoke } from "@tauri-apps/api/core";
import { AnimationEngine } from "../animation/engine";
import { loadSkinAnimation, type LoadedAnimation } from "../animation/assets";
import { drawPixelFrame, splitHorizontal } from "../animation/sprite";
import { BehaviorEngine, type VisualState } from "../behavior/engine";
import { mockActivityWatchStale, mockSemantic, mockSupervisorOffline } from "../mock/semantic";
import type { Activity, CurrentActivityView, Relation, UserMode } from "../model/semantic";
import { loadSkinManifest } from "../skin";
import {
  NativeSupervisorAdapter,
  NativeSupervisorControlAdapter,
  SupervisorPollLoop,
  type ControlErrorKind,
} from "../transport/supervisor";
import { getCurrentWindow } from "@tauri-apps/api/window";
import {
  isInteractiveTarget,
  isClickGesture,
  shouldBeginNativeDrag,
  shouldStartDragging,
  type PointerPoint,
} from "./drag";
import legacyManifest from "../skins/studyguardian-pixel/manifest.json";
import idleURL from "../skins/studyguardian-pixel/sprites/idle.png";
import studyURL from "../skins/studyguardian-pixel/sprites/study.png";
import distractedURL from "../skins/studyguardian-pixel/sprites/distracted.png";
import restURL from "../skins/studyguardian-pixel/sprites/rest.png";
import talkURL from "../skins/studyguardian-pixel/sprites/talk.png";
import celebrateURL from "../skins/studyguardian-pixel/sprites/celebrate.png";

const colors: Record<VisualState, string> = {
  IDLE: "#8aa0b8", CODING: "#50d890", ALGORITHM: "#5fc6ff", READING: "#f1d36b", WRITING: "#d39aff",
  WATCHING: "#ff9e72", LEARNING: "#7fe7df", DISTRACTED: "#ff6978", RESTING: "#b4a7d6", OFFLINE: "#66717e",
  THINKING: "#b4d7ff", CELEBRATE: "#ffe36e", TALKING: "#ffa8df",
};

const skin = loadSkinManifest(legacyManifest);
const skinAssets: Record<string, string> = {
  idle: idleURL, study: studyURL, distracted: distractedURL, rest: restURL, talk: talkURL, celebrate: celebrateURL,
};

function isTauriRuntime(): boolean {
  return typeof window !== "undefined"
    && Object.prototype.hasOwnProperty.call(window, "__TAURI_INTERNALS__");
}

function modeLabel(mode: UserMode): string {
  switch (mode) {
    case "STUDY": return "学习中";
    case "BREAK": return "休息中";
    case "OFF": return "已结束";
    case "STANDBY": return "待机";
  }
}

function controlErrorMessage(kind: ControlErrorKind): string {
  switch (kind) {
    case "rejected": return "当前状态不允许该操作";
    case "unauthorized": return "Supervisor 授权失败";
    case "timeout": return "Supervisor 响应超时";
    case "invalid_response": return "Supervisor 返回异常";
    case "unavailable": return "Supervisor 暂不可用";
  }
}

export function mountApp(root: HTMLElement): void {
  const engine = new BehaviorEngine();
  const animation = new AnimationEngine();
  const nativeRuntime = isTauriRuntime();
  const supervisorAdapter = nativeRuntime ? new NativeSupervisorAdapter() : null;
  const supervisorControl = nativeRuntime ? new NativeSupervisorControlAdapter() : null;
  const supervisorPoll = supervisorAdapter ? new SupervisorPollLoop(supervisorAdapter) : null;
  const showDevPanel = import.meta.env.DEV && import.meta.env.VITE_PET_DEV_PANEL === "1";
  const emergencyFrames = splitHorizontal(96, 96, 24, 96);
  let semantic: CurrentActivityView = mockSemantic({});
  let connected = true;
  let clickThrough = false;
  let lastFrame = performance.now();
  let requestedState: VisualState | null = null;
  let activeAnimation: LoadedAnimation | null = null;
  let animationRequest = 0;
  let panelOpen = false;
  let gesture: { pointerId: number; start: PointerPoint; dragging: boolean } | null = null;
  root.innerHTML = `<section class="pet-shell">
    <canvas class="pet-canvas" width="220" height="220" aria-label="StudyGuardian Pet"></canvas>
    <div class="pet-state" data-state>LEARNING</div>
    <div class="pet-task" data-task></div>
    <div class="pet-control-panel" data-control-panel data-no-drag hidden>
      <div class="pet-control-title" data-control-title></div>
      <input data-task-input maxlength="256" placeholder="当前学习任务" aria-label="当前学习任务" />
      <div class="pet-control-actions">
        <button data-start-study type="button">开始学习</button>
        <button data-start-break type="button">开始休息</button>
      </div>
      <div class="pet-control-feedback" data-control-feedback role="status" aria-live="polite"></div>
      <button data-close-panel type="button">关闭</button>
    </div>
    ${showDevPanel ? `<div class="pet-controls" data-dev-panel>
      <select data-mode aria-label="mock mode"><option>STUDY</option><option>BREAK</option><option>STANDBY</option><option>OFF</option></select>
      <select data-activity aria-label="mock activity"><option>GENERAL_STUDY</option><option>CODING</option><option>ALGORITHM</option><option>READING</option><option>WRITING</option><option>WATCHING</option><option>AI_ASSISTED</option><option>BROWSING</option><option>UNKNOWN</option></select>
      <button data-distracted type="button">分心</button><button data-offline type="button">Supervisor离线</button><button data-stale type="button">AW过期</button>
      <button data-clickthrough type="button">穿透:关</button>
    </div>` : ""}
  </section>`;
  const petShell = root.querySelector<HTMLElement>(".pet-shell")!;
  const canvas = root.querySelector<HTMLCanvasElement>("canvas")!;
  const stateLabel = root.querySelector<HTMLElement>("[data-state]")!;
  const taskLabel = root.querySelector<HTMLElement>("[data-task]")!;
  const controlPanel = root.querySelector<HTMLElement>("[data-control-panel]")!;
  const controlTitle = root.querySelector<HTMLElement>("[data-control-title]")!;
  const taskInput = root.querySelector<HTMLInputElement>("[data-task-input]")!;
  const controlFeedback = root.querySelector<HTMLElement>("[data-control-feedback]")!;
  const startStudy = root.querySelector<HTMLButtonElement>("[data-start-study]")!;
  const startBreak = root.querySelector<HTMLButtonElement>("[data-start-break]")!;
  const closePanel = root.querySelector<HTMLButtonElement>("[data-close-panel]")!;

  const renderPanel = (): void => {
    controlTitle.textContent = !connected ? "Supervisor 离线" : `当前：${modeLabel(semantic.user_mode)}`;
    if (document.activeElement !== taskInput) taskInput.value = semantic.task;
  };

  const setPanelOpen = (open: boolean): void => {
    panelOpen = open;
    controlPanel.hidden = !open;
    if (open) renderPanel();
  };

  const openQuickPanel = async (): Promise<void> => {
    if (nativeRuntime) {
      try {
        await invoke("open_quick_panel");
        return;
      } catch {
        // Keep the compact POC panel as a fail-soft browser/native fallback.
      }
    }
    setPanelOpen(!panelOpen);
  };

  const setControlBusy = (busy: boolean): void => {
    startStudy.disabled = busy;
    startBreak.disabled = busy;
    closePanel.disabled = busy;
    taskInput.disabled = busy;
  };

  const runMode = async (mode: "STUDY" | "BREAK"): Promise<void> => {
    if (!supervisorControl || !supervisorAdapter) {
      controlFeedback.textContent = "仅支持原生 Tauri Pet";
      return;
    }
    setControlBusy(true);
    controlFeedback.textContent = "正在提交…";
    try {
      const result = mode === "STUDY"
        ? await supervisorControl.setModeStudy(taskInput.value.trim())
        : await supervisorControl.setModeBreak();
      if (!result.ok) {
        controlFeedback.textContent = controlErrorMessage(result.error_kind ?? "invalid_response");
        return;
      }
      controlFeedback.textContent = "已提交，正在刷新…";
      const snapshot = await supervisorAdapter.poll();
      connected = snapshot.connected;
      semantic = snapshot.semantic;
      renderPanel();
      controlFeedback.textContent = snapshot.connected
        ? "已更新"
        : controlErrorMessage(snapshot.last_error_kind ?? "unavailable");
    } finally {
      setControlBusy(false);
    }
  };

  const clearGesture = (): void => {
    if (gesture) {
      try { petShell.releasePointerCapture(gesture.pointerId); } catch { /* capture may already be released */ }
    }
    gesture = null;
  };

  petShell.addEventListener("pointerdown", event => {
    if (event.button !== 0 || isInteractiveTarget(event.target)) return;
    gesture = { pointerId: event.pointerId, start: { x: event.clientX, y: event.clientY }, dragging: false };
    try { petShell.setPointerCapture(event.pointerId); } catch { /* pointer capture is optional */ }
  });
  petShell.addEventListener("pointermove", event => {
    if (!gesture || gesture.pointerId !== event.pointerId || gesture.dragging) return;
    const current = { x: event.clientX, y: event.clientY };
    if (!shouldStartDragging(0, nativeRuntime, false) || !shouldBeginNativeDrag(gesture.start, current)) return;
    gesture.dragging = true;
    setPanelOpen(false);
    void getCurrentWindow().startDragging().catch(() => {
      // Native drag failure must not leak raw IPC errors into the UI.
    });
  });
  petShell.addEventListener("pointerup", event => {
    if (!gesture || gesture.pointerId !== event.pointerId) return;
    const completed = gesture;
    const current = { x: event.clientX, y: event.clientY };
    clearGesture();
    if (!completed.dragging && isClickGesture(completed.start, current)) void openQuickPanel();
  });
  petShell.addEventListener("pointercancel", clearGesture);
  window.addEventListener("blur", clearGesture);
  startStudy.addEventListener("click", () => { void runMode("STUDY"); });
  startBreak.addEventListener("click", () => { void runMode("BREAK"); });
  closePanel.addEventListener("click", () => setPanelOpen(false));

  const drawEmergencyPlaceholder = (state: VisualState): void => {
    const ctx = canvas.getContext("2d")!;
    ctx.imageSmoothingEnabled = false;
    ctx.clearRect(0, 0, canvas.width, canvas.height);
    ctx.fillStyle = colors[state];
    const frameOffset = animation.frame()?.sx ?? emergencyFrames[0]?.sx ?? 0;
    const bob = Math.floor(frameOffset / 24) % 2;
    ctx.fillRect(90, 62 + bob, 40, 48);
    ctx.fillRect(82, 76 + bob, 56, 28);
    ctx.fillStyle = "#18202b";
    ctx.fillRect(98, 76, 6, 6); ctx.fillRect(116, 76, 6, 6); ctx.fillRect(100, 94, 20, 4);
    ctx.fillStyle = colors[state];
    ctx.fillRect(76, 114, 68, 8);
  };

  const draw = (state: VisualState): void => {
    const ctx = canvas.getContext("2d")!;
    ctx.imageSmoothingEnabled = false;
    ctx.clearRect(0, 0, canvas.width, canvas.height);
    const frame = animation.frame();
    if (activeAnimation && frame) {
      const displaySize = skin.display_size;
      const offset = Math.floor((canvas.width - displaySize) / 2);
      drawPixelFrame(ctx, activeAnimation.image, frame, offset, offset, displaySize, displaySize);
      return;
    }
    drawEmergencyPlaceholder(state);
  };

  const selectAnimation = async (state: VisualState): Promise<void> => {
    const request = ++animationRequest;
    const loaded = await loadSkinAnimation(skin, state, skinAssets);
    if (request !== animationRequest) return;
    activeAnimation = loaded;
    if (loaded) animation.loop(loaded.clip);
  };

  const refresh = (now: number): void => {
    const state = engine.update({ connected, semantic, nowMs: now });
    if (requestedState !== state) {
      requestedState = state;
      void selectAnimation(state);
    }
    animation.update(now - lastFrame);
    stateLabel.textContent = state;
    stateLabel.style.color = colors[state];
    taskLabel.textContent = !connected ? "Supervisor offline" : !semantic.fresh ? "活动状态不可用" : semantic.task;
    if (panelOpen) renderPanel();
    draw(state);
    lastFrame = now;
    requestAnimationFrame(refresh);
  };
  if (showDevPanel) {
    const mode = root.querySelector<HTMLSelectElement>("[data-mode]")!;
    const activity = root.querySelector<HTMLSelectElement>("[data-activity]")!;
    const updateMock = (overrides: Parameters<typeof mockSemantic>[0]): void => {
      connected = true;
      semantic = mockSemantic(overrides);
    };
    mode.addEventListener("change", () => updateMock({ user_mode: mode.value as UserMode }));
    activity.addEventListener("change", () => updateMock({ activity: activity.value as Activity }));
    root.querySelector("[data-distracted]")!.addEventListener("click", () => updateMock({ relation: "DISTRACTED" as Relation }));
    root.querySelector("[data-offline]")!.addEventListener("click", () => {
      const mock = mockSupervisorOffline();
      connected = mock.connected;
      semantic = mock.semantic;
    });
    root.querySelector("[data-stale]")!.addEventListener("click", () => {
      const mock = mockActivityWatchStale();
      connected = mock.connected;
      semantic = mock.semantic;
    });
    root.querySelector("[data-clickthrough]")!.addEventListener("click", async () => {
      clickThrough = !clickThrough;
      try { await invoke("set_click_through", { enabled: clickThrough }); } catch { /* browser dev mode has no Tauri host */ }
      root.querySelector<HTMLButtonElement>("[data-clickthrough]")!.textContent = `穿透:${clickThrough ? "开" : "关"}`;
    });
  }
  supervisorPoll?.start(snapshot => {
    connected = snapshot.connected;
    semantic = snapshot.semantic;
    if (panelOpen) renderPanel();
  });
  requestAnimationFrame(refresh);
}

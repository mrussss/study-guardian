import { invoke } from "@tauri-apps/api/core";
import { BehaviorEngine, type VisualState } from "../behavior/engine";
import { MockSupervisorRuntime, parseMockScenario } from "../mock/supervisor";
import { mockActivityWatchStale, mockSemantic, mockSupervisorOffline } from "../mock/semantic";
import type { Activity, CurrentActivityView, Relation, UserMode } from "../model/semantic";
import { getPetTaskLabel } from "./task-label";
import {
  NativeSupervisorAdapter,
  NativeSupervisorControlAdapter,
  SupervisorPollLoop,
  type NativeEyeCareSettings,
  type NativeEyeCareStatus,
  type ControlErrorKind,
} from "../transport/supervisor";
const colors: Record<VisualState, string> = {
  IDLE: "#8aa0b8", CODING: "#50d890", ALGORITHM: "#5fc6ff", READING: "#f1d36b", WRITING: "#d39aff",
  WATCHING: "#ff9e72", LEARNING: "#7fe7df", DISTRACTED: "#ff6978", RESTING: "#b4a7d6", OFFLINE: "#66717e",
  THINKING: "#b4d7ff", CELEBRATE: "#ffe36e", TALKING: "#ffa8df",
};

const visualStateLabels: Record<VisualState, string> = {
  IDLE: "等待开始", CODING: "编程中", ALGORITHM: "算法学习", READING: "阅读中", WRITING: "写作中",
  WATCHING: "观看学习资料", LEARNING: "学习中", DISTRACTED: "可能分心", RESTING: "休息中", OFFLINE: "服务离线",
  THINKING: "思考中", CELEBRATE: "完成啦", TALKING: "正在交流",
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
  const nativeRuntime = isTauriRuntime();
  const supervisorAdapter = nativeRuntime ? new NativeSupervisorAdapter() : null;
  const supervisorControl = nativeRuntime ? new NativeSupervisorControlAdapter() : null;
  const supervisorPoll = supervisorAdapter ? new SupervisorPollLoop(supervisorAdapter) : null;
  const showDevPanel = import.meta.env.DEV && import.meta.env.VITE_PET_DEV_PANEL === "1";
  let semantic: CurrentActivityView = mockSemantic({});
  let connected = true;
  let eyeCareSettings: NativeEyeCareSettings | undefined;
  let eyeCareStatus: NativeEyeCareStatus | undefined;
  let clickThrough = false;
  let studySeconds: number | undefined = nativeRuntime ? undefined : 1500;
  let panelOpen = false;
  let reminderClaimInFlight = false;
  root.innerHTML = `<section class="pet-shell">
    <div class="timer-face" title="按住计时器拖动">
      <svg class="timer-ring" viewBox="0 0 40 40" aria-hidden="true"><circle cx="20" cy="20" r="16"/><circle class="timer-ring-progress" cx="20" cy="20" r="16" pathLength="100" data-progress/><path d="M20 10v10l6 4" class="timer-hand"/></svg>
      <div class="timer-main"><span class="pet-state" data-state>等待学习</span><strong class="timer-digits" data-time aria-label="今日学习计时">--:--</strong></div>
    </div>
    <div class="timer-eye" data-eye-timer>护眼 · 等待数据</div>
    <div class="pet-eye-care-bubble" data-eye-care-bubble role="status" aria-live="polite" hidden></div>
    <div class="pet-task" data-task></div>
    <button class="pet-panel-entry" data-open-panel type="button" aria-label="打开 StudyGuardian 快捷面板">面板 <span aria-hidden="true">↗</span></button>
    <div class="pet-entry-error" data-entry-error role="status" hidden></div>
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
  const panelEntry = root.querySelector<HTMLButtonElement>("[data-open-panel]")!;
  const entryError = root.querySelector<HTMLElement>("[data-entry-error]")!;
  const timeLabel = root.querySelector<HTMLElement>("[data-time]")!;
  const eyeTimer = root.querySelector<HTMLElement>("[data-eye-timer]")!;
  const progress = root.querySelector<SVGCircleElement>("[data-progress]")!;
  const stateLabel = root.querySelector<HTMLElement>("[data-state]")!;
  const eyeCareBubble = root.querySelector<HTMLElement>("[data-eye-care-bubble]")!;
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

  const renderEyeCareBubble = (): void => {
    const current = eyeCareStatus;
    const relevant = current?.enabled === true && ["SHORT_BREAK_DUE", "LONG_BREAK_DUE", "SHORT_BREAK", "LONG_BREAK", "WAITING_RETURN"].includes(current.phase);
    if (!relevant || !current) { eyeCareBubble.hidden = true; eyeCareBubble.textContent = ""; root.querySelector(".pet-shell")?.classList.remove("has-eye-care"); return; }
    if (!connected) {
      if (eyeCareBubble.textContent !== "护眼状态暂不可用\n连接恢复后确认提醒") eyeCareBubble.textContent = "护眼状态暂不可用\n连接恢复后确认提醒";
      eyeCareBubble.hidden = false;
      root.querySelector(".pet-shell")?.classList.add("has-eye-care");
      return;
    }
    if (current.notification_suppressed) { eyeCareBubble.hidden = true; eyeCareBubble.textContent = ""; root.querySelector(".pet-shell")?.classList.remove("has-eye-care"); return; }
    const copy: Record<string, string> = {
      SHORT_BREAK_DUE: `该看看远处啦\n休息 ${eyeCareSettings?.short_break_minutes ?? 5} 分钟再回来吧`,
      LONG_BREAK_DUE: `该安排一次完整休息啦\n休息 ${eyeCareSettings?.long_break_minutes ?? 20} 分钟再回来吧`,
      SHORT_BREAK: "远眺休息中\n准备好后再回来",
      LONG_BREAK: "完整休息中\n准备好后再回来",
      WAITING_RETURN: "本轮休息计时完成\n点面板继续学习",
    };
    const message = copy[current.phase] ?? "";
    if (eyeCareBubble.textContent !== message) eyeCareBubble.textContent = message;
    eyeCareBubble.hidden = message.length === 0;
    root.querySelector(".pet-shell")?.classList.toggle("has-eye-care", !eyeCareBubble.hidden);
  };

  const setPanelOpen = (open: boolean): void => {
    panelOpen = open;
    controlPanel.hidden = !open;
    if (open) renderPanel();
  };

  const openQuickPanel = async (): Promise<void> => {
    if (!nativeRuntime) {
      setPanelOpen(!panelOpen);
      return;
    }
    panelEntry.disabled = true;
    entryError.hidden = true;
    try {
      await invoke("open_quick_panel");
    } catch {
      entryError.textContent = "面板未能打开，请重试或使用托盘入口";
      entryError.hidden = false;
    } finally {
      panelEntry.disabled = false;
    }
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

  // Windows/WebView2 owns the CSS non-client drag region. The explicit
  // no-drag button is the only click entry: no capture, move IPC or timer.
  panelEntry.addEventListener("click", () => { void openQuickPanel(); });
  startStudy.addEventListener("click", () => { void runMode("STUDY"); });
  startBreak.addEventListener("click", () => { void runMode("BREAK"); });
  closePanel.addEventListener("click", () => setPanelOpen(false));

  const refresh = (now: number): void => {
    const state = engine.update({ connected, semantic, nowMs: now });
    stateLabel.textContent = visualStateLabels[state];
    stateLabel.style.color = colors[state];
    taskLabel.textContent = connected ? (semantic.task || "自由学习") : getPetTaskLabel(false, semantic.task);
    if (panelOpen) renderPanel();
    const duration = (seconds: number): string => { const n = Math.max(0, Math.floor(seconds)); return `${String(Math.floor(n / 60)).padStart(2, "0")}:${String(n % 60).padStart(2, "0")}`; };
    timeLabel.textContent = connected && studySeconds !== undefined ? duration(studySeconds) : "--:--";
    root.querySelector(".pet-shell")?.classList.toggle("is-running", connected && semantic.user_mode === "STUDY");
    const remaining = Math.max(0, (eyeCareSettings?.focus_minutes ?? 40) * 60 - (eyeCareStatus?.focus_segment_seconds ?? 0));
    eyeTimer.textContent = !connected ? "护眼 · 等待连接" : !eyeCareStatus ? "护眼 · 等待数据" : !eyeCareStatus.enabled ? "护眼 · 未开启" : eyeCareStatus.phase === "FOCUSING" ? `护眼 ${duration(remaining)}` : ["SHORT_BREAK_DUE", "LONG_BREAK_DUE"].includes(eyeCareStatus.phase) ? "护眼 · 该休息了" : "护眼 · 休息中";
    progress.style.strokeDasharray = `${Math.min(100, 100 * (eyeCareStatus?.focus_segment_seconds ?? 0) / ((eyeCareSettings?.focus_minutes ?? 40) * 60))} 100`;
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
    studySeconds = snapshot.study_seconds;
    if (snapshot.eye_care_settings) eyeCareSettings = snapshot.eye_care_settings;
    if (snapshot.eye_care_status) eyeCareStatus = snapshot.eye_care_status;
    renderEyeCareBubble();
    if (panelOpen) renderPanel();
    if (nativeRuntime && snapshot.connected && snapshot.eye_care_status && snapshot.eye_care_status.enabled &&
      (snapshot.eye_care_status.phase === "SHORT_BREAK_DUE" || snapshot.eye_care_status.phase === "LONG_BREAK_DUE") &&
      !snapshot.eye_care_status.notification_suppressed && !reminderClaimInFlight) {
      reminderClaimInFlight = true;
      void invoke("supervisor_claim_eye_care_notification").catch(() => undefined).finally(() => {
        reminderClaimInFlight = false;
      });
    }
  });
  if (!nativeRuntime) {
    const browserMock = new MockSupervisorRuntime(parseMockScenario(window.location.search));
    void browserMock.poll().then(snapshot => {
      connected = snapshot.connected;
      semantic = snapshot.semantic ?? mockSemantic({ user_mode: snapshot.status?.user_mode ?? "STANDBY", task: snapshot.status?.task ?? "" });
      studySeconds = snapshot.status?.study_seconds;
      eyeCareSettings = snapshot.eye_care_settings;
      eyeCareStatus = snapshot.eye_care_status;
      renderEyeCareBubble();
    });
  }
  requestAnimationFrame(refresh);
}

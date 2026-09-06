import { useEffect, useRef, useState, type ReactElement } from "react";
import { createRoot } from "react-dom/client";
import { invoke } from "@tauri-apps/api/core";
import { QuickPanel, type QuickPanelMode } from "./QuickPanel";
import { SupervisorDashboardPollLoop, type SupervisorDashboardSnapshot } from "../transport/supervisor";
import { getMockScenario, getSupervisorControlAdapter, getSupervisorDashboardAdapter, isTauriRuntime } from "../runtime/adapters";
import { MockScenarioToolbar } from "../mock/MockScenarioToolbar";
import type { ControlCenterRoute } from "../center/route";
import type { TaskPickerAction, TaskPickerActionResult } from "../shared/TaskPicker";
import { useTaskSelectionState } from "../shared/use-task-selection-state";
import "../shared/theme/tokens.css";
import "../shared/task-picker.css";
import "./panel.css";

const root = document.querySelector<HTMLElement>("#quick-panel");
if (!root) throw new Error("Quick Panel root is missing");

const invokeWindowCommand = (command: string, args?: Record<string, unknown>): void => {
  if (isTauriRuntime) void invoke(command, args).catch(() => { /* bounded window command failure */ });
};
const openControlCenter = (route: ControlCenterRoute): void => {
  if (isTauriRuntime) { invokeWindowCommand("open_control_center", { route }); return; }
  const url = new URL("/control-center.html", window.location.origin);
  const scenario = getMockScenario();
  if (scenario) url.searchParams.set("mock", scenario);
  url.searchParams.set("route", route);
  window.open(url, "_blank", "noopener");
};
const closeQuickPanel = (): void => { if (isTauriRuntime) invokeWindowCommand("hide_quick_panel"); };
window.addEventListener("keydown", event => {
  if (event.key === "Escape") closeQuickPanel();
});

function formatElapsed(seconds: number): string {
  const safe = Math.max(0, Math.floor(seconds));
  const minutes = Math.floor(safe / 60);
  return `${Math.floor(minutes / 60).toString().padStart(2, "0")}:${(minutes % 60).toString().padStart(2, "0")}`;
}

function controlNotice(kind: string | undefined): string {
  switch (kind) {
    case "timeout": return "本地服务响应超时，请稍后再试";
    case "unauthorized": return "本地服务未授权，请检查运行状态";
    case "rejected": return "本次状态切换未被接受";
    case "invalid_response": return "本地服务返回了无法识别的结果";
    default: return "暂时无法切换状态，请稍后再试";
  }
}

function RuntimeQuickPanel(): ReactElement {
  const [snapshot, setSnapshot] = useState<SupervisorDashboardSnapshot>();
  const [notice, setNotice] = useState<string>();
  const pollerRef = useRef<SupervisorDashboardPollLoop | undefined>(undefined);
  const taskMutationRevision = useRef(0);

  useEffect(() => {
    let stopped = false;
    const adapter = getSupervisorDashboardAdapter();
    const poller = new SupervisorDashboardPollLoop(adapter, 1800);
    pollerRef.current = poller;
    poller.start(next => { if (!stopped) setSnapshot(next); });
    return () => {
      stopped = true;
      poller.stop();
      pollerRef.current = undefined;
    };
  }, []);

  const status = snapshot?.status;
  const motivation = snapshot?.motivation;
  const connected = Boolean(snapshot?.connected);
  const mode: QuickPanelMode = status?.user_mode ?? "STANDBY";
  const snapshotTask = status?.task || (connected ? "未设置任务" : "正在读取当前任务");
  const taskSelection = useTaskSelectionState(snapshotTask);
  const task = taskSelection.task;
  const elapsed = status ? formatElapsed(mode === "BREAK" ? status.break_seconds : status.study_seconds) : "--:--";
  const motivationAvailable = Boolean(motivation);
  const control = getSupervisorControlAdapter();

  const handleTaskResult = (operation: Promise<TaskPickerActionResult>): Promise<TaskPickerActionResult> => operation;
  const handleTaskPickerResult = async (result: TaskPickerActionResult, action: TaskPickerAction): Promise<void> => {
    setNotice(result.ok ? (action === "save" ? "常用任务已保存并选中" : "当前任务已更新") : controlNotice(result.error_kind));
    taskSelection.settle(result.ok, result.task);
    if (result.ok && result.task === undefined) {
      await pollerRef.current?.refresh();
    }
  };

  const handleModeAction = async (nextMode: "STUDY" | "BREAK" | "OFF"): Promise<void> => {
    if (!connected) {
      setNotice("本地服务尚未连接，请稍后再试");
      return;
    }
    setNotice("正在更新状态…");
    const result = nextMode === "STUDY"
      ? await control.setModeStudy(task === "未设置任务" ? "" : task)
      : nextMode === "BREAK" ? await control.setModeBreak() : await control.setModeOff();
    setNotice(result.ok ? "状态已更新" : controlNotice(result.error_kind));
  };

  return <QuickPanel
    mode={mode}
    task={task}
    elapsed={elapsed}
    focusMinutes={motivation?.today_credited_focus_minutes ?? 0}
    targetMinutes={motivation?.daily_target_minutes ?? 0}
    streakDays={motivation?.streak_days ?? 0}
    balanceAP={(motivation?.balance_ap_milli ?? 0) / 1000}
    connected={connected}
    motivationAvailable={motivationAvailable}
    notice={notice}
    taskPresets={snapshot?.task_presets}
    status={status}
    onSelectTask={id => handleTaskResult(control.selectTaskPreset(id))}
    onTemporaryTask={name => handleTaskResult(control.setTask(name))}
    onOptimisticTaskChange={nextTask => { if (nextTask !== undefined) taskSelection.selectOptimistically(nextTask); }}
    onTaskMutationStarted={() => { taskMutationRevision.current += 1; pollerRef.current?.markMutation(); }}
    onTaskResult={handleTaskPickerResult}
    onSaveTask={name => handleTaskResult(control.createTaskPreset(name, true).then(async result => {
      if (!result.ok) return result;
      const latest = await getSupervisorDashboardAdapter().poll();
      const created = latest.task_presets?.pinned.find(item => item.name.toLocaleLowerCase() === name.trim().replace(/\s+/g, " ").toLocaleLowerCase());
      return created ? control.selectTaskPreset(created.id) : control.setTask(name);
    }))}
    onModeAction={handleModeAction}
    onOpenCenter={() => openControlCenter("overview")}
    onOpenSettings={() => openControlCenter("settings")}
    onClose={closeQuickPanel}
  />;
}

const mockScenario = getMockScenario();
createRoot(root).render(<>{mockScenario && <MockScenarioToolbar scenario={mockScenario} />}<RuntimeQuickPanel /></>);

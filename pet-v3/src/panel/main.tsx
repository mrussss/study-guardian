import { useEffect, useState, type ReactElement } from "react";
import { createRoot } from "react-dom/client";
import { invoke } from "@tauri-apps/api/core";
import { QuickPanel, type QuickPanelMode } from "./QuickPanel";
import { NativeSupervisorControlAdapter, NativeSupervisorDashboardAdapter, type SupervisorDashboardSnapshot } from "../transport/supervisor";
import type { ControlCenterRoute } from "../center/route";
import "../shared/theme/tokens.css";
import "./panel.css";

const root = document.querySelector<HTMLElement>("#quick-panel");
if (!root) throw new Error("Quick Panel root is missing");

const isTauriRuntime = typeof window !== "undefined"
  && Object.prototype.hasOwnProperty.call(window, "__TAURI_INTERNALS__");
const invokeWindowCommand = (command: string, args?: Record<string, unknown>): void => {
  if (isTauriRuntime) void invoke(command, args).catch(() => { /* bounded window command failure */ });
};
const openControlCenter = (route: ControlCenterRoute): void => invokeWindowCommand("open_control_center", { route });
const closeQuickPanel = (): void => invokeWindowCommand("hide_quick_panel");
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

  useEffect(() => {
    let stopped = false;
    let inFlight = false;
    const adapter = new NativeSupervisorDashboardAdapter();
    const poll = async (): Promise<void> => {
      if (stopped || inFlight) return;
      inFlight = true;
      try {
        const next = await adapter.poll();
        if (!stopped) setSnapshot(next);
      } finally {
        inFlight = false;
      }
    };
    void poll();
    const timer = window.setInterval(() => void poll(), 1800);
    return () => {
      stopped = true;
      window.clearInterval(timer);
    };
  }, []);

  const status = snapshot?.status;
  const motivation = snapshot?.motivation;
  const connected = Boolean(snapshot?.connected && status);
  const mode: QuickPanelMode = status?.user_mode ?? "STANDBY";
  const task = status?.task || (connected ? "未设置任务" : "正在读取当前任务");
  const elapsed = status ? formatElapsed(mode === "BREAK" ? status.break_seconds : status.study_seconds) : "--:--";
  const motivationAvailable = Boolean(motivation);
  const control = new NativeSupervisorControlAdapter();

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
    onModeAction={handleModeAction}
    onOpenCenter={() => openControlCenter("overview")}
    onOpenSettings={() => openControlCenter("settings")}
    onClose={closeQuickPanel}
  />;
}

createRoot(root).render(isTauriRuntime
  ? <RuntimeQuickPanel />
  : <QuickPanel onOpenCenter={() => openControlCenter("overview")} onOpenSettings={() => openControlCenter("settings")} onClose={closeQuickPanel} />);

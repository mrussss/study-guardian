import type { ReactElement } from "react";
import type { NativeAutomationSettings, NativeSupervisorStatus } from "../transport/supervisor";

function formatThreshold(seconds: number): string {
  if (seconds % 60 === 0) return `${seconds / 60}分钟`;
  return `${seconds}秒`;
}

function formatRemaining(seconds: number): string {
  const minutes = Math.floor(seconds / 60).toString().padStart(2, "0");
  const remainder = (seconds % 60).toString().padStart(2, "0");
  return `${minutes}:${remainder}`;
}

export function automationPauseStatusText(status?: NativeSupervisorStatus, settings?: NativeAutomationSettings): string | undefined {
  if (!status || status.user_mode !== "STUDY" || !settings) return undefined;
  if (!settings.enabled || !settings.auto_pause.enabled) return "自动暂停未启用";
  if (status.pending_automation_intent?.transition === "AUTO_PAUSE") return undefined;

  const afkSeconds = Math.max(0, Math.floor(status.afk_seconds ?? 0));
  const threshold = status.interaction_state === "IDLE_STATIC"
    ? settings.auto_pause.idle_static_seconds
    : status.interaction_state === "IDLE_DYNAMIC"
      ? settings.auto_pause.idle_dynamic_seconds ?? 900
      : undefined;
  if (threshold === undefined) return "自动暂停等待中 · 尚未达到无输入阈值";
  if (afkSeconds <= 0) return `自动暂停等待中 · ${status.interaction_state === "IDLE_STATIC" ? "静态屏幕" : "动态屏幕"}阈值：${formatThreshold(threshold)}`;
  const remaining = Math.max(0, threshold - afkSeconds);
  return remaining > 0 ? `将在 ${formatRemaining(remaining)} 后自动暂停` : "自动暂停等待中 · 正在处理";
}

export function AutomationPauseStatus({ status, settings }: { status?: NativeSupervisorStatus; settings?: NativeAutomationSettings }): ReactElement | null {
  const text = automationPauseStatusText(status, settings);
  return text ? <p className="automation-pause-status" role="status">{text}</p> : null;
}

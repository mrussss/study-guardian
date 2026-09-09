import type { NativeSupervisorStatus } from "../../transport/supervisor";
import type { Activity } from "../../model/semantic";

export type SupervisionTone = "success" | "neutral" | "reminder" | "warning";

export interface SupervisionState {
  behaviorLabel: string;
  behaviorTone: SupervisionTone;
  systemLabel: string;
  systemTone: SupervisionTone;
}

const modeLabels: Record<NativeSupervisorStatus["user_mode"], string> = {
  STANDBY: "等待开始",
  STUDY: "学习中",
  BREAK: "休息中",
  OFF: "今日已结束",
};

export const interactionLabels: Record<NativeSupervisorStatus["interaction_state"], string> = {
  ACTIVE: "有输入",
  IDLE_STATIC: "暂时离开",
  IDLE_DYNAMIC: "无输入，屏幕仍活动",
  UNKNOWN: "状态暂不可用",
};

export const relationLabels: Record<NativeSupervisorStatus["task_relation"], string> = {
  FOCUSED: "专注当前任务",
  DISTRACTED: "可能偏离任务",
  UNKNOWN: "任务关系未知",
};

export const privacyLabels: Record<NativeSupervisorStatus["privacy_state"], string> = {
  NORMAL: "正常",
  SENSITIVE: "隐私保护中",
};

export const activityLabels: Record<Activity, string> = {
  CODING: "编程",
  ALGORITHM: "算法",
  READING: "阅读",
  WRITING: "写作",
  WATCHING: "视频学习",
  AI_ASSISTED: "AI 辅助学习",
  BROWSING: "资料浏览",
  MESSAGING: "即时通讯",
  GAMING: "游戏娱乐",
  GENERAL_STUDY: "学习",
  OTHER: "其他活动",
  UNKNOWN: "其他活动",
};


export function deriveSupervisionState(connected: boolean, status?: NativeSupervisorStatus): SupervisionState {
  if (!connected) {
    return { behaviorLabel: "监督离线", behaviorTone: "warning", systemLabel: "Supervisor 离线", systemTone: "warning" };
  }
  if (!status) {
    return { behaviorLabel: "状态暂不可用", behaviorTone: "warning", systemLabel: "Supervisor 已连接", systemTone: "success" };
  }

  const systemLabel = !status.screen_sensor_ok
    ? "屏幕采集异常"
    : !status.activitywatch_ok
      ? "活动数据异常"
      : "采集服务在线";
  const systemTone: SupervisionTone = systemLabel === "采集服务在线" ? "success" : "warning";
  const afkDuration = status.afk_seconds && status.afk_seconds > 0
    ? status.afk_seconds < 60 ? "不到 1 分钟" : `${Math.floor(status.afk_seconds / 60)} 分钟`
    : "";
  const idleLabel = (dynamic: boolean): string => afkDuration
    ? `已无输入 ${afkDuration} · ${dynamic ? "屏幕仍有变化" : "屏幕静止"}`
    : dynamic ? "无输入，屏幕仍活动" : "暂时离开";

  if (status.user_mode === "STANDBY") return { behaviorLabel: "等待开始", behaviorTone: "neutral", systemLabel, systemTone };
  if (status.user_mode === "BREAK") return { behaviorLabel: "休息中", behaviorTone: "neutral", systemLabel, systemTone };
  if (status.user_mode === "OFF") return { behaviorLabel: "今日已结束", behaviorTone: "neutral", systemLabel, systemTone };
  if (status.user_mode === "STUDY" && !status.activitywatch_ok) return { behaviorLabel: "状态暂不可用", behaviorTone: "warning", systemLabel, systemTone };
  if (status.privacy_state === "SENSITIVE") return { behaviorLabel: "隐私保护中", behaviorTone: "neutral", systemLabel, systemTone };
  if (status.interaction_state === "UNKNOWN") return { behaviorLabel: "状态暂不可用", behaviorTone: "warning", systemLabel, systemTone };
  if (status.interaction_state === "IDLE_STATIC") return { behaviorLabel: idleLabel(false), behaviorTone: "reminder", systemLabel, systemTone };
  if (status.interaction_state === "IDLE_DYNAMIC") return { behaviorLabel: idleLabel(true), behaviorTone: "neutral", systemLabel, systemTone };
  if (status.task_relation === "DISTRACTED") return { behaviorLabel: "可能偏离任务", behaviorTone: "reminder", systemLabel, systemTone };
  if (status.interaction_state === "ACTIVE" && status.task_relation === "FOCUSED") return { behaviorLabel: "正在专注", behaviorTone: "success", systemLabel, systemTone };
  return { behaviorLabel: "正在观察", behaviorTone: "neutral", systemLabel, systemTone };
}

export function formatLastActivity(value: string | undefined, nowMs = Date.now()): string {
  if (!value) return "暂无记录";
  const timestamp = Date.parse(value);
  if (!Number.isFinite(timestamp)) return "暂无记录";
  const elapsedSeconds = Math.max(0, Math.floor((nowMs - timestamp) / 1000));
  if (elapsedSeconds < 60) return "刚刚";
  if (elapsedSeconds < 3600) return `${Math.floor(elapsedSeconds / 60)} 分钟前`;
  if (elapsedSeconds < 86400) return `${Math.floor(elapsedSeconds / 3600)} 小时前`;
  return new Intl.DateTimeFormat("zh-CN", { month: "numeric", day: "numeric", hour: "2-digit", minute: "2-digit" }).format(timestamp);
}

export { modeLabels };

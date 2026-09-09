import type { ControlResult, NativeAutomationIntent, SupervisorDashboardSnapshot } from "../transport/supervisor";

export function automationDecisionNotice(
  pending: NativeAutomationIntent,
  accept: boolean,
  result: ControlResult | undefined,
  refreshed: SupervisorDashboardSnapshot | void | undefined,
): string {
  if (result?.ok) {
    if (pending.transition === "AUTO_PAUSE") return accept ? "已接受自动暂停" : "已拒绝自动暂停";
    if (pending.transition === "AUTO_START") return accept ? "已开始学习" : "已保持待机";
    return accept ? "已接受自动恢复" : "已拒绝自动恢复";
  }

  const status = refreshed?.status;
  const canonicalPending = status?.pending_automation_intent;
  if (status && canonicalPending?.intent_id !== pending.intent_id) {
    if (pending.transition === "AUTO_PAUSE" && status.user_mode === "BREAK") return "已自动暂停";
    if (pending.transition === "AUTO_START" && status.user_mode === "STUDY") return "已开始学习";
    if (pending.transition === "AUTO_RESUME" && status.user_mode === "STUDY") return "已恢复学习";
    if (pending.transition === "AUTO_START") return "开始请求已处理";
    return "自动转场请求已处理";
  }

  return "自动转场响应失败";
}

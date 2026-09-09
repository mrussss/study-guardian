import type { ControlResult, NativeAutomationIntent, SupervisorDashboardSnapshot } from "../transport/supervisor";

export function automationDecisionNotice(
  pending: NativeAutomationIntent,
  accept: boolean,
  result: ControlResult | undefined,
  refreshed: SupervisorDashboardSnapshot | void | undefined,
): string {
  const status = refreshed?.status;
  if (status) {
    const canonicalPending = status.pending_automation_intent;
    if (canonicalPending?.intent_id === pending.intent_id) {
      return "自动转场仍在等待处理";
    }

    if (pending.transition === "AUTO_PAUSE") {
      if (status.user_mode === "BREAK") return accept ? "已接受自动暂停" : "已自动暂停";
      if (status.user_mode === "STUDY") return accept ? "自动暂停请求已处理" : "已拒绝自动暂停";
      return "自动暂停请求已处理";
    }
    if (pending.transition === "AUTO_START") {
      if (status.user_mode === "STUDY") return "已开始学习";
      if (status.user_mode === "STANDBY") return accept ? "开始请求已处理" : "已保持待机";
      return "开始请求已处理";
    }
    if (status.user_mode === "STUDY") return accept ? "已接受自动恢复" : "已恢复学习";
    return "自动恢复请求已处理";
  }

  if (result?.ok) {
    if (pending.transition === "AUTO_PAUSE") return accept ? "已接受自动暂停" : "已拒绝自动暂停";
    if (pending.transition === "AUTO_START") return accept ? "已开始学习" : "已保持待机";
    return accept ? "已接受自动恢复" : "已拒绝自动恢复";
  }
  return "自动转场响应失败";
}

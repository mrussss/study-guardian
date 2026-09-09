import { useEffect, useState, type ReactElement } from "react";
import type { NativeAutomationIntent } from "../transport/supervisor";

export interface AutomationIntentPromptProps {
  pending?: NativeAutomationIntent;
  busy?: boolean;
  onAccept?: () => void | Promise<void>;
  onReject?: () => void | Promise<void>;
}

function secondsUntil(expiresAt: string): number {
  const timestamp = Date.parse(expiresAt);
  if (!Number.isFinite(timestamp)) return 0;
  return Math.max(0, Math.ceil((timestamp - Date.now()) / 1000));
}

export function AutomationIntentPrompt({ pending, busy = false, onAccept, onReject }: AutomationIntentPromptProps): ReactElement | null {
  const [seconds, setSeconds] = useState(() => pending ? secondsUntil(pending.expires_at) : 0);

  useEffect(() => {
    if (!pending) {
      setSeconds(0);
      return;
    }
    const update = (): void => setSeconds(secondsUntil(pending.expires_at));
    update();
    const timer = window.setInterval(update, 250);
    return () => window.clearInterval(timer);
  }, [pending?.intent_id, pending?.expires_at]);

  if (!pending) return null;
  const isPause = pending.transition === "AUTO_PAUSE";
  const title = isPause ? "检测到你可能已离开" : "检测到你正在学习";
  const description = isPause
    ? "将在 " + seconds + " 秒后暂停计时"
    : "将在 " + seconds + " 秒后取消开始";
  const acceptLabel = isPause ? "立即暂停" : "开始计时";
  const rejectLabel = isPause ? "继续计时" : "保持待机";

  return <section className={"automation-intent-prompt is-" + (isPause ? "pause" : "start")} role="alert" aria-live="assertive" aria-label="自动化状态确认">
    <div className="automation-intent-copy"><strong>{title}</strong><span>{description}</span></div>
    <div className="automation-intent-actions">
      <button className="secondary-button" type="button" disabled={busy} onClick={() => void onReject?.()}>{rejectLabel}</button>
      <button className="primary-button" type="button" disabled={busy} onClick={() => void onAccept?.()}>{acceptLabel}</button>
    </div>
  </section>;
}

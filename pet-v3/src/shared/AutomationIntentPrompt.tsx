import { useEffect, useRef, useState, type ReactElement } from "react";
import type { NativeAutomationIntent } from "../transport/supervisor";

export interface AutomationIntentPromptProps {
  pending?: NativeAutomationIntent;
  busy?: boolean;
  onAccept?: () => void | Promise<void>;
  onReject?: () => void | Promise<void>;
  onExpired?: () => void | Promise<void>;
}

function secondsUntil(expiresAt: string): number {
  const timestamp = Date.parse(expiresAt);
  if (!Number.isFinite(timestamp)) return 0;
  return Math.max(0, Math.ceil((timestamp - Date.now()) / 1000));
}

export function AutomationIntentPrompt({ pending, busy = false, onAccept, onReject, onExpired }: AutomationIntentPromptProps): ReactElement | null {
  const [seconds, setSeconds] = useState(() => pending ? secondsUntil(pending.expires_at) : 0);
  const expirationNotifiedRef = useRef("");

  useEffect(() => {
    expirationNotifiedRef.current = "";
    if (!pending) {
      setSeconds(0);
      return;
    }
    const update = (): void => {
      const next = secondsUntil(pending.expires_at);
      setSeconds(next);
      if (next === 0 && expirationNotifiedRef.current !== pending.intent_id) {
        expirationNotifiedRef.current = pending.intent_id;
        void onExpired?.();
      }
    };
    update();
    const timer = window.setInterval(update, 250);
    return () => window.clearInterval(timer);
  }, [pending?.intent_id, pending?.expires_at]);

  if (!pending) return null;
  const isPause = pending.transition === "AUTO_PAUSE";
  const expired = seconds === 0;
  const title = isPause ? "检测到你可能已离开" : "检测到你正在学习";
  const description = expired
    ? (isPause ? "正在应用自动暂停…" : "正在清除开始请求…")
    : (isPause ? "将在 " + seconds + " 秒后暂停计时" : "将在 " + seconds + " 秒后取消开始");
  const acceptLabel = isPause ? "立即暂停" : "开始计时";
  const rejectLabel = isPause ? "继续计时" : "保持待机";
  const actionsDisabled = busy || expired;

  return <section className={"automation-intent-prompt is-" + (isPause ? "pause" : "start")} role="alert" aria-live="assertive" aria-label="自动化状态确认" aria-busy={actionsDisabled}>
    <div className="automation-intent-copy"><strong>{title}</strong><span>{description}</span></div>
    <div className="automation-intent-actions">
      <button className="secondary-button" type="button" disabled={actionsDisabled} onClick={() => void onReject?.()}>{rejectLabel}</button>
      <button className="primary-button" type="button" disabled={actionsDisabled} onClick={() => void onAccept?.()}>{acceptLabel}</button>
    </div>
  </section>;
}

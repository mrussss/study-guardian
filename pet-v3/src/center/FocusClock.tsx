import { useEffect, useRef, useState, type ReactElement } from "react";
import type { NativeMotivationStatus, NativeSupervisorStatus } from "../transport/supervisor";
import { formatFocusMinutes } from "../shared/models/dashboard";

const SMOOTH_CORRECTION_SECONDS = 3;

export type FocusClockAnchor = {
  serverSeconds: number;
  receivedAt: number;
  correctionSeconds: number;
  running: boolean;
};

function safeSeconds(seconds: number): number {
  return Number.isFinite(seconds) ? Math.max(0, Math.floor(seconds)) : 0;
}

export function formatSessionClock(seconds: number): string {
  const safe = safeSeconds(seconds);
  const minutes = Math.floor(safe / 60);
  return `${Math.floor(minutes / 60).toString().padStart(2, "0")}:${(minutes % 60).toString().padStart(2, "0")}:${(safe % 60).toString().padStart(2, "0")}`;
}

export function isEffectiveFocus(connected: boolean, status?: NativeSupervisorStatus): boolean {
  return connected && status?.user_mode === "STUDY" && status.interaction_state === "ACTIVE" && status.privacy_state === "NORMAL" && status.task_relation === "FOCUSED";
}

export function getAnchoredDisplaySeconds(anchor: FocusClockAnchor, now: number): number {
  const elapsed = anchor.running ? Math.max(0, now - anchor.receivedAt) / 1000 : 0;
  const correctionProgress = Math.min(1, elapsed / SMOOTH_CORRECTION_SECONDS);
  const correction = anchor.correctionSeconds * (1 - correctionProgress);
  return Math.max(0, anchor.serverSeconds + elapsed + correction);
}

export function syncFocusClockAnchor(previous: FocusClockAnchor | undefined, serverSeconds: number, running: boolean, receivedAt: number): FocusClockAnchor {
  const safe = safeSeconds(serverSeconds);
  if (!running || !previous || !previous.running) return { serverSeconds: safe, receivedAt, correctionSeconds: 0, running };
  const predicted = getAnchoredDisplaySeconds(previous, receivedAt);
  const difference = predicted - safe;
  if (Math.abs(difference) > SMOOTH_CORRECTION_SECONDS) return { serverSeconds: safe, receivedAt, correctionSeconds: 0, running };
  return { serverSeconds: safe, receivedAt, correctionSeconds: difference, running };
}

export function secondProgress(displaySeconds: number): number {
  const second = Math.max(0, displaySeconds) % 60;
  return second / 60;
}

function nowMilliseconds(): number {
  return typeof performance === "undefined" ? Date.now() : performance.now();
}

export function FocusClock({ connected, status, motivation }: { connected: boolean; status?: NativeSupervisorStatus; motivation?: NativeMotivationStatus }): ReactElement {
  const running = isEffectiveFocus(connected, status);
  const serverSeconds = safeSeconds(status?.study_seconds ?? 0);
  const anchorRef = useRef<FocusClockAnchor | undefined>(undefined);
  const [displaySeconds, setDisplaySeconds] = useState(serverSeconds);
  const ringRadius = 56;
  const ringCircumference = 2 * Math.PI * ringRadius;

  useEffect(() => {
    const receivedAt = nowMilliseconds();
    const anchor = syncFocusClockAnchor(anchorRef.current, serverSeconds, running, receivedAt);
    anchorRef.current = anchor;
    setDisplaySeconds(getAnchoredDisplaySeconds(anchor, receivedAt));
    const timer = window.setInterval(() => {
      const current = anchorRef.current;
      if (current) setDisplaySeconds(getAnchoredDisplaySeconds(current, nowMilliseconds()));
    }, 200);
    return () => window.clearInterval(timer);
  }, [running, serverSeconds]);

  const progress = secondProgress(displaySeconds);
  const progressStyle = {
    strokeDasharray: ringCircumference,
    strokeDashoffset: ringCircumference * (1 - progress),
  };
  const timeLabel = connected && status ? formatSessionClock(displaySeconds) : "--:--:--";
  const stateLabel = running ? "正在记录有效专注" : status?.user_mode === "BREAK" ? "休息中，秒表暂停" : connected ? "秒表已暂停" : "等待 Supervisor 快照";

  return <section className={`focus-clock ${running ? "is-running" : "is-paused"}`} aria-label="专注秒表">
    <div className="focus-clock-face">
      <svg className="focus-clock-ring" viewBox="0 0 140 140" aria-hidden="true">
        <defs><linearGradient id="focusClockGradient" x1="0" y1="0" x2="1" y2="1"><stop offset="0%" stopColor="#c6beff" /><stop offset="100%" stopColor="#8d7dff" /></linearGradient></defs>
        <circle className="focus-clock-track" cx="70" cy="70" r={ringRadius} />
        <circle className="focus-clock-progress" cx="70" cy="70" r={ringRadius} style={progressStyle} />
      </svg>
      <div className="focus-clock-center"><span>本次专注</span><strong>{timeLabel}</strong></div>
    </div>
    <div className="focus-clock-today"><span>今日累计</span><strong>{connected && motivation ? formatFocusMinutes(motivation.today_credited_focus_minutes) : "—"}</strong></div>
    <span className="focus-clock-state">{stateLabel}</span>
  </section>;
}

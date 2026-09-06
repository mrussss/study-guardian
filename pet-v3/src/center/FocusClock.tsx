import type { CSSProperties, ReactElement } from "react";
import type { NativeMotivationStatus, NativeSupervisorStatus } from "../transport/supervisor";
import { formatFocusMinutes } from "../shared/models/dashboard";

export function formatSessionClock(seconds: number): string {
  const safe = Math.max(0, Math.floor(seconds));
  const minutes = Math.floor(safe / 60);
  return `${Math.floor(minutes / 60).toString().padStart(2, "0")}:${(minutes % 60).toString().padStart(2, "0")}:${(safe % 60).toString().padStart(2, "0")}`;
}

export function FocusClock({ connected, status, motivation }: { connected: boolean; status?: NativeSupervisorStatus; motivation?: NativeMotivationStatus }): ReactElement {
  const sessionSeconds = status?.study_seconds ?? 0;
  const active = connected && status?.user_mode === "STUDY" && status.interaction_state === "ACTIVE" && status.privacy_state === "NORMAL" && status.task_relation === "FOCUSED";
  const handAngle = (sessionSeconds % 60) * 6;
  return <section className={`focus-clock ${active ? "is-running" : "is-paused"}`} aria-label="专注时钟">
    <div className="focus-clock-face" style={{ "--focus-hand-angle": `${handAngle}deg` } as CSSProperties}>
      <span className="focus-clock-tick tick-1" /><span className="focus-clock-tick tick-2" /><span className="focus-clock-tick tick-3" /><span className="focus-clock-tick tick-4" />
      <span className="focus-clock-hand" /><span className="focus-clock-pin" />
      <div className="focus-clock-center"><span>今日已专注</span><strong>{connected && motivation ? formatFocusMinutes(motivation.today_credited_focus_minutes) : "—"}</strong><small>本次 {connected && status ? formatSessionClock(sessionSeconds) : "—"}</small></div>
    </div>
    <span className="focus-clock-state">{active ? "正在记录有效专注" : status?.user_mode === "BREAK" ? "休息中，时钟暂停" : "等待 Supervisor 快照"}</span>
  </section>;
}

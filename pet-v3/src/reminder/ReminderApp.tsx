import { useEffect, useMemo, useState, type ReactElement } from "react";
import { invoke } from "@tauri-apps/api/core";
import { NativeSupervisorAdapter, NativeSupervisorControlAdapter, type PetTransportSnapshot, type SupervisorAdapter, type SupervisorControlAdapter, type EyeCareAction } from "../transport/supervisor";

const empty: PetTransportSnapshot = { connected: false, semantic: { schema_version: 1, observed_at: new Date(0).toISOString(), fresh: false, user_mode: "STANDBY", task: "", interaction: "UNKNOWN", relation: "UNKNOWN", privacy: "NORMAL", activity: "UNKNOWN", confidence: 0 } };
const hideNative = (): Promise<void> => invoke("hide_eye_care_reminder");

export function ReminderApp({ adapter: suppliedAdapter, control: suppliedControl, hide = hideNative, pollMilliseconds = 1800 }: {
  adapter?: SupervisorAdapter; control?: Pick<SupervisorControlAdapter, "eyeCareAction">; hide?: () => Promise<void>; pollMilliseconds?: number;
} = {}): ReactElement {
  const adapter = useMemo(() => suppliedAdapter ?? new NativeSupervisorAdapter(), [suppliedAdapter]);
  const control = useMemo(() => suppliedControl ?? new NativeSupervisorControlAdapter(), [suppliedControl]);
  const [snapshot, setSnapshot] = useState(empty);
  const [busy, setBusy] = useState(false);
  const [notice, setNotice] = useState("");
  const dismiss = async (): Promise<void> => {
    try { await hide(); } catch { setNotice("窗口暂时未能收起，请重试关闭。"); }
  };
  useEffect(() => {
    let active = true;
    let timer: ReturnType<typeof setTimeout> | undefined;
    const poll = async (): Promise<void> => {
      try {
        const next = await adapter.poll();
        if (!active) return;
        setSnapshot(next);
        const eye = next.eye_care_status;
        const isDue = eye?.phase === "SHORT_BREAK_DUE" || eye?.phase === "LONG_BREAK_DUE";
        if (next.connected && eye && (!isDue || eye.notification_suppressed)) {
          try { await hide(); } catch { if (active) setNotice("窗口暂时未能收起，请重试关闭。"); }
        }
      } catch { if (active) setNotice("连接暂时中断，恢复后可以继续操作。"); }
      finally { if (active) timer = setTimeout(() => void poll(), pollMilliseconds); }
    };
    void poll();
    return () => { active = false; if (timer !== undefined) clearTimeout(timer); };
  }, [adapter, hide, pollMilliseconds]);
  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent): void => {
      if (event.key === "Escape") { event.preventDefault(); void dismiss(); }
    };
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [hide]);
  const status = snapshot.eye_care_status;
  const settings = snapshot.eye_care_settings;
  const long = status?.phase === "LONG_BREAK_DUE";
  const due = status?.phase === "SHORT_BREAK_DUE" || status?.phase === "LONG_BREAK_DUE";
  const available = snapshot.connected && due && !status?.notification_suppressed;
  const run = async (action: EyeCareAction): Promise<void> => {
    if (busy || !available || !status || !control.eyeCareAction) return;
    setBusy(true); setNotice("");
    try {
      const id = typeof crypto?.randomUUID === "function" ? crypto.randomUUID() : `eye-${Date.now()}-${Math.random().toString(36).slice(2)}`;
      const result = await control.eyeCareAction(action, status.revision, id);
      if (!result.ok) {
        setNotice("状态已更新，请稍候刷新后重试。");
        setSnapshot(await adapter.poll());
        return;
      }
      // Use our bounded native command, not window.hide without window permission.
      await hide();
    } catch { setNotice("操作或窗口关闭未完成，请重试。"); }
    finally { setBusy(false); }
  };
  return <main className="reminder-shell" role="dialog" aria-label="护眼提醒">
    <section className="reminder-card">
      <button className="reminder-close" type="button" aria-label="关闭护眼提醒" onClick={() => void dismiss()}>×</button>
      <div className="reminder-copy">
        <h1>{long ? "该安排一次完整休息了" : "该让眼睛休息一下了"}</h1>
        <p>{long ? `已累计操作电脑约 ${settings?.long_break_after_focus_minutes ?? 120} 分钟，建议离开屏幕休息 ${settings?.long_break_minutes ?? 20} 分钟。` : `已累计操作电脑约 ${settings?.focus_minutes ?? 40} 分钟，起来走走、看看远处吧。`}</p>
      </div>
      <div className="reminder-actions">
        <button className="reminder-primary" type="button" disabled={busy || !available} onClick={() => void run(long ? "START_LONG_BREAK" : "START_SHORT_BREAK")}>开始休息</button>
        <button type="button" disabled={busy || !available || (status?.snooze_count ?? 0) >= (settings?.max_snoozes ?? 2)} onClick={() => void run("SNOOZE")}>延后 {settings?.snooze_minutes ?? 5} 分钟</button>
        <button type="button" disabled={busy || !available} onClick={() => void run("SKIP")}>跳过本次</button>
      </div>
      <p className="reminder-hint">无输入时暂停计时；关闭提醒后，到期仍会再提醒。</p>
      {!snapshot.connected && <p className="reminder-notice" role="status">正在连接，请稍候…</p>}
      {notice && <p className="reminder-notice" role="status">{notice}</p>}
    </section>
  </main>;
}

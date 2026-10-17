import { useEffect, useMemo, useState, type ReactElement } from "react";
import { getCurrentWindow } from "@tauri-apps/api/window";
import { Eye, Moon } from "lucide-react";
import { NativeSupervisorAdapter, NativeSupervisorControlAdapter, type NativeEyeCareStatus, type PetTransportSnapshot } from "../transport/supervisor";
import type { EyeCareAction } from "../transport/supervisor";

function requestId(): string {
  try {
    if (typeof crypto !== "undefined" && "randomUUID" in crypto) return crypto.randomUUID();
  } catch { /* deterministic fallback is not a secret */ }
  return `reminder-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 10)}`;
}

function formatMinutes(minutes: number): string {
  return `${minutes} 分钟`;
}

function hideReminder(): void {
  void getCurrentWindow().hide();
}

function due(status?: NativeEyeCareStatus): boolean {
  return status?.phase === "SHORT_BREAK_DUE" || status?.phase === "LONG_BREAK_DUE";
}

export function ReminderApp(): ReactElement {
  const adapter = useMemo(() => new NativeSupervisorAdapter(), []);
  const control = useMemo(() => new NativeSupervisorControlAdapter(), []);
  const [snapshot, setSnapshot] = useState<PetTransportSnapshot>({ connected: false, semantic: { schema_version: 1, observed_at: new Date(0).toISOString(), fresh: false, user_mode: "STANDBY", task: "", interaction: "UNKNOWN", relation: "UNKNOWN", privacy: "NORMAL", activity: "UNKNOWN", confidence: 0 } });
  const [busy, setBusy] = useState(false);
  const [notice, setNotice] = useState("");

  useEffect(() => {
    let active = true;
    let timer: ReturnType<typeof setTimeout> | null = null;
    const poll = async (): Promise<void> => {
      const next = await adapter.poll();
      if (!active) return;
      setSnapshot(next);
      if (!due(next.eye_care_status)) {
        hideReminder();
        return;
      }
      timer = setTimeout(() => void poll(), 1800);
    };
    void poll();
    return () => { active = false; if (timer !== null) clearTimeout(timer); };
  }, [adapter]);

  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent): void => {
      if (event.key === "Escape") {
        event.preventDefault();
        hideReminder();
      }
    };
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, []);

  const status = snapshot.eye_care_status;
  const settings = snapshot.eye_care_settings;
  const long = status?.phase === "LONG_BREAK_DUE";
  const title = long ? "该安排一次完整休息了" : "该让眼睛休息一下了";
  const message = long
    ? `今天已经累计使用电脑 ${formatMinutes(settings?.long_break_after_focus_minutes ?? 120)}。建议离开屏幕 ${settings?.long_break_minutes ?? 20} 分钟，起身走动、喝水、看看室外。`
    : `已经连续使用电脑 ${formatMinutes(settings?.focus_minutes ?? 40)}。出去走走，看看远处，让眼睛真正离开近距离屏幕。`;
  const run = async (action: EyeCareAction): Promise<void> => {
    if (busy || !status) return;
    setBusy(true);
    setNotice("");
    const result = await control.eyeCareAction(action, status.revision, requestId());
    if (!result.ok) {
      setNotice("状态已变化，请稍候刷新");
      setBusy(false);
      return;
    }
    hideReminder();
  };

  return <main className="reminder-shell" role="dialog" aria-label="护眼提醒">
    <section className="reminder-card">
      <div className="reminder-icon" aria-hidden="true">{long ? <Moon size={22} /> : <Eye size={22} />}</div>
      <div className="reminder-copy">
        <h1>{title}</h1>
        <p>{message}</p>
      </div>
      <div className="reminder-actions">
        <button className="reminder-primary" type="button" disabled={busy} onClick={() => void run(long ? "START_LONG_BREAK" : "START_SHORT_BREAK")}>开始休息</button>
        <button type="button" disabled={busy} onClick={() => void run("SNOOZE")}>延后 5 分钟</button>
        <button type="button" disabled={busy} onClick={() => void run("SKIP")}>跳过本次</button>
      </div>
      <p className="reminder-hint">关闭窗口不会跳过提醒，5 分钟后会再次提示。</p>
      {notice && <p className="reminder-notice" role="status">{notice}</p>}
    </section>
  </main>;
}

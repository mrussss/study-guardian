import { useEffect, useRef, useState, type ReactElement } from "react";
import { Eye, Moon, RefreshCw } from "lucide-react";
import { getSupervisorControlAdapter } from "../runtime/adapters";
import type { NativeEyeCareSettings, NativeEyeCareStatus, NativeSupervisorStatus, SupervisorDashboardSnapshot } from "../transport/supervisor";

export type EyeCareRefresh = () => Promise<SupervisorDashboardSnapshot | void>;

const defaultSettings: NativeEyeCareSettings = {
  enabled: false, focus_minutes: 40, short_break_minutes: 5,
  long_break_after_focus_minutes: 120, long_break_minutes: 20,
  snooze_minutes: 5, max_snoozes: 2,
};

function formatDuration(seconds: number): string {
  const safe = Math.max(0, Math.floor(seconds));
  const hours = Math.floor(safe / 3600);
  const minutes = Math.floor((safe % 3600) / 60);
  const remainder = safe % 60;
  return hours > 0
    ? `${hours}:${String(minutes).padStart(2, "0")}:${String(remainder).padStart(2, "0")}`
    : `${String(minutes).padStart(2, "0")}:${String(remainder).padStart(2, "0")}`;
}

function makeRequestId(): string {
  try {
    if (typeof crypto !== "undefined" && "randomUUID" in crypto) return crypto.randomUUID();
  } catch { /* use a non-secret local fallback */ }
  return `eye-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 12)}`;
}

function statusActionLabel(status?: NativeEyeCareStatus): string {
  switch (status?.phase) {
    case "SHORT_BREAK_DUE": return "该远眺休息 5 分钟了";
    case "LONG_BREAK_DUE": return "该安排一次完整休息了";
    case "SHORT_BREAK": return "远眺休息中";
    case "LONG_BREAK": return "完整休息中";
    case "WAITING_RETURN": return "本轮休息计时完成";
    default: return "护眼节奏已开启";
  }
}

export function EyeCareWidget({
  connected, settings, eyeCareStatus, supervisorStatus, userMode, onRefresh, compact = false,
  control = getSupervisorControlAdapter(),
}: {
  connected: boolean;
  settings?: NativeEyeCareSettings;
  eyeCareStatus?: NativeEyeCareStatus;
  supervisorStatus?: NativeSupervisorStatus;
  userMode?: NativeSupervisorStatus["user_mode"];
  onRefresh?: EyeCareRefresh;
  compact?: boolean;
  control?: ReturnType<typeof getSupervisorControlAdapter>;
}): ReactElement | null {
  const lastSettings = useRef<NativeEyeCareSettings | undefined>(undefined);
  const lastStatus = useRef<NativeEyeCareStatus | undefined>(undefined);
  if (settings) lastSettings.current = settings;
  if (eyeCareStatus) lastStatus.current = eyeCareStatus;
  const currentSettings = settings ?? lastSettings.current;
  const currentStatus = eyeCareStatus ?? lastStatus.current;
  const enabled = currentSettings?.enabled === true || currentStatus?.enabled === true;
  const authoritative = connected && settings !== undefined && eyeCareStatus !== undefined;
  const focusDataAvailable = supervisorStatus?.activitywatch_ok === true && supervisorStatus.activitywatch_stable_ok !== false;
  const [now, setNow] = useState(() => Date.now());
  const [busy, setBusy] = useState(false);
  const [notice, setNotice] = useState("");

  useEffect(() => {
    if (!authoritative || !enabled) return;
    const timer = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(timer);
  }, [authoritative, enabled]);

  if (!enabled || !currentStatus) return null;
  const phase = currentStatus.phase;
  const values = currentSettings ?? defaultSettings;
  const focusRemaining = Math.max(0, values.focus_minutes * 60 - currentStatus.focus_segment_seconds);
  const timestampRemaining = (value?: string): number | undefined => {
    if (!value) return undefined;
    const parsed = Date.parse(value);
    return Number.isFinite(parsed) ? Math.max(0, Math.ceil((parsed - now) / 1000)) : undefined;
  };
  const restRemaining = timestampRemaining(currentStatus.planned_break_end_at);
  const focusUnavailable = authoritative && userMode === "STUDY" && !focusDataAvailable;

  const runAction = async (action: Parameters<NonNullable<typeof control.eyeCareAction>>[0]): Promise<void> => {
    if (busy || !authoritative || !control.eyeCareAction) return;
    setBusy(true);
    setNotice("");
    const result = await control.eyeCareAction(action, currentStatus.revision, makeRequestId());
    if (!result.ok) {
      setNotice(result.error_kind === "rejected" ? "状态已变化，请刷新后重试" : "操作暂时无法完成，请稍后重试");
      setBusy(false);
      return;
    }
    await onRefresh?.();
    setBusy(false);
  };

  const duePhase = phase === "SHORT_BREAK_DUE" || phase === "LONG_BREAK_DUE";
  const snoozeRemaining = timestampRemaining(currentStatus.snooze_until);
  const snoozed = duePhase && snoozeRemaining !== undefined && snoozeRemaining > 0;
  const due = duePhase && !snoozed;
  const rest = phase === "SHORT_BREAK" || phase === "LONG_BREAK";
  const dueActionsAllowed = authoritative && (userMode === undefined || userMode === "STUDY");
  const phaseTitle = phase === "SHORT_BREAK_DUE" ? `该远眺休息 ${values.short_break_minutes} 分钟了`
    : phase === "LONG_BREAK_DUE" ? "该安排一次完整休息了"
      : phase === "FOCUSING" && userMode === "OFF" ? "今天已结束，护眼累计已保留"
        : phase === "FOCUSING" && userMode !== undefined && userMode !== "STUDY" ? "等待继续学习后再累计"
          : statusActionLabel(currentStatus);

  return <section className={`eye-care-widget${compact ? " is-compact" : ""} is-${due ? "due" : rest ? "rest" : "calm"}`} aria-label="护眼节奏">
    <div className="eye-care-copy">
      <span className="eye-care-icon" aria-hidden="true">{phase === "LONG_BREAK" || phase === "LONG_BREAK_DUE" ? <Moon size={16} /> : <Eye size={16} />}</span>
      <div className="eye-care-copy-main">
        <strong>{!authoritative ? "护眼状态暂不可用" : focusUnavailable ? "等待有效专注数据" : snoozed ? "护眼提醒已延后" : phaseTitle}</strong>
        <span>
          {!authoritative ? "连接恢复后将继续显示最新状态" : focusUnavailable ? "数据恢复前不会按墙上时间补算" : snoozed ? `约 ${formatDuration(snoozeRemaining ?? 0)} 后再次提醒` : phase === "FOCUSING" && userMode === "STUDY" ? `距离远眺休息 ${formatDuration(focusRemaining)}` : phase === "FOCUSING" ? "开始有效专注后会继续累计。" : phase === "SHORT_BREAK_DUE" ? "起来走动一下，看看远处；休息时尽量少看手机。" : phase === "LONG_BREAK_DUE" ? "起身走动、喝水，给自己一段完整休息。" : rest ? (restRemaining === undefined ? "看看远处，休息时尽量少看手机。" : `剩余 ${formatDuration(restRemaining)} · 休息时尽量少看手机。`) : phase === "WAITING_RETURN" ? "准备好后，由你决定何时继续学习。" : "有效专注时长达到后会在这里提醒。"}
        </span>
      </div>
    </div>
    {authoritative && due && <div className="eye-care-actions">
      {dueActionsAllowed && <>
        <button className="eye-care-primary" type="button" disabled={busy} onClick={() => void runAction(phase === "LONG_BREAK_DUE" ? "START_LONG_BREAK" : "START_SHORT_BREAK")}>{phase === "LONG_BREAK_DUE" ? "开始完整休息" : "开始远眺"}</button>
        {currentStatus.snooze_count < values.max_snoozes && <button type="button" disabled={busy} onClick={() => void runAction("SNOOZE")}>延后 {values.snooze_minutes} 分钟</button>}
        <button type="button" disabled={busy} onClick={() => void runAction("SKIP")}>跳过</button>
      </>}
    </div>}
    {authoritative && rest && <div className="eye-care-countdown" aria-live="off">{restRemaining === undefined ? "--:--" : formatDuration(restRemaining)}<button type="button" disabled={busy} onClick={() => void runAction("FINISH_EARLY")}>提前结束</button></div>}
    {authoritative && phase === "WAITING_RETURN" && <div className="eye-care-actions"><button className="eye-care-primary" type="button" disabled={busy} onClick={() => void runAction("RESUME_STUDY")}>继续学习</button></div>}
    {!authoritative && <span className="eye-care-offline"><RefreshCw size={13} />状态未连接</span>}
    {notice && <span className="eye-care-notice" role="status">{notice}</span>}
  </section>;
}

export function EyeCareSettingsCard({ settings: source, connected, onRefresh, control = getSupervisorControlAdapter() }: {
  settings?: NativeEyeCareSettings;
  connected: boolean;
  onRefresh?: EyeCareRefresh;
  control?: ReturnType<typeof getSupervisorControlAdapter>;
}): ReactElement {
  const [draft, setDraft] = useState<NativeEyeCareSettings>(() => ({ ...defaultSettings, ...(source ?? {}) }));
  const [dirty, setDirty] = useState(false);
  const [busy, setBusy] = useState(false);
  const [notice, setNotice] = useState("");
  const sourceFingerprint = useRef(source ? JSON.stringify(source) : undefined);
  useEffect(() => {
    const fingerprint = source ? JSON.stringify(source) : undefined;
    if (fingerprint && fingerprint !== sourceFingerprint.current) {
      sourceFingerprint.current = fingerprint;
      if (!dirty) setDraft({ ...defaultSettings, ...source });
    }
  }, [source, dirty]);

  const update = <K extends keyof NativeEyeCareSettings>(key: K, value: NativeEyeCareSettings[K]): void => {
    setDraft(current => ({ ...current, [key]: value }));
    setDirty(true);
    setNotice("");
  };
  const save = async (): Promise<void> => {
    if (busy || !connected || !control.saveEyeCareSettings) return;
    setBusy(true);
    setNotice("");
    const result = await control.saveEyeCareSettings(draft);
    if (!result.ok) {
      setNotice(result.error_kind === "rejected" ? "设置未通过校验，请检查数值" : "保存失败，已保留当前草稿");
      setBusy(false);
      return;
    }
    const refreshed = await onRefresh?.();
    const canonical = refreshed?.eye_care_settings;
    if (canonical) setDraft({ ...canonical });
    setDirty(false);
    setNotice(canonical ? "护眼设置已保存" : "已保存，正在等待服务确认");
    setBusy(false);
  };

  return <section className="surface-section eye-care-settings-card" aria-labelledby="eye-care-settings-title">
    <div className="section-header"><div><h2 id="eye-care-settings-title">护眼节奏</h2><p>按有效专注时间提醒，休息结束后由你决定何时继续</p></div><label className="eye-care-toggle"><input type="checkbox" checked={draft.enabled} disabled={!connected || busy} onChange={event => update("enabled", event.target.checked)} /><span>开启</span></label></div>
    <div className="eye-care-settings-summary"><strong>推荐节奏</strong><span>有效专注 {draft.focus_minutes} 分钟 → 远眺 {draft.short_break_minutes} 分钟</span><span>累计专注 {draft.long_break_after_focus_minutes} 分钟 → 完整休息 {draft.long_break_minutes} 分钟</span></div>
    <div className="eye-care-setting-grid">
      <label>有效专注间隔（分钟）<input aria-label="有效专注间隔（分钟）" type="number" min={20} max={90} step={1} value={draft.focus_minutes} disabled={!connected || busy} onChange={event => update("focus_minutes", Number(event.target.value))} /></label>
      <label>远眺休息（分钟）<input aria-label="远眺休息（分钟）" type="number" min={1} max={20} step={1} value={draft.short_break_minutes} disabled={!connected || busy} onChange={event => update("short_break_minutes", Number(event.target.value))} /></label>
      <label>完整休息间隔（分钟）<input aria-label="完整休息间隔（分钟）" type="number" min={60} max={240} step={1} value={draft.long_break_after_focus_minutes} disabled={!connected || busy} onChange={event => update("long_break_after_focus_minutes", Number(event.target.value))} /></label>
      <label>完整休息时长（分钟）<input aria-label="完整休息时长（分钟）" type="number" min={5} max={60} step={1} value={draft.long_break_minutes} disabled={!connected || busy} onChange={event => update("long_break_minutes", Number(event.target.value))} /></label>
      <label>延后时长（分钟）<input aria-label="延后时长（分钟）" type="number" min={1} max={30} step={1} value={draft.snooze_minutes} disabled={!connected || busy} onChange={event => update("snooze_minutes", Number(event.target.value))} /></label>
      <label>最多延后次数<input aria-label="最多延后次数" type="number" min={0} max={5} step={1} value={draft.max_snoozes} disabled={!connected || busy} onChange={event => update("max_snoozes", Number(event.target.value))} /></label>
    </div>
    <p className="eye-care-settings-help">护眼节奏只依据系统已确认的有效专注累计；不会验证你是否看远，也不会新增摄像头采集、截图或 AI 请求。</p>
    <div className="setting-actions"><button className="primary-button" type="button" disabled={!connected || busy || !dirty} onClick={() => void save()}>{busy ? "正在保存…" : "保存护眼设置"}</button>{notice && <span role="status">{notice}</span>}</div>
  </section>;
}

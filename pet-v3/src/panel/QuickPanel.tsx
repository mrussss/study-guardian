import { useState, type ReactElement } from "react";
import {
  ArrowUpRight,
  BookOpen,
  CheckCircle2,
  ChevronRight,
  Clock3,
  Coffee,
  Flame,
  Play,
  Settings2,
  ShieldCheck,
  Sparkles,
  X,
} from "lucide-react";
import { clampProgress, formatFocusMinutes } from "../shared/models/dashboard";

export type QuickPanelMode = "STANDBY" | "STUDY" | "BREAK" | "OFF";

export interface QuickPanelProps {
  mode?: QuickPanelMode;
  task?: string;
  elapsed?: string;
  focusMinutes?: number;
  targetMinutes?: number;
  streakDays?: number;
  balanceAP?: number;
  connected?: boolean;
  motivationAvailable?: boolean;
  notice?: string;
  onModeAction?: (mode: "STUDY" | "BREAK" | "OFF") => void;
  onOpenCenter?: () => void;
  onOpenSettings?: () => void;
  onClose?: () => void;
}

const modeCopy: Record<QuickPanelMode, { kicker: string; title: string; description: string }> = {
  STANDBY: { kicker: "准备开始", title: "准备开始学习", description: "给今天留下一点可见的进展" },
  STUDY: { kicker: "当前状态", title: "学习中", description: "把注意力留给眼前这一小步" },
  BREAK: { kicker: "当前状态", title: "休息中", description: "短暂离开屏幕，再回来继续" },
  OFF: { kicker: "今天已结束", title: "完成今天", description: "回看进展，给明天留一条线索" },
};

function modeStatus(mode: QuickPanelMode): string {
  return mode === "STUDY" ? "专注正常" : mode === "BREAK" ? "休息计时" : mode === "OFF" ? "已收好" : "等待开始";
}

export function QuickPanel({
  mode = "STUDY",
  task = "Go Context 与 goroutine",
  elapsed = "00:42",
  focusMinutes = 86,
  targetMinutes = 120,
  streakDays = 5,
  balanceAP = 12.43,
  connected = true,
  motivationAvailable = true,
  notice,
  onModeAction,
  onOpenCenter,
  onOpenSettings,
  onClose,
}: QuickPanelProps): ReactElement {
  const [localNotice, setLocalNotice] = useState("");
  const copy = modeCopy[mode];
  const progress = clampProgress(targetMinutes > 0 ? focusMinutes / targetMinutes : 0);

  const action = (nextMode: "STUDY" | "BREAK" | "OFF", message: string): void => {
    onModeAction?.(nextMode);
    setLocalNotice(message);
  };

  const displayNotice = notice ?? localNotice;
  const displayTarget = motivationAvailable && targetMinutes > 0 ? formatFocusMinutes(targetMinutes) : "—";
  const displayFocus = motivationAvailable && targetMinutes > 0 ? formatFocusMinutes(focusMinutes) : "—";

  return (
    <section className="quick-panel" aria-label="StudyGuardian 快捷面板">
      <header className="quick-panel-header">
        <div className="brand-lockup">
          <span className="brand-mark" aria-hidden="true"><Sparkles size={16} strokeWidth={2.2} /></span>
          <div>
            <div className="brand-name">StudyGuardian</div>
            <div className="brand-caption">本地专注陪伴</div>
          </div>
        </div>
        <div className="quick-panel-header-actions">
          <div className={`health-chip ${connected ? "" : "is-warning"}`}><ShieldCheck size={14} />{connected ? "正常" : "待连接"}</div>
          <button className="quick-panel-close" type="button" aria-label="关闭快捷面板" title="关闭 (Esc)" onClick={onClose}><X size={16} /></button>
        </div>
      </header>

      <div className="quick-panel-content">
        <section className="focus-summary" aria-labelledby="quick-state-title">
          <div className="focus-summary-topline">
            <span className="eyebrow">{copy.kicker}</span>
            <span className="mode-status"><span className="status-dot" />{modeStatus(mode)}</span>
          </div>
          <div className="focus-title-row">
            <h1 id="quick-state-title">{copy.title}</h1>
            <div className="elapsed"><Clock3 size={16} /><span>{elapsed}</span></div>
          </div>
          <p className="focus-description">{copy.description}</p>
          <div className="task-line"><BookOpen size={15} /><span>{task}</span></div>
        </section>

        <div className="quick-actions">
          {mode === "BREAK" ? (
            <button className="action-button action-primary" type="button" onClick={() => action("STUDY", "已准备继续学习")}> <Play size={16} fill="currentColor" />继续学习</button>
          ) : mode === "OFF" || mode === "STANDBY" ? (
            <button className="action-button action-primary" type="button" onClick={() => action("STUDY", "已准备开始学习")}> <Play size={16} fill="currentColor" />开始学习</button>
          ) : (
            <button className="action-button action-primary" type="button" onClick={() => action("BREAK", "已提交休息请求")}> <Coffee size={16} />休息一下</button>
          )}
          {mode === "STUDY" ? (
            <button className="action-button action-secondary" type="button" onClick={() => action("OFF", "今天的学习已收好")}>结束学习</button>
          ) : mode === "BREAK" ? (
            <button className="action-button action-secondary" type="button" onClick={() => action("OFF", "今天的学习已收好")}>结束今天</button>
          ) : (
            <button className="action-button action-secondary" type="button" onClick={() => action("STUDY", "已准备开始学习")}>重新开始</button>
          )}
        </div>

        <section className="progress-section" aria-labelledby="focus-progress-title">
          <div className="section-heading-row">
            <div>
              <div className="eyebrow" id="focus-progress-title">今日有效专注</div>
          <div className="progress-value">{displayFocus} <span>/ {displayTarget}</span></div>
          </div>
            <span className="progress-percent">{motivationAvailable && targetMinutes > 0 ? `${Math.round(progress * 100)}%` : "—"}</span>
          </div>
          <div className="progress-track" role="progressbar" aria-valuemin={0} aria-valuemax={targetMinutes || 1} aria-valuenow={motivationAvailable ? focusMinutes : 0} aria-label="今日有效专注进度">
            <span style={{ width: `${progress * 100}%` }} />
          </div>
          <div className="progress-caption"><span>{motivationAvailable && targetMinutes > 0 ? `目标 ${targetMinutes} 分钟` : "等待今日目标"}</span><span>{connected ? "保持现在的节奏" : "正在连接本地服务"}</span></div>
        </section>

        <div className="quick-metrics">
          <div className="quick-metric"><span className="metric-icon metric-icon-warm"><Flame size={16} /></span><div><strong>{motivationAvailable ? `${streakDays} 天` : "—"}</strong><span>连续专注</span></div></div>
          <div className="quick-metric"><span className="metric-icon metric-icon-violet"><Sparkles size={16} /></span><div><strong>{motivationAvailable ? `${balanceAP.toFixed(2)} AP` : "—"}</strong><span>当前余额</span></div></div>
        </div>

        {displayNotice && <div className="quick-notice" role="status"><CheckCircle2 size={15} />{displayNotice}</div>}
      </div>

      <footer className="quick-panel-footer">
        <button className="footer-link" type="button" onClick={onOpenCenter}><span><BookOpen size={16} />学习中心</span><ArrowUpRight size={15} /></button>
        <button className="footer-link footer-settings" type="button" onClick={onOpenSettings}><span><Settings2 size={16} />设置</span><ChevronRight size={15} /></button>
      </footer>
    </section>
  );
}

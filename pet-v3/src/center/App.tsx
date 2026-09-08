import { useEffect, useRef, useState, type ComponentType, type ReactElement } from "react";
import {
  Activity,
  ArrowUpRight,
  BarChart3,
  BookOpen,
  Check,
  CheckCircle2,
  ChevronRight,
  CircleHelp,
  Clock3,
  Coffee,
  Flame,
  Gift,
  History,
  LayoutDashboard,
  ListChecks,
  MoreHorizontal,
  Plus,
  Play,
  Settings2,
  ShieldCheck,
  Sparkles,
  Target,
  Trash2,
  Trophy,
  WalletCards,
  X,
} from "lucide-react";
import { Area, AreaChart, ResponsiveContainer, Tooltip, XAxis, YAxis } from "recharts";
import { clampProgress, formatFocusMinutes, totalFocusMinutes, type FocusDay } from "../shared/models/dashboard";
import { BrandMark } from "../shared/BrandMark";
import { activityLabels, deriveSupervisionState, formatLastActivity, interactionLabels, modeLabels, privacyLabels, relationLabels } from "../shared/models/supervision-state";
import { useTaskSelectionState } from "../shared/use-task-selection-state";
import { getSupervisorControlAdapter, getSystemIntegrationAdapter } from "../runtime/adapters";
import { TaskWheel } from "../shared/task-wheel/TaskWheel";
import type { TaskWheelAction } from "../shared/task-wheel/TaskWheelDialog";
import type { TaskPickerActionResult } from "../shared/task-mutation";
import { HelpDrawer } from "../shared/HelpDrawer";
import { FocusClock } from "./FocusClock";
import type { ControlResult, NativeAchievement, NativeAIEndpointSettings, NativeAISettings, NativeAutomationIntent, NativeAutomationSettings, NativeMission, NativeMotivationStatus, NativeReward, NativeReviewSummary, NativeTaskPresetList, ReviewGenerationStatusSnapshot, SupervisorDashboardSnapshot } from "../transport/supervisor";

type NavItem = { id: string; label: string; icon: ComponentType<{ size?: number; strokeWidth?: number }> };
type DashboardRefresh = () => Promise<SupervisorDashboardSnapshot | void>;

const primaryNav: NavItem[] = [
  { id: "overview", label: "总览", icon: LayoutDashboard },
  { id: "missions", label: "任务", icon: ListChecks },
  { id: "achievements", label: "成就", icon: Trophy },
  { id: "rewards", label: "奖励", icon: Gift },
  { id: "review", label: "学习复盘", icon: BookOpen },
  { id: "history", label: "历史", icon: History },
];

const secondaryNav: NavItem[] = [
  { id: "settings", label: "设置", icon: Settings2 },
  { id: "system", label: "系统状态", icon: Activity },
];

const focusData: FocusDay[] = [
  { label: "周六", minutes: 42, target: 120, completed: false },
  { label: "周日", minutes: 78, target: 120, completed: false },
  { label: "周一", minutes: 105, target: 120, completed: false },
  { label: "周二", minutes: 64, target: 120, completed: false },
  { label: "周三", minutes: 132, target: 120, completed: true },
  { label: "周四", minutes: 96, target: 120, completed: false },
  { label: "今天", minutes: 86, target: 120, completed: false },
];

const missionRows = [
  { title: "完成 Go Context 练习", note: "今天 · 进行中", reward: "+0.40 AP", done: false },
  { title: "整理本周学习笔记", note: "今天 · 待开始", reward: "+0.25 AP", done: false },
  { title: "完成一次 30 分钟专注", note: "已完成", reward: "+0.30 AP", done: true },
];

const achievement = { title: "一周坚持", description: "连续打卡 7 天，保持稳定的节奏", progress: .71, detail: "5 / 7 天" };

type DashboardProps = { snapshot?: SupervisorDashboardSnapshot; live?: boolean; initialActive?: string; routeRevision?: number; onNavigate?: (id: string) => void; onTaskChanged?: () => void | Promise<void>; onTaskMutationStarted?: () => void; onRefresh?: DashboardRefresh; onOpenHelp?: () => void };

const modeTitle: Record<"STANDBY" | "STUDY" | "BREAK" | "OFF", string> = {
  STANDBY: "准备开始",
  STUDY: "学习中",
  BREAK: "休息中",
  OFF: "今天已结束",
};

const reviewStatusLabels: Record<NativeReviewSummary["status"], string> = {
  PENDING: "正在生成",
  READY: "最新",
  STALE: "有新记录，建议更新",
  FAILED: "生成失败",
};

function reviewStatusLabel(status: NativeReviewSummary["status"] | undefined): string {
  return status ? reviewStatusLabels[status] : "待生成";
}

function liveMissionRows(missions: NativeMission[] | undefined): typeof missionRows {
  return (missions ?? []).map(mission => ({
    title: mission.title,
    note: mission.status === "COMPLETED" ? "已完成" : mission.status === "CANCELLED" ? "已取消" : "今天 · 进行中",
    reward: `+${(mission.reward_milli_ap / 1000).toFixed(2)} AP`,
    done: mission.status === "COMPLETED",
  }));
}

function displayTitle(id: string): string {
  return [...primaryNav, ...secondaryNav].find(item => item.id === id)?.label ?? "总览";
}

function navGroup(items: NavItem[], active: string, setActive: (id: string) => void): ReactElement {
  return <nav className="center-nav-group" aria-label="主导航">
    {items.map(item => {
      const Icon = item.icon;
      return <button className={`center-nav-item ${active === item.id ? "is-active" : ""}`} type="button" key={item.id} onClick={() => setActive(item.id)} aria-current={active === item.id ? "page" : undefined}>
        <Icon size={17} strokeWidth={active === item.id ? 2.3 : 1.9} /><span>{item.label}</span>
      </button>;
    })}
  </nav>;
}

function Dashboard({ snapshot, live = false, onNavigate, onTaskChanged, onTaskMutationStarted, onRefresh, onOpenHelp }: DashboardProps): ReactElement {
  const status = snapshot?.status;
  const motivation = snapshot?.motivation;
  const liveData = live && snapshot?.connected === true;
  const currentMode = status?.user_mode ?? (liveData ? "STANDBY" : "STUDY");
  const snapshotTask = status?.task || (liveData ? "未设置任务" : "Go Context 与 goroutine");
  const taskSelection = useTaskSelectionState(snapshotTask);
  const currentTask = taskSelection.task;
  const currentMinutes = motivation?.today_credited_focus_minutes ?? 0;
  const targetMinutes = motivation?.daily_target_minutes ?? 0;
  const progress = motivation?.target_progress ?? (liveData ? 0 : clampProgress(86 / 120));
  const chartData = liveData
    ? (snapshot?.history ?? []).slice().reverse().map(day => ({ label: day.date.slice(5), minutes: day.focus_minutes, target: day.target_minutes, completed: day.target_completed }))
    : focusData;
  const liveAchievement = snapshot?.achievements?.filter(item => !item.unlocked).sort((a, b) => a.progress - b.progress)[0] ?? snapshot?.achievements?.[0];
  const achievementView = liveData
    ? liveAchievement
      ? { title: liveAchievement.name, description: liveAchievement.description, progress: liveAchievement.progress, detail: `${Math.round(liveAchievement.progress * 100)}%` }
      : { title: "暂无成就数据", description: "完成一次有效专注后，这里会出现下一步目标", progress: 0, detail: "等待记录" }
    : achievement;
  const weeklyFocus = liveData ? totalFocusMinutes(chartData) : 603;
  const supervision = deriveSupervisionState(live ? Boolean(snapshot?.connected) : true, status);
  const modeCaption = currentMode === "STUDY" ? "把注意力放回当下，剩下的交给节奏。" : currentMode === "BREAK" ? "短暂离开屏幕，再回来继续。" : currentMode === "OFF" ? "今天已经收好，明天再从容开始。" : "给今天留下一点可见的进展。";
  const progressLabel = motivation ? `${Math.round(progress * 100)}%` : liveData ? "—" : "72%";
  const targetLabel = motivation ? `${targetMinutes} min` : liveData ? "—" : "120 min";
  const [taskNotice, setTaskNotice] = useState("");
  const control = getSupervisorControlAdapter();
  const taskOperation = (operation: Promise<ControlResult>): Promise<ControlResult> => operation;
  const taskResult = async (result: TaskPickerActionResult, action: TaskWheelAction): Promise<void> => {
    setTaskNotice(result.ok ? (action === "save" ? "常用任务已保存并选中" : "当前任务已更新") : "当前任务暂时无法更新");
    taskSelection.settle(result.ok, result.task);
    if (result.ok && result.task === undefined) {
      await onTaskChanged?.();
    }
  };
  const saveTask = async (name: string): Promise<ControlResult> => {
    const created = await control.createTaskPreset(name, true);
    if (!created.ok) return created;
    return taskOperation(control.setTask(name));
  };
  const modeAction = async (next: "STUDY" | "BREAK" | "OFF"): Promise<void> => {
    const result = next === "STUDY" ? await control.setModeStudy(currentTask === "未设置任务" ? "" : currentTask) : next === "BREAK" ? await control.setModeBreak() : await control.setModeOff();
    setTaskNotice(result.ok ? "状态已更新" : "状态暂时无法更新");
  };
  return <div className="dashboard-page">
    <div className="page-heading">
      <div><p className="heading-kicker">{liveData ? "今天" : "2026 年 9 月 4 日 · 星期五"}</p><h1>{liveData ? "今天，保持一点进展就够了" : "今天，保持一点进展就够了"}</h1><p className="heading-subtitle">{modeCaption}</p></div>
      <button className="quiet-button" type="button" onClick={onOpenHelp}><CircleHelp size={17} />帮助</button>
    </div>

    <section className="focus-hero" aria-labelledby="current-focus-title">
      <div className="hero-main">
        <div className="hero-topline"><span className="hero-kicker"><span className="live-dot" />当前状态</span><span className={`hero-health is-${supervision.behaviorTone}`}><ShieldCheck size={15} />{supervision.behaviorLabel}</span></div>
        <h2 id="current-focus-title">{modeTitle[currentMode]}</h2>
        <div className="hero-task-control"><BookOpen size={17} /><TaskWheel currentTask={currentTask} presets={snapshot?.task_presets} disabled={!liveData}
          onOptimisticTaskChange={task => {
            if (task !== undefined) { onTaskMutationStarted?.(); taskSelection.selectOptimistically(task); }
          }}
          onResult={taskResult}
          onSelect={id => taskOperation(control.selectTaskPreset(id))}
          onTemporary={name => taskOperation(control.setTask(name))}
          onSavePinned={saveTask}
          onUpdatePreset={(id, name, pinned, sortOrder) => taskOperation(control.updateTaskPreset(id, name, pinned, sortOrder))}
          onDeletePreset={id => taskOperation(control.deleteTaskPreset(id))}
        /></div>
        {taskNotice && <span className="hero-notice" role="status">{taskNotice}</span>}
        <p className="hero-caption">{liveData ? (status?.user_mode === "STUDY" ? `已保持专注 ${formatFocusMinutes(Math.floor(status.study_seconds / 60))}，继续完成眼前这一小段。` : modeCaption) : "已保持专注 42 分钟，继续完成眼前这一小段。"}</p>
        <div className="hero-actions">
          {currentMode === "STUDY" && <><button className="primary-button" type="button" onClick={() => void modeAction("BREAK")}><CoffeeIcon />休息一下</button><button className="secondary-button" type="button" onClick={() => void modeAction("OFF")}>结束学习</button></>}
          {currentMode === "BREAK" && <><button className="primary-button" type="button" onClick={() => void modeAction("STUDY")}><Play size={17} />继续学习</button><button className="secondary-button" type="button" onClick={() => void modeAction("OFF")}>结束今天</button></>}
          {currentMode === "STANDBY" && <button className="primary-button" type="button" onClick={() => void modeAction("STUDY")}><Play size={17} />开始学习</button>}
          {currentMode === "OFF" && <><button className="primary-button" type="button" onClick={() => onNavigate?.("review")}><BookOpen size={17} />查看今日复盘</button><button className="secondary-button" type="button" onClick={() => void modeAction("STUDY")}>重新开始学习</button></>}
        </div>
      </div>
      <FocusClock connected={liveData} status={status} motivation={motivation} />
    </section>

    <section className="metric-strip" aria-label="今日概览">
      <div className="metric-cell"><span className="metric-label">连续</span><strong><Flame size={16} />{motivation ? `${motivation.streak_days} 天` : liveData ? "—" : "5 天"}</strong><span className="metric-help">保持中</span></div>
      <div className="metric-cell"><span className="metric-label">本周专注</span><strong><Clock3 size={16} />{liveData ? formatFocusMinutes(weeklyFocus) : "10h 03m"}</strong><span className="metric-help">{liveData ? "过去 7 天" : "比上周 +12%"}</span></div>
      <div className="metric-cell"><span className="metric-label">AP 余额</span><strong><Sparkles size={16} />{motivation ? (motivation.balance_ap_milli / 1000).toFixed(3) : liveData ? "—" : "12.430"}</strong><span className="metric-help">可兑换奖励</span></div>
      <div className="metric-cell"><span className="metric-label">今日打卡</span><strong><CheckCircle2 size={16} />{motivation ? (motivation.checkin_completed ? "已完成" : "未完成") : liveData ? "—" : "已完成"}</strong><span className="metric-help">继续积累</span></div>
    </section>

    <div className="dashboard-grid">
      <section className="surface-section chart-section" aria-labelledby="focus-trend-title">
        <div className="section-header"><div><h2 id="focus-trend-title">专注趋势</h2><p>过去 7 天 · 有效专注分钟</p></div><button className="text-button" type="button">查看历史<ArrowUpRight size={15} /></button></div>
        <div className="chart-legend"><span><i className="legend-dot legend-dot-accent" />有效专注</span><span><i className="legend-line" />目标 {motivation ? `${motivation.daily_target_minutes} min` : liveData ? "—" : "120 min"}</span></div>
        <div className="focus-chart">{chartData.length > 0 ? <ResponsiveContainer width="100%" height="100%"><AreaChart data={chartData} margin={{ top: 12, right: 8, left: -24, bottom: 0 }}>
          <defs><linearGradient id="focusFill" x1="0" y1="0" x2="0" y2="1"><stop offset="0%" stopColor="var(--sg-accent)" stopOpacity={.22} /><stop offset="100%" stopColor="var(--sg-accent)" stopOpacity={0} /></linearGradient></defs>
          <XAxis dataKey="label" axisLine={false} tickLine={false} tick={{ fill: "var(--sg-text-tertiary)", fontSize: 12 }} dy={8} />
          <YAxis axisLine={false} tickLine={false} tick={{ fill: "var(--sg-text-tertiary)", fontSize: 12 }} domain={[0, 180]} ticks={[0, 60, 120, 180]} />
          <Tooltip cursor={{ stroke: "var(--sg-border-strong)", strokeDasharray: "4 4" }} contentStyle={{ background: "var(--sg-surface-raised)", border: "1px solid var(--sg-border)", borderRadius: 10, color: "var(--sg-text)", fontSize: 12 }} />
          <Area type="monotone" dataKey="target" stroke="var(--sg-border-strong)" strokeDasharray="5 5" strokeWidth={1.5} fill="none" name="目标" />
          <Area type="monotone" dataKey="minutes" stroke="var(--sg-accent)" strokeWidth={2.5} fill="url(#focusFill)" name="专注" />
        </AreaChart></ResponsiveContainer> : <div className="chart-empty">等待 Supervisor 返回近 7 天记录</div>}</div>
      </section>

      <MissionSummary missions={snapshot?.missions} live={liveData} motivation={motivation} onNavigate={onNavigate} onRefresh={onRefresh} />

      <section className="surface-section achievement-section" aria-labelledby="achievement-title">
        <div className="section-header"><div><h2 id="achievement-title">下一步成就</h2><p>{liveData ? "来自 Supervisor 的当前进度" : "再坚持两天，就到了"}</p></div><Trophy className="section-icon" size={20} /></div>
        <div className="achievement-highlight"><span className="achievement-icon"><Target size={20} /></span><div><strong>{achievementView.title}</strong><span>{achievementView.description}</span></div></div>
        <div className="achievement-progress"><div className="thin-progress"><span style={{ width: `${achievementView.progress * 100}%` }} /></div><span>{achievementView.detail}</span></div>
      </section>

      <section className="surface-section review-section" aria-labelledby="review-title">
        <div className="section-header"><div><h2 id="review-title">今日复盘</h2><p>{snapshot?.review ? (snapshot.review.generation_mode === "AI" ? "AI 总结" : "本地总结 · 未使用云端 AI") : "结束学习后自动整理"}</p></div><span className="review-badge">{reviewStatusLabel(snapshot?.review?.status)}</span></div>
        <div className="review-body"><span className="review-icon"><BarChart3 size={20} /></span><div><strong>{snapshot?.review?.headline ?? "今天的故事还在继续"}</strong><span>{snapshot?.review ? snapshot.review.tomorrow_priority : "完成一次学习后，就能看到今天的进展摘要。"}</span></div></div>
        <button className="section-link" type="button" onClick={() => onNavigate?.("review")}>{snapshot?.review ? "查看今日总结" : "打开学习复盘"}<ChevronRight size={16} /></button>
      </section>
    </div>
  </div>;
}

function CoffeeIcon(): ReactElement { return <Coffee size={17} />; }

function DataPage({ title, description, actions, children }: { title: string; description: string; actions?: ReactElement; children: ReactElement }): ReactElement {
  return <div className="data-page"><div className="data-page-heading"><div><p className="heading-kicker">StudyGuardian · 本地数据</p><h1>{title}</h1><p>{description}</p></div>{actions}</div>{children}</div>;
}

function EmptyData({ text }: { text: string }): ReactElement {
  return <div className="data-empty"><Sparkles size={20} /><span>{text}</span></div>;
}

function MissionSummary({ missions, live, motivation, onNavigate, onRefresh }: { missions?: NativeMission[]; live: boolean; motivation?: NativeMotivationStatus; onNavigate?: (id: string) => void; onRefresh?: DashboardRefresh }): ReactElement {
  const [busyId, setBusyId] = useState<string>();
  const [notice, setNotice] = useState("");
  const control = getSupervisorControlAdapter();
  const open = (missions ?? []).filter(item => item.status === "OPEN").slice(0, 3);
  const fallback: NativeMission[] = live ? [] : missionRows.map((row, index) => ({ id: `fallback-${index}`, title: row.title, description: "", reward_milli_ap: 0, status: row.done ? "COMPLETED" as const : "OPEN" as const, created_at: "" }));
  const items = live ? open : fallback;
  const complete = async (id: string): Promise<void> => {
    setBusyId(id); const result = await control.completeMission(id); setBusyId(undefined);
    if (!result.ok) { setNotice("任务暂时无法完成"); return; }
    await onRefresh?.();
  };
  const completed = (missions ?? []).filter(item => item.status === "COMPLETED").length;
  const total = (missions ?? []).filter(item => item.status !== "CANCELLED").length;
  const focusProgress = motivation ? Math.round(motivation.target_progress * 100) : 0;
  return <section className="surface-section mission-section" aria-labelledby="missions-title">
    <div className="section-header"><div><h2 id="missions-title">今日任务</h2><p>{live ? `${completed} / ${total} 已完成` : "完成小步，也算进展"}</p></div><button className="text-button" type="button" onClick={() => onNavigate?.("missions")}><Plus size={14} />添加任务</button></div>
    <div className="mission-list">{items.length > 0 ? items.map(item => <div className={`mission-row ${item.status === "COMPLETED" ? "is-done" : ""}`} key={item.id}>
      {item.status === "OPEN" ? <button className="mission-check mission-check-button" type="button" disabled={busyId === item.id} aria-label={`完成任务 ${item.title}`} onClick={() => void complete(item.id)}>{busyId === item.id ? "…" : ""}</button> : <span className="mission-check">{item.status === "COMPLETED" && <Check size={14} />}</span>}
      <div className="mission-copy"><strong>{item.title}</strong><span>{item.status === "COMPLETED" ? "已完成" : item.linked_task_name ? `关联：${item.linked_task_name}` : "今天 · 待完成"}</span></div><span className="mission-reward">{live ? "+0 AP" : item.status === "COMPLETED" ? "已完成" : "待完成"}</span>
    </div>) : <div className="mission-empty"><span>暂无任务记录</span><button className="text-button" type="button" onClick={() => onNavigate?.("missions")}>添加今日任务</button></div>}</div>
    <div className="mission-goal"><div><span>今日专注目标</span><strong>{motivation ? `${focusProgress}%` : "—"}</strong></div><div className="thin-progress"><span style={{ width: `${focusProgress}%` }} /></div></div>
    {notice && <span className="settings-notice" role="status">{notice}</span>}
    <button className="section-link" type="button" onClick={() => onNavigate?.("missions")}>查看全部任务<ChevronRight size={16} /></button>
  </section>;
}

function MissionsPage({ missions, taskPresets, onRefresh }: { missions?: NativeMission[]; taskPresets?: NativeTaskPresetList; onRefresh?: DashboardRefresh }): ReactElement {
  const [adding, setAdding] = useState(false);
  const [title, setTitle] = useState("");
  const [description, setDescription] = useState("");
  const [linkedTask, setLinkedTask] = useState("");
  const [notice, setNotice] = useState("");
  const [busyId, setBusyId] = useState<string>();
  const control = getSupervisorControlAdapter();
  const allTasks = [...(taskPresets?.pinned ?? []), ...(taskPresets?.recent ?? [])];
  const active = (missions ?? []).filter(item => item.status === "OPEN");
  const completed = (missions ?? []).filter(item => item.status === "COMPLETED");
  const suggested = allTasks.find(item => title.trim().toLocaleLowerCase().includes(item.name.trim().toLocaleLowerCase()));
  const create = async (): Promise<void> => {
    if (!title.trim()) { setNotice("请先填写任务标题"); return; }
    setNotice("正在创建…");
    const selected = allTasks.find(item => item.name === linkedTask);
    const result = await control.createMission(title, description, undefined, selected?.name, selected?.id);
    if (!result.ok) { setNotice("任务创建失败，请稍后重试"); return; }
    setTitle(""); setDescription(""); setLinkedTask(""); setAdding(false); setNotice("今日任务已添加"); await onRefresh?.();
  };
  const complete = async (id: string): Promise<void> => { setBusyId(id); const result = await control.completeMission(id); setBusyId(undefined); setNotice(result.ok ? "任务已完成" : "任务完成失败"); if (result.ok) await onRefresh?.(); };
  const cancel = async (id: string): Promise<void> => { setBusyId(id); const result = await control.cancelMission(id); setBusyId(undefined); setNotice(result.ok ? "任务已取消" : "任务取消失败"); if (result.ok) await onRefresh?.(); };
  const startTask = async (mission: NativeMission): Promise<void> => {
    if (!mission.linked_task_name) { setNotice("请先为任务关联当前专注任务"); return; }
    setNotice("正在切换当前任务…");
    const result = mission.linked_task_preset_id ? await control.selectTaskPreset(mission.linked_task_preset_id) : await control.setTask(mission.linked_task_name);
    if (!result.ok) { setNotice("当前任务切换失败"); return; }
    const modeResult = await control.setModeStudy(mission.linked_task_name);
    setNotice(modeResult.ok ? `已开始：${mission.linked_task_name}` : "任务已切换，但学习模式未能启动");
    await onRefresh?.();
  };
  const renderMission = (mission: NativeMission): ReactElement => <div className={`mission-card ${mission.status === "COMPLETED" ? "is-done" : ""}`} key={mission.id}>
    <div className="mission-card-main"><button className="mission-check mission-check-button" type="button" disabled={mission.status !== "OPEN" || busyId === mission.id} aria-label={`${mission.status === "OPEN" ? "完成" : "已完成"} ${mission.title}`} onClick={() => mission.status === "OPEN" && void complete(mission.id)}>{mission.status === "COMPLETED" && <Check size={14} />}</button><div><strong>{mission.title}</strong>{mission.description && <p>{mission.description}</p>}<span>{mission.linked_task_name ? `关联：${mission.linked_task_name}` : "未关联当前专注任务"}{mission.due_date ? ` · 截止 ${mission.due_date}` : ""}</span></div></div>
    <div className="mission-card-actions"><em>+0 AP</em>{mission.status === "OPEN" && <><button type="button" onClick={() => void startTask(mission)} disabled={!mission.linked_task_name}>开始此任务</button><button type="button" onClick={() => void cancel(mission.id)} disabled={busyId === mission.id}>取消</button></>}</div>
  </div>;
  return <DataPage title="任务" description="把今天要完成的成果拆小一点。" actions={<button className="primary-button page-action-button" type="button" onClick={() => setAdding(true)}><Plus size={16} />添加任务</button>}>
    <div className="mission-page-content">
      {adding && <section className="surface-section mission-create-card"><div className="section-header"><div><h2>添加今日任务</h2><p>自定义任务奖励固定为 0 AP。</p></div><button className="icon-button" type="button" aria-label="关闭添加任务" onClick={() => setAdding(false)}><X size={17} /></button></div><div className="mission-form"><label><span>任务标题</span><input autoFocus maxLength={256} value={title} placeholder="例如：完成 Go Context 练习" onChange={event => setTitle(event.target.value)} /></label><label><span>描述（可选）</span><textarea maxLength={1024} value={description} placeholder="写下可验收的下一步" onChange={event => setDescription(event.target.value)} /></label><label><span>关联专注任务</span><select value={linkedTask} onChange={event => setLinkedTask(event.target.value)}><option value="">不关联</option>{allTasks.map(item => <option key={item.id} value={item.name}>{item.name}</option>)}</select></label>{suggested && !linkedTask && <button className="mission-suggestion" type="button" onClick={() => setLinkedTask(suggested.name)}>规则建议关联到“{suggested.name}”</button>}<div className="setting-actions"><button className="primary-button" type="button" onClick={() => void create()}>创建任务</button><button className="secondary-button" type="button" onClick={() => setAdding(false)}>取消</button></div></div></section>}
      <section className="surface-section mission-page-section"><div className="section-header"><div><h2>今日任务 <span className="mission-count">{completed.length} / {active.length + completed.length} 已完成</span></h2><p>已取消的任务默认隐藏，不会恢复 AP。</p></div><ListChecks className="section-icon" size={20} /></div>{active.length > 0 ? <div className="mission-card-list">{active.map(renderMission)}</div> : <EmptyData text="暂无进行中的任务" />}</section>
      {completed.length > 0 && <section className="surface-section mission-page-section completed-missions"><div className="section-header"><div><h2>已完成</h2><p>完成记录只读保留</p></div><CheckCircle2 className="section-icon" size={20} /></div><div className="mission-card-list">{completed.map(renderMission)}</div></section>}
      {notice && <p className="settings-notice mission-page-notice" role="status">{notice}</p>}
    </div>
  </DataPage>;
}

function AchievementsPage({ achievements }: { achievements?: NativeAchievement[] }): ReactElement {
  return <DataPage title="成就" description="把稳定的节奏，留在自己的进度里。"><section className="surface-section data-card"><div className="section-header"><div><h2>成就进度</h2><p>已完成和下一步目标</p></div><Trophy className="section-icon" size={20} /></div>{achievements && achievements.length > 0 ? <div className="data-list">{achievements.map(item => <div className={`data-row ${item.unlocked ? "is-done" : ""}`} key={item.achievement_id}><div><strong>{item.name}</strong><span>{item.description}</span><div className="data-progress"><span style={{ width: `${item.progress * 100}%` }} /></div></div><em>{item.unlocked ? "已解锁" : `${Math.round(item.progress * 100)}%`}</em></div>)}</div> : <EmptyData text="暂无成就记录" />}</section></DataPage>;
}

function RewardsPage({ rewards }: { rewards?: NativeReward[] }): ReactElement {
  return <DataPage title="奖励" description="有效专注带来的 AP，可以换成现实里的小奖励。"><section className="surface-section data-card"><div className="section-header"><div><h2>奖励目录</h2><p>当前可用的本地奖励</p></div><Gift className="section-icon" size={20} /></div>{rewards && rewards.length > 0 ? <div className="data-list">{rewards.map(item => <div className="data-row" key={item.id}><div><strong>{item.name}</strong><span>{item.description}</span></div><em>{(item.cost_milli_ap / 1000).toFixed(2)} AP</em></div>)}</div> : <EmptyData text="暂无奖励目录" />}</section></DataPage>;
}

function HistoryPage({ history }: { history?: SupervisorDashboardSnapshot["history"] }): ReactElement {
  return <DataPage title="历史" description="回看最近 7 天的有效专注，不追踪原始屏幕内容。"><section className="surface-section data-card"><div className="section-header"><div><h2>专注记录</h2><p>仅显示 Supervisor 提供的分钟级汇总</p></div><History className="section-icon" size={20} /></div>{history && history.length > 0 ? <div className="data-list">{history.map(day => <div className="data-row" key={day.date}><div><strong>{day.date}</strong><span>目标 {day.target_minutes} 分钟 · {day.target_completed ? "已达标" : "进行中"}</span></div><em>{day.focus_minutes} min</em></div>)}</div> : <EmptyData text="暂无历史记录" />}</section></DataPage>;
}

function reviewTopicLabel(name: string): string {
  const key = name.trim().toUpperCase();
  return Object.prototype.hasOwnProperty.call(activityLabels, key) ? activityLabels[key as keyof typeof activityLabels] : name;
}

export function ReviewPage({ review, onRefresh, control = getSupervisorControlAdapter(), pollIntervalMs = 2000, maxWaitMs = 120000 }: { review?: NativeReviewSummary; onRefresh?: DashboardRefresh; control?: ReturnType<typeof getSupervisorControlAdapter>; pollIntervalMs?: number; maxWaitMs?: number }): ReactElement {
  const [notice, setNotice] = useState<{ text: string; tone: "neutral" | "success" | "warning" }>();
  const [generating, setGenerating] = useState(false);
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const runRef = useRef(0);
  useEffect(() => () => {
    runRef.current += 1;
    if (timerRef.current !== null) clearTimeout(timerRef.current);
  }, []);
  const finish = async (status: ReviewGenerationStatusSnapshot, token: number): Promise<void> => {
    if (token !== runRef.current) return;
    const refreshedSnapshot = await onRefresh?.();
    if (token !== runRef.current) return;
    const refreshedReview = refreshedSnapshot && typeof refreshedSnapshot === "object" && "review" in refreshedSnapshot
      ? refreshedSnapshot.review
      : undefined;
    if (status.state === "READY") {
      const message = status.generation_mode === "FALLBACK" && status.error_kind === "timeout"
        ? "AI 响应超时，已生成本地总结"
        : status.generation_mode === "FALLBACK" ? "今日总结已生成本地总结" : "AI 总结已生成";
      setNotice({ text: message, tone: "success" });
    } else if (refreshedReview?.generation_mode === "FALLBACK" && refreshedReview.status !== "PENDING") {
      const message = status.error_kind === "timeout" || refreshedReview.error_code === "timeout"
        ? "AI 响应超时，已生成本地总结"
        : "今日总结已生成本地总结";
      setNotice({ text: message, tone: "success" });
    } else {
      setNotice({ text: "今日总结暂时无法生成", tone: "warning" });
    }
    setGenerating(false);
  };

  const poll = async (generationId: string | undefined, startedAt: number, token: number, finalAttempt = false): Promise<void> => {
    if (token !== runRef.current) return;
    let status: ReviewGenerationStatusSnapshot;
    try {
      status = await control.getReviewGenerationStatus();
    } catch {
      status = { state: "FAILED", error_kind: "unavailable" };
    }
    if (token !== runRef.current) return;
    if (generationId && status.generation_id && status.generation_id !== generationId) {
      if (finalAttempt) {
        await onRefresh?.();
        if (token === runRef.current) {
          setNotice({ text: "正在生成，你可以继续使用 StudyGuardian", tone: "neutral" });
          setGenerating(false);
        }
        return;
      }
      timerRef.current = setTimeout(() => void poll(generationId, startedAt, token), Math.max(100, pollIntervalMs));
      return;
    }
    if (status.state === "READY" || status.state === "FAILED") {
      await finish(status, token);
      return;
    }
    if (Date.now() - startedAt >= maxWaitMs) {
      if (!finalAttempt) {
        await poll(generationId, startedAt, token, true);
        return;
      }
      await onRefresh?.();
      if (token === runRef.current) {
        setNotice({ text: "正在生成，你可以继续使用 StudyGuardian", tone: "neutral" });
        setGenerating(false);
      }
      return;
    }
    timerRef.current = setTimeout(() => void poll(generationId, startedAt, token), Math.max(100, pollIntervalMs));
  };

  const generate = async (): Promise<void> => {
    if (generating) return;
    const token = ++runRef.current;
    setGenerating(true);
    setNotice({ text: "正在生成今日总结…", tone: "neutral" });
    try {
      const result = await control.startReviewGeneration();
      if (token !== runRef.current) return;
      if (result.state === "FAILED" || result.state === "IDLE") {
        await finish(result, token);
        return;
      }
      await poll(result.generation_id, Date.now(), token);
    } catch {
      setNotice({ text: "今日总结暂时无法生成", tone: "warning" });
      setGenerating(false);
    }
  };
  const reason = review?.status === "STALE" ? "生成后又有新的学习记录。" : review?.generation_mode === "FALLBACK" ? (review.error_code && review.error_code !== "provider_not_configured" ? "AI 暂时不可用，本次已自动使用本地总结。" : "尚未配置 AI，本次使用本地证据生成。") : "根据今天记录的学习活动整理。";
  const noticeView = notice && <span className={`settings-notice is-${notice.tone}`} role="status" aria-live="polite">{notice.text}</span>;
  return <DataPage title="学习复盘" description="摘要来自本地 Review，不展示原始聊天或屏幕内容。"><section className="surface-section data-card" aria-busy={generating}>{review ? <><div className="section-header"><div><h2>{review.headline}</h2><p>{review.date} · {reason}</p></div><span className="review-badge">{reviewStatusLabel(review.status)}</span></div>{review.status === "STALE" && <p className="review-stale-notice">生成后又有新的学习记录，当前内容仍可查看。</p>}<div className="review-detail-grid"><div><span className="eyebrow">学习进展</span>{review.topics.length > 0 ? review.topics.map(topic => <p key={topic.name}><strong>{reviewTopicLabel(topic.name)}</strong> · {topic.summary}</p>) : <p>今天记录了学习活动，但还没有足够证据确认具体完成项。</p>}</div><div><span className="eyebrow">尚未记录完成项</span>{review.unfinished.length > 0 ? review.unfinished.map(item => <p key={item}>{item}</p>) : <p>暂无待办</p>}</div><div><span className="eyebrow">明日优先级</span><p>{review.tomorrow_priority || "暂无记录"}</p></div></div>{review.status === "STALE" && <button className="primary-button" type="button" disabled={generating} onClick={() => void generate()}>{generating ? "正在更新…" : "更新今日总结"}</button>}</> : <div className="review-generate-empty"><EmptyData text="今日总结将在结束学习后约 5 分钟自动生成" /><button className="primary-button" type="button" disabled={generating} onClick={() => void generate()}>{generating ? "正在生成…" : "立即生成"}</button></div>}{noticeView}</section></DataPage>;
}
function SystemPage({ snapshot }: { snapshot?: SupervisorDashboardSnapshot }): ReactElement {
  const status = snapshot?.status;
  const semantic = snapshot?.semantic;
  const ai = snapshot?.ai;
  const supervision = deriveSupervisionState(Boolean(snapshot?.connected), status);
  return <DataPage title="系统状态" description="查看本地 Supervisor 与受限功能的健康状态。"><section className="surface-section data-card"><div className="section-header"><div><h2>本地服务</h2><p>不会显示 token、路径或原始错误</p></div><Activity className="section-icon" size={20} /></div><div className="system-status-grid">
    <div><span>Supervisor</span><strong>{snapshot?.connected ? "已连接" : "离线"}</strong></div>
    <div><span>当前模式</span><strong>{status ? modeLabels[status.user_mode] : "暂无"}</strong></div>
    <div><span>交互状态</span><strong>{status ? interactionLabels[status.interaction_state] : "暂无"}</strong></div>
    <div><span>任务关系</span><strong>{status ? relationLabels[status.task_relation] : "暂无"}</strong></div>
    <div><span>模式来源</span><strong>{status?.mode_origin === "AUTOMATION" ? "自动计时" : status ? "手动操作" : "暂无"}</strong></div>
    <div><span>暂停原因</span><strong>{status?.pause_reason === "IDLE" ? "持续静止" : status?.pause_reason === "LOCKED" ? "锁屏" : status?.pause_reason === "SENSOR_UNAVAILABLE" ? "传感器不可用" : status?.pause_reason === "SLEEP" ? "系统休眠" : "无"}</strong></div>
    <div><span>隐私状态</span><strong>{status ? privacyLabels[status.privacy_state] : "暂无"}</strong></div>
    <div><span>最近活动</span><strong>{formatLastActivity(status?.last_activity_at)}</strong></div>
    <div><span>ActivityWatch</span><strong>{status ? (status.activitywatch_ok ? "正常" : "异常") : "待检查"}</strong></div>
    <div><span>Screen Sensor</span><strong>{status ? (status.screen_sensor_ok ? "正常" : "异常") : "待检查"}</strong></div>
    <div><span>AI</span><strong>{ai?.enabled && ai.text_configured ? "已配置" : "规则模式"}</strong></div>
    <div><span>语义来源</span><strong>{semantic?.source_kind === "VISION_AI" ? "视觉 AI" : semantic?.source_kind === "TEXT_AI" ? "文字 AI" : semantic?.source_kind === "LOCAL_RULE" ? "本地规则" : "暂无"}</strong></div>
    <div><span>语义置信度</span><strong>{semantic?.fresh ? `${Math.round(semantic.confidence * 100)}%` : "暂无"}</strong></div>
    <div><span>活动类型</span><strong>{semantic?.fresh ? reviewTopicLabel(semantic.activity) : "暂无"}</strong></div>
    <div><span>主题</span><strong>{semantic?.fresh ? (semantic.topic || "未识别") : "暂无"}</strong></div>
    <div><span>子主题</span><strong>{semantic?.fresh ? (semantic.subtopic || "未识别") : "暂无"}</strong></div>
    <div><span>学习动作</span><strong>{semantic?.fresh ? (semantic.action || "未识别") : "暂无"}</strong></div>
    <div><span>进展信号</span><strong>{semantic?.fresh ? reviewTopicLabel(semantic.progress_signal ?? "UNKNOWN") : "暂无"}</strong></div>
  </div><p className={`system-summary is-${supervision.systemTone}`}>{supervision.systemLabel}</p></section></DataPage>;
}

const providerHints: Record<string, { base_url: string; model: string; fallback_models: string[]; timeout_seconds?: number }> = {
  none: { base_url: "", model: "", fallback_models: [] },
  aihubmix: { base_url: "https://aihubmix.com/v1", model: "coding-glm-5.3-free", fallback_models: [], timeout_seconds: 20 },
  deepseek: { base_url: "https://api.deepseek.com", model: "deepseek-chat", fallback_models: [] },
  qwen: { base_url: "https://dashscope.aliyuncs.com/compatible-mode/v1", model: "qwen-plus", fallback_models: [] },
  kimi: { base_url: "https://api.moonshot.cn/v1", model: "moonshot-v1-8k", fallback_models: [] },
  zhipu: { base_url: "https://open.bigmodel.cn/api/paas/v4", model: "glm-4-flash", fallback_models: [] },
  siliconflow: { base_url: "https://api.siliconflow.cn/v1", model: "Qwen/Qwen2.5-7B-Instruct", fallback_models: [] },
  doubao: { base_url: "https://ark.cn-beijing.volces.com/api/v3", model: "", fallback_models: [] },
  openai: { base_url: "https://api.openai.com/v1", model: "gpt-4o-mini", fallback_models: [] },
  "openai-compatible": { base_url: "", model: "", fallback_models: [] }, ollama: { base_url: "http://127.0.0.1:11434/v1", model: "qwen2.5", fallback_models: [] },
};

const providerLabels: Record<string, string> = { aihubmix: "AIHubMix 中转", "openai-compatible": "其他 OpenAI 兼容服务" };

type FallbackTarget = "text" | "vision";
type FallbackDrafts = Partial<Record<FallbackTarget, string>>;
type FallbackErrors = Partial<Record<FallbackTarget, string>>;

function parseFallbackModels(raw: string): { models: string[]; error?: string } {
  const models: string[] = [];
  const seen = new Set<string>();
  for (const value of raw.split(/[,，;；\r\n]+/).map(item => item.trim()).filter(Boolean)) {
    if (/\p{C}/u.test(value) || /\s/.test(value)) {
      return { models: [], error: "模型 ID 不能包含控制字符或内部空白" };
    }
    if (value.length > 128) return { models: [], error: "模型 ID 最多 128 个字符" };
    if (!seen.has(value)) {
      seen.add(value);
      models.push(value);
    }
  }
  if (models.length > 3) return { models: [], error: "最多只能设置 3 个备用模型" };
  return { models };
}

function AIEndpointEditor({ title, target, endpoint, keyValue, onKeyValue, onChange, onFallbackChange, fallbackDraft, fallbackError, onFallbackBlur, onPutSecret, onDeleteSecret, onTest, busy }: {
  title: string; target: "text" | "vision"; endpoint: NativeAIEndpointSettings; keyValue: string; onKeyValue: (value: string) => void;
  onChange: (value: NativeAIEndpointSettings) => void; onFallbackChange: (value: string) => void; fallbackDraft: string; fallbackError?: string; onFallbackBlur: () => void;
  onPutSecret: () => void; onDeleteSecret: () => void; onTest: () => void; busy?: boolean;
}): ReactElement {
  const changeProvider = (provider: string): void => {
    const hint = providerHints[provider] ?? { base_url: "", model: "", fallback_models: [] };
    const visionHint = provider === "aihubmix" ? { model: "ox-alpha", fallback_models: [] } : hint;
    const fallbackModels = target === "vision" ? visionHint.fallback_models : hint.fallback_models;
    onFallbackChange(fallbackModels.join(", "));
    onChange({ ...endpoint, provider, enabled: target === "text" ? provider !== "none" : endpoint.enabled, base_url: hint.base_url, model: target === "vision" ? visionHint.model : hint.model, fallback_models: fallbackModels, timeout_seconds: hint.timeout_seconds ?? endpoint.timeout_seconds });
  };
  return <div className="ai-endpoint-card ai-subsection-card"><div className="ai-endpoint-heading"><div><strong>{title}</strong><span>{target === "vision" ? "仅在文字判断仍不确定且明确启用时使用" : "本地规则无法判断时才调用"}</span></div>{target === "vision" && <label className="switch-label"><input type="checkbox" checked={endpoint.enabled} onChange={event => onChange({ ...endpoint, enabled: event.target.checked })} />启用</label>}</div>
    <div className="ai-field-grid"><label><span>服务商</span><select value={endpoint.provider} disabled={busy} onChange={event => changeProvider(event.target.value)}>{Object.keys(providerHints).map(value => <option value={value} key={value}>{providerLabels[value] ?? value}</option>)}</select></label><label><span>主模型</span><input value={endpoint.model} disabled={busy} onChange={event => onChange({ ...endpoint, model: event.target.value })} /></label><label className="wide"><span>API 地址</span><input value={endpoint.base_url} disabled={busy} onChange={event => onChange({ ...endpoint, base_url: event.target.value })} /></label><label className="wide"><span>备用模型（按顺序）</span><textarea aria-label="备用模型（按顺序）" aria-describedby={`${target}-fallback-help${fallbackError ? ` ${target}-fallback-error` : ""}`} rows={2} value={fallbackDraft} disabled={busy} onChange={event => onFallbackChange(event.target.value)} onBlur={onFallbackBlur} /><small id={`${target}-fallback-help`}>最多 3 个，支持中英文逗号、分号或换行分隔。</small>{fallbackError && <span className="field-error" id={`${target}-fallback-error`} role="alert">{fallbackError}</span>}</label><label><span>JSON 模式</span><select value={endpoint.json_mode} disabled={busy} onChange={event => onChange({ ...endpoint, json_mode: event.target.value as NativeAIEndpointSettings["json_mode"] })}><option value="auto">自动</option><option value="json_object">JSON Object</option><option value="off">关闭</option></select></label><label><span>单模型超时（秒）</span><input type="number" min={1} max={120} value={endpoint.timeout_seconds} disabled={busy} onChange={event => onChange({ ...endpoint, timeout_seconds: Number(event.target.value) })} /></label></div>
    <div className="secret-row"><span className={endpoint.api_key_configured ? "secret-state is-set" : "secret-state"}>{endpoint.api_key_configured ? "API Key 已配置" : "API Key 未配置"}</span><input type="password" autoComplete="new-password" value={keyValue} placeholder="输入新 Key（不会回显）" disabled={busy} onChange={event => onKeyValue(event.target.value)} /><button type="button" disabled={busy || !keyValue.trim()} onClick={onPutSecret}>保存 Key</button>{endpoint.api_key_configured && <button type="button" disabled={busy} onClick={onDeleteSecret}>删除 Key</button>}<button className="test-button" type="button" disabled={busy} onClick={onTest}>测试连接</button></div>
  </div>;
}

function sameAISettings(left: NativeAISettings, right: NativeAISettings): boolean {
  return JSON.stringify(left) === JSON.stringify(right);
}

export function AISettingsPanel({ settings: source, onRefresh, control: controlOverride }: {
  settings?: NativeAISettings;
  onRefresh?: DashboardRefresh;
  control?: ReturnType<typeof getSupervisorControlAdapter>;
}): ReactElement {
  const fallback: NativeAISettings = { enabled: false, min_confidence: .75, proxy: { mode: "environment", url: "" }, text: { enabled: false, provider: "none", model: "", fallback_models: [], base_url: "", api_key_configured: false, timeout_seconds: 6, json_mode: "auto" }, vision: { enabled: false, provider: "none", model: "", fallback_models: [], base_url: "", api_key_configured: false, timeout_seconds: 8, json_mode: "auto" } };
  const [draft, setDraft] = useState<NativeAISettings>();
  const [textKey, setTextKey] = useState(""); const [visionKey, setVisionKey] = useState(""); const [notice, setNotice] = useState("");
  const [fallbackDrafts, setFallbackDrafts] = useState<FallbackDrafts>({});
  const [fallbackErrors, setFallbackErrors] = useState<FallbackErrors>({});
  const [proxyNotice, setProxyNotice] = useState<{ label: string; tone: "neutral" | "success" | "warning" }>();
  const [busyTarget, setBusyTarget] = useState<"text" | "vision" | "proxy" | "settings">();
  const settings = draft ?? (source ? { ...source, proxy: source.proxy ?? fallback.proxy } : fallback); const control = controlOverride ?? getSupervisorControlAdapter();
  useEffect(() => {
    if (draft && source && sameAISettings(draft, source)) setDraft(undefined);
  }, [draft, source]);
  const endpoint = (target: "text" | "vision", value: NativeAIEndpointSettings): void => setDraft({ ...settings, [target]: value });
  const refreshCanonical = async (): Promise<void> => {
    if (!onRefresh) return;
    await onRefresh();
    setDraft(undefined);
    setFallbackDrafts({});
    setFallbackErrors({});
  };
  const fallbackValue = (target: FallbackTarget): string => fallbackDrafts[target] ?? settings[target].fallback_models.join(", ");
  const settingsForPersistence = (): NativeAISettings | undefined => {
    const next: NativeAISettings = { ...settings, text: { ...settings.text }, vision: { ...settings.vision } };
    const errors: FallbackErrors = {};
    for (const target of ["text", "vision"] as const) {
      const parsed = parseFallbackModels(fallbackValue(target));
      if (parsed.error) errors[target] = parsed.error;
      else next[target].fallback_models = parsed.models;
    }
    setFallbackErrors(errors);
    return Object.keys(errors).length === 0 ? next : undefined;
  };
  const commitFallbackDraft = (target: FallbackTarget): void => {
    const parsed = parseFallbackModels(fallbackValue(target));
    setFallbackErrors(current => ({ ...current, [target]: parsed.error }));
    if (parsed.error) return;
    setDraft({ ...settings, [target]: { ...settings[target], fallback_models: parsed.models } });
  };
  const setFallbackDraft = (target: FallbackTarget, value: string): void => {
    setFallbackDrafts(current => ({ ...current, [target]: value }));
    setFallbackErrors(current => ({ ...current, [target]: undefined }));
  };
  const persistCurrentSettings = async (): Promise<NativeAISettings | undefined> => {
    const next = settingsForPersistence();
    if (!next) return undefined;
    const result = await control.saveAISettings(next);
    if (!result.ok) {
      setNotice("AI 设置未通过验证或暂时无法保存");
      return undefined;
    }
    return next;
  };
  const save = async (): Promise<void> => {
    setBusyTarget("settings");
    const saved = await persistCurrentSettings();
    if (saved) {
      setDraft(saved);
      setNotice("AI 设置已保存并立即应用");
      await refreshCanonical();
    }
    setBusyTarget(undefined);
  };
  const putSecret = async (target: "text" | "vision"): Promise<void> => {
    const key = target === "text" ? textKey : visionKey;
    setBusyTarget(target);
    const saved = await persistCurrentSettings();
    if (!saved) { setBusyTarget(undefined); return; }
    const result = await control.putAISecret(target, key);
    if (!result.ok) {
      setNotice("API Key 保存失败");
      setBusyTarget(undefined);
      return;
    }
    setDraft({ ...saved, [target]: { ...saved[target], api_key_configured: true } });
    if (target === "text") setTextKey(""); else setVisionKey("");
    setNotice(`${target === "text" ? "文字" : "视觉"} API Key 已安全保存`);
    await refreshCanonical();
    setBusyTarget(undefined);
  };
  const deleteSecret = async (target: "text" | "vision"): Promise<void> => {
    if (!window.confirm("确认删除本机保存的 API Key？")) return;
    setBusyTarget(target);
    const saved = await persistCurrentSettings();
    if (!saved) { setBusyTarget(undefined); return; }
    const result = await control.deleteAISecret(target);
    if (result.ok) {
      setDraft({ ...saved, [target]: { ...saved[target], api_key_configured: false } });
      setNotice("API Key 已删除");
      await refreshCanonical();
    } else setNotice("API Key 删除失败");
    setBusyTarget(undefined);
  };
  const test = async (target: "text" | "vision"): Promise<void> => {
    setBusyTarget(target); setNotice("正在保存当前配置…");
    if (!await persistCurrentSettings()) { setBusyTarget(undefined); return; }
    setNotice("正在执行真实连接测试…");
    const result = await control.testAIConnection(target);
    const errorLabels: Record<string, string> = {
      authentication_failed: "API Key 无效或无权访问",
      model_rate_limited: "当前模型限流，请稍后重试",
      account_rate_limited: "账号或配额受限，未切换模型",
      timeout: "模型响应超时",
      network_unavailable: "网络暂时不可用",
      model_not_found: "模型不存在或当前账号不可用",
      invalid_response: "服务返回格式不兼容",
      provider_unavailable: "服务商当前不可用",
      unavailable: "连接暂时不可用",
      model_unavailable: "当前模型没有可用通道",
      proxy_unreachable: "代理无法连接",
      tls_failed: "TLS 安全连接失败",
      invalid_output: "模型返回内容无效",
    };
    setNotice(result.ok ? `连接正常 · ${result.provider} / ${result.model} · ${result.latency_ms}ms` : `连接失败 · ${errorLabels[result.error_kind ?? "provider_unavailable"] ?? "未知错误"}`);
    await refreshCanonical();
    setBusyTarget(undefined);
  };
  const testProxy = async (): Promise<void> => {
    setBusyTarget("proxy"); setProxyNotice({ label: "正在保存当前配置…", tone: "neutral" });
    if (!await persistCurrentSettings()) {
      setProxyNotice({ label: "网络设置保存失败", tone: "warning" });
      setBusyTarget(undefined);
      return;
    }
    setProxyNotice({ label: "正在测试网络连接…", tone: "neutral" });
    const result = await control.testAIProxy();
    const errorLabels: Record<string, string> = {
      timeout: "网络连接超时", proxy_unreachable: "代理无法连接", network_unavailable: "网络暂时不可用",
      tls_failed: "TLS 安全连接失败", provider_unavailable: "服务商当前不可用", unavailable: "连接暂时不可用",
    };
    setProxyNotice({
      label: result.ok ? `网络可达 · ${result.latency_ms}ms` : errorLabels[result.error_kind ?? "unavailable"] ?? "连接暂时不可用",
      tone: result.ok ? "success" : "warning",
    });
    await refreshCanonical();
    setBusyTarget(undefined);
  };
  return <section className="surface-section data-card settings-card ai-settings-card"><div className="section-header"><div><h2>AI 智能判断</h2><p>优先使用本地规则；视觉 AI 默认关闭，只作为进一步兜底。</p></div><label className="switch-label"><input type="checkbox" checked={settings.enabled} disabled={busyTarget !== undefined} onChange={event => setDraft({ ...settings, enabled: event.target.checked })} />{settings.enabled ? "开启" : "关闭"}</label></div>
    <label className="confidence-field"><span>最低置信度 {Math.round(settings.min_confidence * 100)}%</span><input type="range" min={0.5} max={1} step={0.05} value={settings.min_confidence} disabled={busyTarget !== undefined} onChange={event => setDraft({ ...settings, min_confidence: Number(event.target.value) })} /></label>
    <div className="ai-network-card ai-subsection-card"><div className="ai-endpoint-heading"><div><strong>网络连接</strong><span>文字判断、视觉判断和每日复盘共用此代理策略。</span></div><div className="ai-card-heading-actions">{proxyNotice && <span className={`ai-network-status is-${proxyNotice.tone}`} role="status" aria-live="polite">{proxyNotice.label}</span>}<button className="ai-card-action" type="button" disabled={busyTarget !== undefined} onClick={() => void testProxy()}>测试网络</button></div></div><div className="ai-proxy-grid"><label><span>代理模式</span><select value={settings.proxy.mode} disabled={busyTarget !== undefined} onChange={event => setDraft({ ...settings, proxy: { mode: event.target.value as NativeAISettings["proxy"]["mode"], url: event.target.value === "manual" ? settings.proxy.url : "" } })}><option value="environment">环境变量</option><option value="direct">直连</option><option value="manual">手动代理</option></select></label>{settings.proxy.mode === "manual" ? <><label className="wide"><span>代理地址</span><input value={settings.proxy.url} placeholder="http://127.0.0.1:7890" disabled={busyTarget !== undefined} onChange={event => setDraft({ ...settings, proxy: { ...settings.proxy, url: event.target.value } })} /></label><p className="ai-network-help">仅支持 http/https 主机地址，不填写账号密码、路径或查询参数。</p></> : <p className="ai-network-help">{settings.proxy.mode === "environment" ? "读取 Supervisor 进程的 HTTP_PROXY/HTTPS_PROXY，不读取 Windows 系统代理。" : "不使用代理，直接连接服务商。"}</p>}</div></div>
    <AIEndpointEditor title="文字判断" target="text" endpoint={settings.text} keyValue={textKey} onKeyValue={setTextKey} onFallbackChange={value => setFallbackDraft("text", value)} fallbackDraft={fallbackValue("text")} fallbackError={fallbackErrors.text} onFallbackBlur={() => commitFallbackDraft("text")} onChange={value => endpoint("text", value)} onPutSecret={() => void putSecret("text")} onDeleteSecret={() => void deleteSecret("text")} onTest={() => void test("text")} busy={busyTarget !== undefined} />
    <AIEndpointEditor title="视觉判断" target="vision" endpoint={settings.vision} keyValue={visionKey} onKeyValue={setVisionKey} onFallbackChange={value => setFallbackDraft("vision", value)} fallbackDraft={fallbackValue("vision")} fallbackError={fallbackErrors.vision} onFallbackBlur={() => commitFallbackDraft("vision")} onChange={value => endpoint("vision", value)} onPutSecret={() => void putSecret("vision")} onDeleteSecret={() => void deleteSecret("vision")} onTest={() => void test("vision")} busy={busyTarget !== undefined} />
    <div className="setting-actions"><button className="primary-button" type="button" disabled={busyTarget !== undefined} onClick={() => void save()}>保存并应用 AI 设置</button>{notice && <span role="status">{notice}</span>}</div>
  </section>;
}

function SettingsPage({ snapshot, onRefresh }: { snapshot?: SupervisorDashboardSnapshot; onRefresh?: DashboardRefresh }): ReactElement {
  const [targetInput, setTargetInput] = useState("");
  const [quietDraft, setQuietDraft] = useState<Array<{ start: string; end: string }>>();
  const [notice, setNotice] = useState("");
  const currentTarget = snapshot?.motivation?.daily_target_minutes;
  const inputValue = targetInput !== "" ? targetInput : currentTarget?.toString() ?? "";
  const quietPeriods = quietDraft ?? snapshot?.reminder_settings?.quiet_periods ?? [
    { start: "12:00", end: "14:00" }, { start: "17:30", end: "19:00" }, { start: "21:00", end: "24:00" },
  ];
  const control = getSupervisorControlAdapter();
  const [autostart, setAutostart] = useState<{ enabled: boolean; available: boolean }>({ enabled: false, available: true });
  const [autostartBusy, setAutostartBusy] = useState(false);
  useEffect(() => {
    let active = true;
    void getSystemIntegrationAdapter().getAutostartState().then(state => { if (active) setAutostart(state); });
    return () => { active = false; };
  }, []);
  const toggleAutostart = async (): Promise<void> => {
    setAutostartBusy(true);
    const state = await getSystemIntegrationAdapter().setAutostartEnabled(!autostart.enabled);
    setAutostart(state);
    setNotice(state.available ? (state.enabled ? "已开启开机启动" : "已关闭开机启动") : "系统启动设置暂时不可用");
    setAutostartBusy(false);
  };
  const saveTarget = async (): Promise<void> => {
    const minutes = Number(inputValue);
    if (!Number.isSafeInteger(minutes) || minutes < 1 || minutes > 1440) { setNotice("请输入 1–1440 分钟"); return; }
    const result = await control.setDailyTarget(minutes);
    setNotice(result.ok ? "每日目标已保存" : "每日目标暂时无法保存");
    if (result.ok) setTargetInput("");
  };
  const saveQuiet = async (): Promise<void> => {
    const validClock = (value: string, end: boolean): boolean => /^(?:[01]\d|2[0-3]):[0-5]\d$/.test(value) || (end && value === "24:00");
    if (quietPeriods.some(period => !validClock(period.start, false) || !validClock(period.end, true))) { setNotice("时间格式应为 HH:MM；24:00 只能作为结束时间"); return; }
    const result = await control.setReminderSettings(snapshot?.reminder_settings?.cooldown_minutes ?? 10, quietPeriods);
    setNotice(result.ok ? "免打扰时段已保存并立即生效" : "时段重叠、顺序无效或暂时无法保存");
    if (result.ok) setQuietDraft(undefined);
  };
  const updateQuiet = (index: number, key: "start" | "end", value: string): void => setQuietDraft(quietPeriods.map((period, itemIndex) => itemIndex === index ? { ...period, [key]: value } : period));
  return <DataPage title="设置" description="设置保存在本机；token 和 AI secret 只在 native 端读取。"><div className="settings-stack">
    <section className="surface-section data-card autostart-card"><div className="section-header"><div><h2>Windows 启动</h2><p>登录 Windows 后在后台启动 StudyGuardian。</p></div><label className="switch-label"><input type="checkbox" checked={autostart.enabled} disabled={!autostart.available || autostartBusy} onChange={() => void toggleAutostart()} />{autostart.enabled ? "开启" : "关闭"}</label></div>{!autostart.available && <small>当前安装中找不到稳定启动器，请重新部署后再试。</small>}</section>
    <section className="surface-section data-card settings-card"><div className="section-header"><div><h2>每日专注目标</h2><p>目标会写入 canonical motivation storage</p></div><Settings2 className="section-icon" size={20} /></div><label className="setting-field"><span>目标分钟数</span><input type="number" min={1} max={1440} value={inputValue} onChange={event => setTargetInput(event.target.value)} /><small>范围 1–1440 分钟</small></label><div className="setting-actions"><button className="primary-button" type="button" onClick={() => void saveTarget()}>保存目标</button></div></section>
    <section className="surface-section data-card settings-card"><div className="section-header"><div><h2>免打扰时段</h2><p>这些时段继续记录学习状态，但不主动弹出提醒。</p></div><ShieldCheck className="section-icon" size={20} /></div>
      <div className="quiet-period-list">{quietPeriods.map((period, index) => <div className="quiet-period-row" key={`${index}-${period.start}-${period.end}`}><input aria-label={`时段 ${index + 1} 开始`} inputMode="numeric" maxLength={5} value={period.start} onChange={event => updateQuiet(index, "start", event.target.value)} /><span>—</span><input aria-label={`时段 ${index + 1} 结束`} inputMode="numeric" maxLength={5} value={period.end} onChange={event => updateQuiet(index, "end", event.target.value)} /><button className="icon-button" type="button" aria-label={`删除时段 ${index + 1}`} onClick={() => setQuietDraft(quietPeriods.filter((_, itemIndex) => itemIndex !== index))}><Trash2 size={16} /></button></div>)}</div>
      <div className="setting-actions"><button className="secondary-button" type="button" disabled={quietPeriods.length >= 12} onClick={() => setQuietDraft([...quietPeriods, { start: "09:00", end: "10:00" }])}><Plus size={16} />添加时段</button><button className="primary-button" type="button" onClick={() => void saveQuiet()}>保存免打扰</button></div>
    </section>
    <AutomationSettingsCard settings={snapshot?.automation_settings} pending={snapshot?.status?.pending_automation_intent} onRefresh={onRefresh} />
    <AISettingsPanel settings={snapshot?.ai_settings} onRefresh={onRefresh} />
    {notice && <span className="settings-notice" role="status">{notice}</span>}
  </div></DataPage>;
}

function AutomationSettingsCard({ settings: source, pending, onRefresh }: { settings?: NativeAutomationSettings; pending?: NativeAutomationIntent; onRefresh?: DashboardRefresh }): ReactElement {
  const fallback: NativeAutomationSettings = { enabled: false, auto_start: { enabled: true, focused_stable_seconds: 90, min_confidence: .8, allow_unclassified: true, confirm: false }, auto_pause: { enabled: true, idle_static_seconds: 300, locked_seconds: 15, confirm: false }, auto_resume: { enabled: true, focused_stable_seconds: 45 }, transition_cooldown_seconds: 30, manual_override_minutes: 30 };
  const [draft, setDraft] = useState<NativeAutomationSettings>(source ?? fallback);
  const [busy, setBusy] = useState(false);
  const [notice, setNotice] = useState("");
  const [pendingBusy, setPendingBusy] = useState(false);
  useEffect(() => { if (source) setDraft(source); }, [source]);
  const resolvePending = async (accept: boolean): Promise<void> => {
    if (!pending) return;
    setPendingBusy(true);
    const control = getSupervisorControlAdapter();
    const result = accept ? await control.acceptAutomationIntent?.(pending.intent_id) : await control.rejectAutomationIntent?.(pending.intent_id);
    setNotice(result?.ok ? (accept ? "已接受自动转场" : "已拒绝自动转场") : "自动转场响应失败");
    await onRefresh?.();
    setPendingBusy(false);
  };
  const save = async (): Promise<void> => {
    setBusy(true);
    const control = getSupervisorControlAdapter();
    const result = control.saveAutomationSettings ? await control.saveAutomationSettings(draft) : { ok: false as const, error_kind: "unavailable" as const };
    setNotice(result.ok ? "自动学习计时设置已保存" : "自动学习计时设置暂时无法保存");
    if (result.ok) await onRefresh?.();
    setBusy(false);
  };
  return <section className="surface-section data-card settings-card automation-settings-card"><div className="section-header"><div><h2>自动学习计时</h2><p>本地规则与语义判断只生成转场意图；实际模式切换仍由 Supervisor 控制。</p></div><label className="switch-label"><input type="checkbox" checked={draft.enabled} disabled={busy} onChange={event => setDraft({ ...draft, enabled: event.target.checked })} />{draft.enabled ? "开启" : "关闭"}</label></div>
    {pending && <div className="automation-pending-intent" role="status"><div><strong>{pending.transition === "AUTO_PAUSE" ? "检测到你可能已离开" : "检测到你正在学习"}</strong><span>{pending.transition === "AUTO_PAUSE" ? "是否暂停计时？" : `是否开始“${pending.task || "当前任务"}”的计时？`}</span></div><div className="setting-actions"><button className="primary-button" type="button" disabled={pendingBusy} onClick={() => void resolvePending(true)}>接受</button><button className="secondary-button" type="button" disabled={pendingBusy} onClick={() => void resolvePending(false)}>拒绝</button></div></div>}
    <div className="automation-setting-grid"><label><span>自动开始</span><input type="checkbox" checked={draft.auto_start.enabled} disabled={busy || !draft.enabled} onChange={event => setDraft({ ...draft, auto_start: { ...draft.auto_start, enabled: event.target.checked } })} /></label><label><span>自动暂停</span><input type="checkbox" checked={draft.auto_pause.enabled} disabled={busy || !draft.enabled} onChange={event => setDraft({ ...draft, auto_pause: { ...draft.auto_pause, enabled: event.target.checked } })} /></label><label><span>自动恢复</span><input type="checkbox" checked={draft.auto_resume.enabled} disabled={busy || !draft.enabled} onChange={event => setDraft({ ...draft, auto_resume: { ...draft.auto_resume, enabled: event.target.checked } })} /></label><label><span>开始前确认</span><input type="checkbox" checked={draft.auto_start.confirm} disabled={busy || !draft.enabled} onChange={event => setDraft({ ...draft, auto_start: { ...draft.auto_start, confirm: event.target.checked } })} /></label><label><span>暂停前提醒</span><input type="checkbox" checked={draft.auto_pause.confirm} disabled={busy || !draft.enabled} onChange={event => setDraft({ ...draft, auto_pause: { ...draft.auto_pause, confirm: event.target.checked } })} /></label><label><span>开始稳定秒数</span><input type="number" min={1} max={3600} value={draft.auto_start.focused_stable_seconds} disabled={busy} onChange={event => setDraft({ ...draft, auto_start: { ...draft.auto_start, focused_stable_seconds: Number(event.target.value) } })} /></label><label><span>静止暂停秒数</span><input type="number" min={1} max={86400} value={draft.auto_pause.idle_static_seconds} disabled={busy} onChange={event => setDraft({ ...draft, auto_pause: { ...draft.auto_pause, idle_static_seconds: Number(event.target.value) } })} /></label><label><span>锁屏暂停秒数</span><input type="number" min={1} max={3600} value={draft.auto_pause.locked_seconds} disabled={busy} onChange={event => setDraft({ ...draft, auto_pause: { ...draft.auto_pause, locked_seconds: Number(event.target.value) } })} /></label><label><span>恢复稳定秒数</span><input type="number" min={1} max={3600} value={draft.auto_resume.focused_stable_seconds} disabled={busy} onChange={event => setDraft({ ...draft, auto_resume: { ...draft.auto_resume, focused_stable_seconds: Number(event.target.value) } })} /></label></div>
    <p className="settings-help">自动暂停只处理锁屏或持续静止；娱乐内容仍记录为偏离，不会擅自结束学习。手动休息或结束学习不会自动恢复。</p><div className="setting-actions"><button className="primary-button" type="button" disabled={busy} onClick={() => void save()}>保存自动计时</button>{notice && <span role="status">{notice}</span>}</div>
  </section>;
}
function ComingSoon({ title }: { title: string }): ReactElement {
  return <div className="coming-page"><div className="coming-icon"><Sparkles size={24} /></div><h1>{title}</h1><p>这个入口已经为 Control Center 预留，当前阶段先完成总览与视觉基础。</p><button className="secondary-button" type="button"><Play size={16} />回到总览</button></div>;
}

function LiveSection({ active, snapshot, live, onRefresh }: { active: string; snapshot?: SupervisorDashboardSnapshot; live: boolean; onRefresh?: DashboardRefresh }): ReactElement {
  if (!live) return <ComingSoon title={displayTitle(active)} />;
  switch (active) {
    case "missions": return <MissionsPage missions={snapshot?.missions} taskPresets={snapshot?.task_presets} onRefresh={onRefresh} />;
    case "achievements": return <AchievementsPage achievements={snapshot?.achievements} />;
    case "rewards": return <RewardsPage rewards={snapshot?.rewards} />;
    case "review": return <ReviewPage review={snapshot?.review} onRefresh={onRefresh} />;
    case "history": return <HistoryPage history={snapshot?.history} />;
    case "system": return <SystemPage snapshot={snapshot} />;
    case "settings": return <SettingsPage snapshot={snapshot} onRefresh={onRefresh} />;
    default: return <ComingSoon title={displayTitle(active)} />;
  }
}

export function ControlCenter({ snapshot, live = false, initialActive = "overview", routeRevision = 0, onTaskChanged, onTaskMutationStarted, onRefresh, onOpenHelp }: DashboardProps): ReactElement {
  const [active, setActive] = useState(initialActive);
  const [helpOpen, setHelpOpen] = useState(false);
  useEffect(() => setActive(initialActive), [initialActive, routeRevision]);
  const supervision = deriveSupervisionState(live ? Boolean(snapshot?.connected) : true, snapshot?.status);
  const serviceLabel = live ? supervision.systemLabel : "本地服务正常";
  return <div className="control-center-shell">
    <aside className="center-sidebar">
      <div className="center-brand"><BrandMark className="center-brand-mark" /><div><strong>StudyGuardian</strong><span>专注工作台</span></div></div>
      <div className="center-sidebar-content">
        <div className="nav-label">工作台</div>{navGroup(primaryNav, active, setActive)}
        <div className="nav-divider" />
        <div className="nav-label">管理</div>{navGroup(secondaryNav, active, setActive)}
      </div>
      <div className="center-sidebar-footer"><div className={`sidebar-health is-${live ? supervision.systemTone : "success"}`}><ShieldCheck size={16} /><div><strong>{serviceLabel}</strong><span>{live && !snapshot?.connected ? "等待响应" : "刚刚更新"}</span></div></div><button className="profile-button" type="button" aria-label="打开帮助" onClick={() => setHelpOpen(true)}><CircleHelp size={16} /></button></div>
    </aside>
    <main className="center-main">
      <header className="center-topbar"><div><span className="breadcrumb">StudyGuardian <ChevronRight size={14} />{displayTitle(active)}</span><span className="topbar-note">数据保存在本机</span></div><div className="topbar-actions"><button className="icon-button" type="button" aria-label="查看通知"><Activity size={17} /></button><button className="avatar-button" type="button" aria-label="用户菜单">SG</button></div></header>
      {active === "overview" ? <Dashboard snapshot={snapshot} live={live} onNavigate={setActive} onTaskChanged={onTaskChanged} onTaskMutationStarted={onTaskMutationStarted} onRefresh={onRefresh} onOpenHelp={() => setHelpOpen(true)} /> : <LiveSection active={active} snapshot={snapshot} live={live} onRefresh={onRefresh} />}
    </main>
    <HelpDrawer open={helpOpen} onClose={() => setHelpOpen(false)} onNavigate={setActive} />
  </div>;
}

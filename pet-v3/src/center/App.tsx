import { useState, type ComponentType, type ReactElement } from "react";
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
  Play,
  Settings2,
  ShieldCheck,
  Sparkles,
  Target,
  Trophy,
  WalletCards,
} from "lucide-react";
import { Area, AreaChart, ResponsiveContainer, Tooltip, XAxis, YAxis } from "recharts";
import { clampProgress, formatFocusMinutes, totalFocusMinutes, type FocusDay } from "../shared/models/dashboard";
import type { NativeMission, SupervisorDashboardSnapshot } from "../transport/supervisor";

type NavItem = { id: string; label: string; icon: ComponentType<{ size?: number; strokeWidth?: number }> };

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

type DashboardProps = { snapshot?: SupervisorDashboardSnapshot; live?: boolean };

const modeTitle: Record<"STANDBY" | "STUDY" | "BREAK" | "OFF", string> = {
  STANDBY: "准备开始",
  STUDY: "学习中",
  BREAK: "休息中",
  OFF: "今天已结束",
};

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

function Dashboard({ snapshot, live = false }: DashboardProps): ReactElement {
  const status = snapshot?.status;
  const motivation = snapshot?.motivation;
  const liveData = live && snapshot?.connected === true;
  const currentMode = status?.user_mode ?? (liveData ? "STANDBY" : "STUDY");
  const currentTask = status?.task || (liveData ? "未设置任务" : "Go Context 与 goroutine");
  const currentMinutes = motivation?.today_credited_focus_minutes ?? 0;
  const targetMinutes = motivation?.daily_target_minutes ?? 0;
  const progress = motivation?.target_progress ?? (liveData ? 0 : clampProgress(86 / 120));
  const chartData = liveData
    ? (snapshot?.history ?? []).slice().reverse().map(day => ({ label: day.date.slice(5), minutes: day.focus_minutes, target: day.target_minutes, completed: day.target_completed }))
    : focusData;
  const rows = liveData ? liveMissionRows(snapshot?.missions) : missionRows;
  const liveAchievement = snapshot?.achievements?.filter(item => !item.unlocked).sort((a, b) => a.progress - b.progress)[0] ?? snapshot?.achievements?.[0];
  const achievementView = liveData
    ? liveAchievement
      ? { title: liveAchievement.name, description: liveAchievement.description, progress: liveAchievement.progress, detail: `${Math.round(liveAchievement.progress * 100)}%` }
      : { title: "暂无成就数据", description: "完成一次有效专注后，这里会出现下一步目标", progress: 0, detail: "等待记录" }
    : achievement;
  const weeklyFocus = liveData ? totalFocusMinutes(chartData) : 603;
  const healthLabel = live ? (snapshot?.connected ? "监督正常" : "服务连接中") : "监督正常";
  const modeCaption = currentMode === "STUDY" ? "把注意力放回当下，剩下的交给节奏。" : currentMode === "BREAK" ? "短暂离开屏幕，再回来继续。" : currentMode === "OFF" ? "今天已经收好，明天再从容开始。" : "给今天留下一点可见的进展。";
  const progressLabel = motivation ? `${Math.round(progress * 100)}%` : liveData ? "—" : "72%";
  const targetLabel = motivation ? `${targetMinutes} min` : liveData ? "—" : "120 min";
  return <div className="dashboard-page">
    <div className="page-heading">
      <div><p className="heading-kicker">{liveData ? "今天" : "2026 年 9 月 4 日 · 星期五"}</p><h1>{liveData ? "今天，保持一点进展就够了" : "今天，保持一点进展就够了"}</h1><p className="heading-subtitle">{modeCaption}</p></div>
      <button className="quiet-button" type="button"><CircleHelp size={17} />帮助</button>
    </div>

    <section className="focus-hero" aria-labelledby="current-focus-title">
      <div className="hero-main">
        <div className="hero-topline"><span className="hero-kicker"><span className="live-dot" />当前状态</span><span className="hero-health"><ShieldCheck size={15} />{healthLabel}</span></div>
        <h2 id="current-focus-title">{modeTitle[currentMode]}</h2>
        <p className="hero-task"><BookOpen size={17} />{currentTask}</p>
        <p className="hero-caption">{liveData ? (status?.user_mode === "STUDY" ? `已保持专注 ${formatFocusMinutes(Math.floor(status.study_seconds / 60))}，继续完成眼前这一小段。` : modeCaption) : "已保持专注 42 分钟，继续完成眼前这一小段。"}</p>
        <div className="hero-actions"><button className="primary-button" type="button"><CoffeeIcon />休息一下</button><button className="secondary-button" type="button">结束学习</button></div>
      </div>
      <div className="hero-progress" aria-label={motivation ? `今日目标 ${progressLabel}` : "今日目标等待数据"}>
        <div className="progress-ring" style={{ background: `conic-gradient(var(--sg-accent) ${progress * 360}deg, var(--sg-accent-soft) 0)` }}><div><strong>{progressLabel}</strong><span>今日目标</span></div></div>
        <div className="ring-copy"><strong>{motivation ? currentMinutes : liveData ? "—" : 86} <span>/ {targetLabel}</span></strong><span>今日有效专注</span></div>
      </div>
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

      <section className="surface-section mission-section" aria-labelledby="missions-title">
        <div className="section-header"><div><h2 id="missions-title">今日任务</h2><p>完成小步，也算进展</p></div><button className="icon-button" type="button" aria-label="更多任务操作"><MoreHorizontal size={18} /></button></div>
        <div className="mission-list">{rows.length > 0 ? rows.map(row => <div className={`mission-row ${row.done ? "is-done" : ""}`} key={row.title}>
          <span className="mission-check">{row.done && <Check size={14} />}</span><div className="mission-copy"><strong>{row.title}</strong><span>{row.note}</span></div><span className="mission-reward">{row.reward}</span>
        </div>) : <div className="mission-empty">暂无任务记录</div>}</div>
        <button className="section-link" type="button">查看全部任务<ChevronRight size={16} /></button>
      </section>

      <section className="surface-section achievement-section" aria-labelledby="achievement-title">
        <div className="section-header"><div><h2 id="achievement-title">下一步成就</h2><p>{liveData ? "来自 Supervisor 的当前进度" : "再坚持两天，就到了"}</p></div><Trophy className="section-icon" size={20} /></div>
        <div className="achievement-highlight"><span className="achievement-icon"><Target size={20} /></span><div><strong>{achievementView.title}</strong><span>{achievementView.description}</span></div></div>
        <div className="achievement-progress"><div className="thin-progress"><span style={{ width: `${achievementView.progress * 100}%` }} /></div><span>{achievementView.detail}</span></div>
      </section>

      <section className="surface-section review-section" aria-labelledby="review-title">
        <div className="section-header"><div><h2 id="review-title">今日复盘</h2><p>结束学习后自动整理</p></div><span className="review-badge">待生成</span></div>
        <div className="review-body"><span className="review-icon"><BarChart3 size={20} /></span><div><strong>今天的故事还在继续</strong><span>完成一次学习后，就能看到今天的进展摘要。</span></div></div>
        <button className="section-link" type="button">打开学习复盘<ChevronRight size={16} /></button>
      </section>
    </div>
  </div>;
}

function CoffeeIcon(): ReactElement { return <Coffee size={17} />; }

function ComingSoon({ title }: { title: string }): ReactElement {
  return <div className="coming-page"><div className="coming-icon"><Sparkles size={24} /></div><h1>{title}</h1><p>这个入口已经为 Control Center 预留，当前阶段先完成总览与视觉基础。</p><button className="secondary-button" type="button"><Play size={16} />回到总览</button></div>;
}

export function ControlCenter({ snapshot, live = false }: DashboardProps): ReactElement {
  const [active, setActive] = useState("overview");
  const serviceLabel = live ? (snapshot?.connected ? "本地服务正常" : "正在连接本地服务") : "本地服务正常";
  return <div className="control-center-shell">
    <aside className="center-sidebar">
      <div className="center-brand"><span className="center-brand-mark"><Sparkles size={17} /></span><div><strong>StudyGuardian</strong><span>专注工作台</span></div></div>
      <div className="center-sidebar-content">
        <div className="nav-label">工作台</div>{navGroup(primaryNav, active, setActive)}
        <div className="nav-divider" />
        <div className="nav-label">管理</div>{navGroup(secondaryNav, active, setActive)}
      </div>
      <div className="center-sidebar-footer"><div className="sidebar-health"><ShieldCheck size={16} /><div><strong>{serviceLabel}</strong><span>{live && !snapshot?.connected ? "等待响应" : "刚刚更新"}</span></div></div><button className="profile-button" type="button" aria-label="打开帮助"><CircleHelp size={16} /></button></div>
    </aside>
    <main className="center-main">
      <header className="center-topbar"><div><span className="breadcrumb">StudyGuardian <ChevronRight size={14} />{displayTitle(active)}</span><span className="topbar-note">数据保存在本机</span></div><div className="topbar-actions"><button className="icon-button" type="button" aria-label="查看通知"><Activity size={17} /></button><button className="avatar-button" type="button" aria-label="用户菜单">SG</button></div></header>
      {active === "overview" ? <Dashboard snapshot={snapshot} live={live} /> : <ComingSoon title={displayTitle(active)} />}
    </main>
  </div>;
}

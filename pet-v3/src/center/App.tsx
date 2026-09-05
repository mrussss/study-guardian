import { useEffect, useState, type ComponentType, type ReactElement } from "react";
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
} from "lucide-react";
import { Area, AreaChart, ResponsiveContainer, Tooltip, XAxis, YAxis } from "recharts";
import { clampProgress, formatFocusMinutes, totalFocusMinutes, type FocusDay } from "../shared/models/dashboard";
import { NativeSupervisorControlAdapter, NativeSystemIntegrationAdapter } from "../transport/supervisor";
import { TaskPicker } from "../shared/TaskPicker";
import type { NativeAchievement, NativeAIEndpointSettings, NativeAISettings, NativeMission, NativeReward, NativeReviewSummary, SupervisorDashboardSnapshot } from "../transport/supervisor";

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

type DashboardProps = { snapshot?: SupervisorDashboardSnapshot; live?: boolean; initialActive?: string; routeRevision?: number; onNavigate?: (id: string) => void };

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

function Dashboard({ snapshot, live = false, onNavigate }: DashboardProps): ReactElement {
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
  const [taskNotice, setTaskNotice] = useState("");
  const control = new NativeSupervisorControlAdapter();
  const taskOperation = async (operation: Promise<{ ok: boolean }>, success: string): Promise<boolean> => {
    const result = await operation;
    setTaskNotice(result.ok ? success : "当前任务暂时无法更新");
    return result.ok;
  };
  const saveTask = async (name: string): Promise<boolean> => {
    const created = await control.createTaskPreset(name, true);
    if (!created.ok) { setTaskNotice("任务已存在或暂时无法保存"); return false; }
    return taskOperation(control.setTask(name), "常用任务已保存并选中");
  };
  const modeAction = async (next: "STUDY" | "BREAK" | "OFF"): Promise<void> => {
    const result = next === "STUDY" ? await control.setModeStudy(currentTask === "未设置任务" ? "" : currentTask) : next === "BREAK" ? await control.setModeBreak() : await control.setModeOff();
    setTaskNotice(result.ok ? "状态已更新" : "状态暂时无法更新");
  };
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
        <TaskPicker currentTask={currentTask} presets={snapshot?.task_presets} disabled={!liveData} onSelect={id => taskOperation(control.selectTaskPreset(id), "当前任务已更新")} onTemporary={name => taskOperation(control.setTask(name), "当前任务已更新")} onSavePinned={saveTask} />
        {taskNotice && <span className="hero-notice" role="status">{taskNotice}</span>}
        <p className="hero-caption">{liveData ? (status?.user_mode === "STUDY" ? `已保持专注 ${formatFocusMinutes(Math.floor(status.study_seconds / 60))}，继续完成眼前这一小段。` : modeCaption) : "已保持专注 42 分钟，继续完成眼前这一小段。"}</p>
        <div className="hero-actions">
          {currentMode === "STUDY" && <><button className="primary-button" type="button" onClick={() => void modeAction("BREAK")}><CoffeeIcon />休息一下</button><button className="secondary-button" type="button" onClick={() => void modeAction("OFF")}>结束学习</button></>}
          {currentMode === "BREAK" && <><button className="primary-button" type="button" onClick={() => void modeAction("STUDY")}><Play size={17} />继续学习</button><button className="secondary-button" type="button" onClick={() => void modeAction("OFF")}>结束今天</button></>}
          {currentMode === "STANDBY" && <button className="primary-button" type="button" onClick={() => void modeAction("STUDY")}><Play size={17} />开始学习</button>}
          {currentMode === "OFF" && <><button className="primary-button" type="button" onClick={() => onNavigate?.("review")}><BookOpen size={17} />查看今日复盘</button><button className="secondary-button" type="button" onClick={() => void modeAction("STUDY")}>重新开始学习</button></>}
        </div>
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
        <div className="section-header"><div><h2 id="review-title">今日复盘</h2><p>{snapshot?.review ? (snapshot.review.generation_mode === "AI" ? "AI 总结" : "本地总结 · 未使用云端 AI") : "结束学习后自动整理"}</p></div><span className="review-badge">{snapshot?.review?.status === "READY" ? "已生成" : "待生成"}</span></div>
        <div className="review-body"><span className="review-icon"><BarChart3 size={20} /></span><div><strong>{snapshot?.review?.headline ?? "今天的故事还在继续"}</strong><span>{snapshot?.review ? snapshot.review.tomorrow_priority : "完成一次学习后，就能看到今天的进展摘要。"}</span></div></div>
        <button className="section-link" type="button" onClick={() => onNavigate?.("review")}>{snapshot?.review ? "查看今日总结" : "打开学习复盘"}<ChevronRight size={16} /></button>
      </section>
    </div>
  </div>;
}

function CoffeeIcon(): ReactElement { return <Coffee size={17} />; }

function DataPage({ title, description, children }: { title: string; description: string; children: ReactElement }): ReactElement {
  return <div className="data-page"><div className="data-page-heading"><div><p className="heading-kicker">StudyGuardian · 本地数据</p><h1>{title}</h1><p>{description}</p></div></div>{children}</div>;
}

function EmptyData({ text }: { text: string }): ReactElement {
  return <div className="data-empty"><Sparkles size={20} /><span>{text}</span></div>;
}

function MissionsPage({ missions }: { missions?: NativeMission[] }): ReactElement {
  return <DataPage title="任务" description="把下一步拆小一点，完成也算进展。"><section className="surface-section data-card"><div className="section-header"><div><h2>当前任务</h2><p>Supervisor 返回的任务列表</p></div><ListChecks className="section-icon" size={20} /></div>{missions && missions.length > 0 ? <div className="data-list">{liveMissionRows(missions).map(row => <div className={`data-row ${row.done ? "is-done" : ""}`} key={row.title}><div><strong>{row.title}</strong><span>{row.note}</span></div><em>{row.reward}</em></div>)}</div> : <EmptyData text="暂无任务记录" />}</section></DataPage>;
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

function ReviewPage({ review }: { review?: NativeReviewSummary }): ReactElement {
  const [notice, setNotice] = useState("");
  const generate = async (): Promise<void> => { setNotice("正在整理本地证据…"); const result = await new NativeSupervisorControlAdapter().generateReview(); setNotice(result.ok ? "今日总结已生成，正在刷新" : "今日总结暂时无法生成"); };
  const label = review?.generation_mode === "AI" ? "AI 总结" : "本地总结";
  const reason = review?.generation_mode === "FALLBACK" ? (review.error_code && review.error_code !== "provider_not_configured" ? "AI 暂时不可用，本次已自动使用本地总结。" : "尚未配置 AI，本次使用本地证据生成。") : "通过 Provider、净化和校验链路生成。";
  return <DataPage title="学习复盘" description="摘要来自 canonical Review，不展示原始聊天或屏幕内容。"><section className="surface-section data-card">{review ? <><div className="section-header"><div><h2>{review.headline}</h2><p>{review.date} · {reason}</p></div><span className="review-badge">{label}</span></div><div className="review-detail-grid"><div><span className="eyebrow">主题</span>{review.topics.length > 0 ? review.topics.map(topic => <p key={topic.name}><strong>{topic.name}</strong> · {topic.summary}</p>) : <p>暂无足够主题证据</p>}</div><div><span className="eyebrow">不能确认</span>{review.unfinished.map(item => <p key={item}>{item}</p>)}</div><div><span className="eyebrow">明日优先级</span><p>{review.tomorrow_priority || "暂无记录"}</p></div><div><span className="eyebrow">诊断</span><p>{review.status} · revision {review.revision} · attempt {review.attempt_count} · warnings {review.warnings_count}</p></div></div>{review.status === "STALE" && <button className="primary-button" type="button" onClick={() => void generate()}>更新今日总结</button>}</> : <div className="review-generate-empty"><EmptyData text="今日总结将在结束学习后约 5 分钟自动生成" /><button className="primary-button" type="button" onClick={() => void generate()}>立即生成</button></div>}{notice && <span className="settings-notice" role="status">{notice}</span>}</section></DataPage>;
}
function SystemPage({ snapshot }: { snapshot?: SupervisorDashboardSnapshot }): ReactElement {
  const status = snapshot?.status;
  const ai = snapshot?.ai;
  return <DataPage title="系统状态" description="查看本地 Supervisor 与受限功能的健康状态。"><section className="surface-section data-card"><div className="section-header"><div><h2>本地服务</h2><p>不会显示 token、路径或原始错误</p></div><Activity className="section-icon" size={20} /></div><div className="system-status-grid"><div><span>Supervisor</span><strong>{snapshot?.connected ? "已连接" : "连接中"}</strong></div><div><span>ActivityWatch</span><strong>{status?.activitywatch_ok ? "正常" : "待检查"}</strong></div><div><span>Screen Sensor</span><strong>{status?.screen_sensor_ok ? "正常" : "待检查"}</strong></div><div><span>AI</span><strong>{ai?.enabled && ai.text_configured ? "已配置" : "规则模式"}</strong></div></div></section></DataPage>;
}

const providerHints: Record<string, { base_url: string; model: string }> = {
  none: { base_url: "", model: "" }, deepseek: { base_url: "https://api.deepseek.com", model: "deepseek-chat" },
  qwen: { base_url: "https://dashscope.aliyuncs.com/compatible-mode/v1", model: "qwen-plus" },
  kimi: { base_url: "https://api.moonshot.cn/v1", model: "moonshot-v1-8k" },
  zhipu: { base_url: "https://open.bigmodel.cn/api/paas/v4", model: "glm-4-flash" },
  siliconflow: { base_url: "https://api.siliconflow.cn/v1", model: "Qwen/Qwen2.5-7B-Instruct" },
  doubao: { base_url: "https://ark.cn-beijing.volces.com/api/v3", model: "" },
  openai: { base_url: "https://api.openai.com/v1", model: "gpt-4o-mini" },
  "openai-compatible": { base_url: "", model: "" }, ollama: { base_url: "http://127.0.0.1:11434/v1", model: "qwen2.5" },
};

function AIEndpointEditor({ title, target, endpoint, keyValue, onKeyValue, onChange, onPutSecret, onDeleteSecret, onTest }: {
  title: string; target: "text" | "vision"; endpoint: NativeAIEndpointSettings; keyValue: string; onKeyValue: (value: string) => void;
  onChange: (value: NativeAIEndpointSettings) => void; onPutSecret: () => void; onDeleteSecret: () => void; onTest: () => void;
}): ReactElement {
  const changeProvider = (provider: string): void => {
    const hint = providerHints[provider] ?? { base_url: "", model: "" };
    onChange({ ...endpoint, provider, enabled: target === "text" ? provider !== "none" : endpoint.enabled, base_url: hint.base_url, model: hint.model });
  };
  return <div className="ai-endpoint-card"><div className="ai-endpoint-heading"><div><strong>{title}</strong><span>{target === "vision" ? "仅在文字判断仍不确定且明确启用时使用" : "本地规则无法判断时才调用"}</span></div>{target === "vision" && <label className="switch-label"><input type="checkbox" checked={endpoint.enabled} onChange={event => onChange({ ...endpoint, enabled: event.target.checked })} />启用</label>}</div>
    <div className="ai-field-grid"><label><span>服务商</span><select value={endpoint.provider} onChange={event => changeProvider(event.target.value)}>{Object.keys(providerHints).map(value => <option value={value} key={value}>{value}</option>)}</select></label><label><span>模型</span><input value={endpoint.model} onChange={event => onChange({ ...endpoint, model: event.target.value })} /></label><label className="wide"><span>API 地址</span><input value={endpoint.base_url} onChange={event => onChange({ ...endpoint, base_url: event.target.value })} /></label><label><span>JSON 模式</span><select value={endpoint.json_mode} onChange={event => onChange({ ...endpoint, json_mode: event.target.value as NativeAIEndpointSettings["json_mode"] })}><option value="auto">自动</option><option value="json_object">JSON Object</option><option value="off">关闭</option></select></label><label><span>超时（秒）</span><input type="number" min={1} max={120} value={endpoint.timeout_seconds} onChange={event => onChange({ ...endpoint, timeout_seconds: Number(event.target.value) })} /></label></div>
    <div className="secret-row"><span className={endpoint.api_key_configured ? "secret-state is-set" : "secret-state"}>{endpoint.api_key_configured ? "API Key 已配置" : "API Key 未配置"}</span><input type="password" autoComplete="new-password" value={keyValue} placeholder="输入新 Key（不会回显）" onChange={event => onKeyValue(event.target.value)} /><button type="button" disabled={!keyValue.trim()} onClick={onPutSecret}>保存 Key</button>{endpoint.api_key_configured && <button type="button" onClick={onDeleteSecret}>删除 Key</button>}<button className="test-button" type="button" onClick={onTest}>测试连接</button></div>
  </div>;
}

function AISettingsPanel({ settings: source }: { settings?: NativeAISettings }): ReactElement {
  const fallback: NativeAISettings = { enabled: false, min_confidence: .75, text: { enabled: false, provider: "none", model: "", base_url: "", api_key_configured: false, timeout_seconds: 6, json_mode: "auto" }, vision: { enabled: false, provider: "none", model: "", base_url: "", api_key_configured: false, timeout_seconds: 8, json_mode: "auto" } };
  const [draft, setDraft] = useState<NativeAISettings>();
  const [textKey, setTextKey] = useState(""); const [visionKey, setVisionKey] = useState(""); const [notice, setNotice] = useState("");
  const settings = draft ?? source ?? fallback; const control = new NativeSupervisorControlAdapter();
  const endpoint = (target: "text" | "vision", value: NativeAIEndpointSettings): void => setDraft({ ...settings, [target]: value });
  const save = async (): Promise<void> => { const result = await control.saveAISettings(settings); setNotice(result.ok ? "AI 设置已保存并立即应用" : "AI 设置未通过验证或暂时无法保存"); if (result.ok) setDraft(undefined); };
  const putSecret = async (target: "text" | "vision"): Promise<void> => { const key = target === "text" ? textKey : visionKey; const result = await control.putAISecret(target, key); if (target === "text") setTextKey(""); else setVisionKey(""); setNotice(result.ok ? `${target === "text" ? "文字" : "视觉"} API Key 已安全保存` : "API Key 保存失败"); };
  const deleteSecret = async (target: "text" | "vision"): Promise<void> => { if (!window.confirm("确认删除本机保存的 API Key？")) return; const result = await control.deleteAISecret(target); setNotice(result.ok ? "API Key 已删除" : "API Key 删除失败"); };
  const test = async (target: "text" | "vision"): Promise<void> => { setNotice("正在执行真实连接测试…"); const result = await control.testAIConnection(target); setNotice(result.ok ? `连接正常 · ${result.provider} / ${result.model} · ${result.latency_ms}ms` : `连接失败 · ${result.error_kind ?? "provider_unavailable"}`); };
  return <section className="surface-section data-card settings-card ai-settings-card"><div className="section-header"><div><h2>AI 智能判断</h2><p>优先使用本地规则；视觉 AI 默认关闭，只作为进一步兜底。</p></div><label className="switch-label"><input type="checkbox" checked={settings.enabled} onChange={event => setDraft({ ...settings, enabled: event.target.checked })} />{settings.enabled ? "开启" : "关闭"}</label></div>
    <label className="confidence-field"><span>最低置信度 {Math.round(settings.min_confidence * 100)}%</span><input type="range" min={0.5} max={1} step={0.05} value={settings.min_confidence} onChange={event => setDraft({ ...settings, min_confidence: Number(event.target.value) })} /></label>
    <AIEndpointEditor title="文字判断" target="text" endpoint={settings.text} keyValue={textKey} onKeyValue={setTextKey} onChange={value => endpoint("text", value)} onPutSecret={() => void putSecret("text")} onDeleteSecret={() => void deleteSecret("text")} onTest={() => void test("text")} />
    <AIEndpointEditor title="视觉判断" target="vision" endpoint={settings.vision} keyValue={visionKey} onKeyValue={setVisionKey} onChange={value => endpoint("vision", value)} onPutSecret={() => void putSecret("vision")} onDeleteSecret={() => void deleteSecret("vision")} onTest={() => void test("vision")} />
    <div className="setting-actions"><button className="primary-button" type="button" onClick={() => void save()}>保存并应用 AI 设置</button>{notice && <span role="status">{notice}</span>}</div>
  </section>;
}

function SettingsPage({ snapshot }: { snapshot?: SupervisorDashboardSnapshot }): ReactElement {
  const [targetInput, setTargetInput] = useState("");
  const [quietDraft, setQuietDraft] = useState<Array<{ start: string; end: string }>>();
  const [notice, setNotice] = useState("");
  const currentTarget = snapshot?.motivation?.daily_target_minutes;
  const inputValue = targetInput !== "" ? targetInput : currentTarget?.toString() ?? "";
  const quietPeriods = quietDraft ?? snapshot?.reminder_settings?.quiet_periods ?? [
    { start: "12:00", end: "14:00" }, { start: "17:30", end: "19:00" }, { start: "21:00", end: "24:00" },
  ];
  const control = new NativeSupervisorControlAdapter();
  const [autostart, setAutostart] = useState<{ enabled: boolean; available: boolean }>({ enabled: false, available: true });
  const [autostartBusy, setAutostartBusy] = useState(false);
  useEffect(() => {
    let active = true;
    void new NativeSystemIntegrationAdapter().getAutostartState().then(state => { if (active) setAutostart(state); });
    return () => { active = false; };
  }, []);
  const toggleAutostart = async (): Promise<void> => {
    setAutostartBusy(true);
    const state = await new NativeSystemIntegrationAdapter().setAutostartEnabled(!autostart.enabled);
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
    <section className="surface-section data-card settings-card"><div className="section-header"><div><h2>Windows 启动</h2><p>登录 Windows 后在后台启动 StudyGuardian。</p></div><label className="switch-label"><input type="checkbox" checked={autostart.enabled} disabled={!autostart.available || autostartBusy} onChange={() => void toggleAutostart()} />{autostart.enabled ? "开启" : "关闭"}</label></div>{!autostart.available && <small>当前安装中找不到稳定启动器，请重新部署后再试。</small>}</section>
    <section className="surface-section data-card settings-card"><div className="section-header"><div><h2>每日专注目标</h2><p>目标会写入 canonical motivation storage</p></div><Settings2 className="section-icon" size={20} /></div><label className="setting-field"><span>目标分钟数</span><input type="number" min={1} max={1440} value={inputValue} onChange={event => setTargetInput(event.target.value)} /><small>范围 1–1440 分钟</small></label><div className="setting-actions"><button className="primary-button" type="button" onClick={() => void saveTarget()}>保存目标</button></div></section>
    <section className="surface-section data-card settings-card"><div className="section-header"><div><h2>免打扰时段</h2><p>这些时段继续记录学习状态，但不主动弹出提醒。</p></div><ShieldCheck className="section-icon" size={20} /></div>
      <div className="quiet-period-list">{quietPeriods.map((period, index) => <div className="quiet-period-row" key={`${index}-${period.start}-${period.end}`}><input aria-label={`时段 ${index + 1} 开始`} inputMode="numeric" maxLength={5} value={period.start} onChange={event => updateQuiet(index, "start", event.target.value)} /><span>—</span><input aria-label={`时段 ${index + 1} 结束`} inputMode="numeric" maxLength={5} value={period.end} onChange={event => updateQuiet(index, "end", event.target.value)} /><button className="icon-button" type="button" aria-label={`删除时段 ${index + 1}`} onClick={() => setQuietDraft(quietPeriods.filter((_, itemIndex) => itemIndex !== index))}><Trash2 size={16} /></button></div>)}</div>
      <div className="setting-actions"><button className="secondary-button" type="button" disabled={quietPeriods.length >= 12} onClick={() => setQuietDraft([...quietPeriods, { start: "09:00", end: "10:00" }])}><Plus size={16} />添加时段</button><button className="primary-button" type="button" onClick={() => void saveQuiet()}>保存免打扰</button></div>
    </section>
    <AISettingsPanel settings={snapshot?.ai_settings} />
    {notice && <span className="settings-notice" role="status">{notice}</span>}
  </div></DataPage>;
}
function ComingSoon({ title }: { title: string }): ReactElement {
  return <div className="coming-page"><div className="coming-icon"><Sparkles size={24} /></div><h1>{title}</h1><p>这个入口已经为 Control Center 预留，当前阶段先完成总览与视觉基础。</p><button className="secondary-button" type="button"><Play size={16} />回到总览</button></div>;
}

function LiveSection({ active, snapshot, live }: { active: string; snapshot?: SupervisorDashboardSnapshot; live: boolean }): ReactElement {
  if (!live) return <ComingSoon title={displayTitle(active)} />;
  switch (active) {
    case "missions": return <MissionsPage missions={snapshot?.missions} />;
    case "achievements": return <AchievementsPage achievements={snapshot?.achievements} />;
    case "rewards": return <RewardsPage rewards={snapshot?.rewards} />;
    case "review": return <ReviewPage review={snapshot?.review} />;
    case "history": return <HistoryPage history={snapshot?.history} />;
    case "system": return <SystemPage snapshot={snapshot} />;
    case "settings": return <SettingsPage snapshot={snapshot} />;
    default: return <ComingSoon title={displayTitle(active)} />;
  }
}

export function ControlCenter({ snapshot, live = false, initialActive = "overview", routeRevision = 0 }: DashboardProps): ReactElement {
  const [active, setActive] = useState(initialActive);
  useEffect(() => setActive(initialActive), [initialActive, routeRevision]);
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
      {active === "overview" ? <Dashboard snapshot={snapshot} live={live} onNavigate={setActive} /> : <LiveSection active={active} snapshot={snapshot} live={live} />}
    </main>
  </div>;
}

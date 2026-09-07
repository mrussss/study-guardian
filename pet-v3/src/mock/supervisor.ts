import type {
  AutostartState, ControlResult, NativeAIConnectionResult, NativeAIProxyTestResult, NativeAISettings, NativeMission, NativeTaskPreset, ReviewGenerationResult,
  SupervisorControlAdapter, SupervisorDashboardAdapter, SupervisorDashboardSnapshot, SystemIntegrationAdapter,
} from "../transport/supervisor";

export const MOCK_SCENARIOS = [
  { id: "normal", label: "正常数据" }, { id: "slow", label: "慢请求" },
  { id: "failure", label: "请求失败" }, { id: "offline", label: "Supervisor 离线" },
  { id: "rapid", label: "快速任务切换" }, { id: "progress-empty", label: "尚未开始" },
  { id: "progress-complete", label: "目标已完成" }, { id: "reminder", label: "提醒状态" },
  { id: "sensor-failure", label: "屏幕采集异常" }, { id: "activitywatch-failure", label: "活动数据异常" },
] as const;
export type MockScenarioId = typeof MOCK_SCENARIOS[number]["id"];

export function parseMockScenario(search: string): MockScenarioId {
  const requested = new URLSearchParams(search).get("mock");
  return MOCK_SCENARIOS.some(item => item.id === requested) ? requested as MockScenarioId : "normal";
}
function clone<T>(value: T): T { return JSON.parse(JSON.stringify(value)) as T; }

function initialSnapshot(scenario: MockScenarioId): SupervisorDashboardSnapshot {
  const today = new Date().toISOString().slice(0, 10);
  const progress = scenario === "progress-empty" ? 0 : scenario === "progress-complete" ? 1 : 0.43;
  const focusMinutes = Math.round(120 * progress);
  const sensorFailure = scenario === "sensor-failure";
  const activityWatchFailure = scenario === "activitywatch-failure";
  return {
    connected: scenario !== "offline",
    status: {
      user_mode: scenario === "progress-empty" ? "STANDBY" : "STUDY",
      interaction_state: "ACTIVE", task_relation: scenario === "reminder" ? "DISTRACTED" : "FOCUSED",
      privacy_state: "NORMAL", confidence: scenario === "reminder" ? 0.58 : 0.94,
      task: scenario === "rapid" ? "Go" : "算法", study_seconds: focusMinutes * 60 + 17,
      break_seconds: 0, active_seconds: focusMinutes * 60 + 17, activitywatch_ok: !activityWatchFailure,
      screen_sensor_ok: !sensorFailure, last_activity_at: new Date().toISOString(),
    },
    motivation: {
      today_credited_focus_minutes: focusMinutes, total_credited_focus_minutes: 1842 + focusMinutes,
      today_earned_ap_milli: focusMinutes * 50, today_spent_ap_milli: 0, balance_ap_milli: 6376,
      checkin_completed: scenario === "progress-complete", daily_target_minutes: 120,
      target_progress: progress, streak_days: 5,
      ...(scenario === "reminder" ? { last_event: { id: 42, type: "DISTRACTION", message: "已偏离当前任务 2 分钟，回来继续吧", created_at: new Date().toISOString() } } : {}),
    },
    task_presets: {
      pinned: [
        { id: "go", name: "Go", pinned: true, sort_order: 0, use_count: 18 },
        { id: "algorithm", name: "算法", pinned: true, sort_order: 1, use_count: 12 },
        { id: "interview", name: "八股", pinned: true, sort_order: 2, use_count: 9 },
      ],
      recent: [{ id: "english", name: "英语", pinned: false, sort_order: 0, use_count: 4 }],
    },
    reminder_settings: { cooldown_minutes: 10, quiet_periods: [
      { start: "12:00", end: "14:00" }, { start: "17:30", end: "19:00" }, { start: "21:00", end: "24:00" },
    ] },
    ai_settings: {
      enabled: false, min_confidence: 0.75,
      proxy: { mode: "environment", url: "" },
      text: { enabled: false, provider: "none", model: "", fallback_models: [], base_url: "", api_key_configured: false, timeout_seconds: 6, json_mode: "auto" },
      vision: { enabled: false, provider: "none", model: "", fallback_models: [], base_url: "", api_key_configured: false, timeout_seconds: 8, json_mode: "auto" },
    },
    history: [
      { date: today, focus_minutes: focusMinutes, target_minutes: 120, checkin_completed: progress === 1, target_completed: progress === 1 },
      { date: "2026-09-05", focus_minutes: 96, target_minutes: 120, checkin_completed: true, target_completed: false },
      { date: "2026-09-04", focus_minutes: 132, target_minutes: 120, checkin_completed: true, target_completed: true },
      { date: "2026-09-03", focus_minutes: 64, target_minutes: 120, checkin_completed: true, target_completed: false },
      { date: "2026-09-02", focus_minutes: 105, target_minutes: 120, checkin_completed: true, target_completed: false },
      { date: "2026-09-01", focus_minutes: 78, target_minutes: 120, checkin_completed: true, target_completed: false },
      { date: "2026-08-31", focus_minutes: 42, target_minutes: 120, checkin_completed: false, target_completed: false },
    ],
    achievements: [
      { achievement_id: "week", name: "一周坚持", description: "连续打卡 7 天", progress: 0.71, unlocked: false },
      { achievement_id: "first", name: "第一次专注", description: "完成首次有效专注", progress: 1, unlocked: true, unlocked_at: "2026-09-01T09:00:00+08:00" },
    ],
    missions: [
      { id: "mission-go", title: "完成 Go Context 练习", description: "整理取消链路", reward_milli_ap: 0, status: "OPEN", created_at: today, linked_task_name: "Go", link_source: "MANUAL" },
      { id: "mission-note", title: "整理本周学习笔记", description: "提炼三个重点", reward_milli_ap: 250, status: "OPEN", created_at: today },
      { id: "mission-focus", title: "完成一次 30 分钟专注", description: "保持连续专注", reward_milli_ap: 300, status: "COMPLETED", created_at: today, completed_at: new Date().toISOString() },
    ],
    rewards: [{ id: "coffee", name: "咖啡时间", type: "BREAK", cost_milli_ap: 1200, description: "兑换一次安心休息", enabled: true }],
    ai: { enabled: false, text_provider: "none", text_configured: false, vision_enabled: false },
    review: {
      schema_version: 1, date: today, headline: "今天保持了稳定推进",
      topics: [{ name: "算法", summary: "完成了核心练习与复盘", confidence: 0.92 }],
      accomplishments: [{ text: "完成一次连续专注", confidence: 0.95 }], unfinished: ["整理错题"],
      difficulties: ["切换任务时容易分心"], behavior: { distraction_count: scenario === "reminder" ? 3 : 1, largest_distraction_seconds: 120, average_recovery_seconds: 35 },
      tomorrow_priority: "继续完成算法练习", warnings: [], status: "READY", generation_mode: "FALLBACK",
      provider: "local", model: "", revision: 1, attempt_count: 1, warnings_count: 0,
    },
  };
}

export class MockSupervisorRuntime implements SupervisorDashboardAdapter, SupervisorControlAdapter, SystemIntegrationAdapter {
  private snapshot: SupervisorDashboardSnapshot;
  private autostart = false;
  private nextPreset = 1;
  private nextMission = 1;
  constructor(readonly scenario: MockScenarioId) { this.snapshot = initialSnapshot(scenario); }

  async poll(): Promise<SupervisorDashboardSnapshot> {
    await this.wait(this.scenario === "slow" ? 900 : 25);
    return this.scenario === "offline" ? { connected: false, last_error_kind: "unavailable" } : clone(this.snapshot);
  }
  setModeStudy(task: string): Promise<ControlResult> { return this.mutate(() => {
    if (this.snapshot.status) { this.snapshot.status.user_mode = "STUDY"; if (task.trim()) this.snapshot.status.task = task.trim(); }
    return { ok: true };
  }); }
  setModeBreak(): Promise<ControlResult> { return this.mutate(() => { if (this.snapshot.status) this.snapshot.status.user_mode = "BREAK"; return { ok: true }; }); }
  setModeOff(): Promise<ControlResult> { return this.mutate(() => { if (this.snapshot.status) this.snapshot.status.user_mode = "OFF"; return { ok: true }; }); }
  setTask(task: string): Promise<ControlResult> {
    const normalized = task.trim().replace(/\s+/g, " ");
    return this.mutate(() => { if (!normalized) return { ok: false, error_kind: "rejected" }; if (this.snapshot.status) this.snapshot.status.task = normalized; return { ok: true, task: normalized }; });
  }
  createTaskPreset(name: string, pinned: boolean): Promise<ControlResult> {
    const normalized = name.trim().replace(/\s+/g, " ");
    return this.mutate(() => {
      if (!normalized || !this.snapshot.task_presets) return { ok: false, error_kind: "rejected" };
      const all = [...this.snapshot.task_presets.pinned, ...this.snapshot.task_presets.recent];
      if (!all.some(item => item.name.toLocaleLowerCase() === normalized.toLocaleLowerCase())) {
        const preset: NativeTaskPreset = { id: `mock-${this.nextPreset++}`, name: normalized, pinned, sort_order: this.snapshot.task_presets.pinned.length, use_count: 0 };
        (pinned ? this.snapshot.task_presets.pinned : this.snapshot.task_presets.recent).push(preset);
      }
      return { ok: true, task: normalized };
    });
  }
  selectTaskPreset(id: string): Promise<ControlResult> { return this.mutate(() => {
    const presets = this.snapshot.task_presets;
    const preset = presets && [...presets.pinned, ...presets.recent].find(item => item.id === id);
    if (!preset) return { ok: false, error_kind: "rejected" };
    if (this.snapshot.status) this.snapshot.status.task = preset.name;
    preset.use_count += 1; preset.last_used_at = new Date().toISOString();
    return { ok: true, task: preset.name };
  }); }
  updateTaskPreset(id: string, name: string, pinned: boolean, sortOrder: number): Promise<ControlResult> { return this.mutate(() => {
    const presets = this.snapshot.task_presets;
    const preset = presets && [...presets.pinned, ...presets.recent].find(item => item.id === id);
    if (!preset) return { ok: false, error_kind: "rejected" };
    preset.name = name.trim(); preset.pinned = pinned; preset.sort_order = sortOrder;
    presets.pinned = presets.pinned.filter(item => item.id !== id);
    presets.recent = presets.recent.filter(item => item.id !== id);
    const target = pinned ? presets.pinned : presets.recent;
    target.push(preset);
    target.sort((left, right) => left.sort_order - right.sort_order);
    return { ok: true };
  }); }
  deleteTaskPreset(id: string): Promise<ControlResult> { return this.mutate(() => {
    if (!this.snapshot.task_presets) return { ok: false, error_kind: "rejected" };
    this.snapshot.task_presets.pinned = this.snapshot.task_presets.pinned.filter(item => item.id !== id);
    this.snapshot.task_presets.recent = this.snapshot.task_presets.recent.filter(item => item.id !== id);
    return { ok: true };
  }); }
  setReminderSettings(cooldownMinutes: number, quietPeriods: Array<{ start: string; end: string }>): Promise<ControlResult> { return this.mutate(() => {
    this.snapshot.reminder_settings = { cooldown_minutes: cooldownMinutes, quiet_periods: clone(quietPeriods) }; return { ok: true };
  }); }
  saveAISettings(settings: NativeAISettings): Promise<ControlResult> { return this.mutate(() => {
    this.snapshot.ai_settings = clone(settings);
    this.snapshot.ai = { enabled: settings.enabled, text_provider: settings.text.provider, text_configured: settings.text.api_key_configured, vision_enabled: settings.vision.enabled, text_model: settings.text.model };
    return { ok: true };
  }); }
  putAISecret(target: "text" | "vision", apiKey: string): Promise<ControlResult> { return this.mutate(() => {
    if (!apiKey.trim() || !this.snapshot.ai_settings) return { ok: false, error_kind: "rejected" };
    this.snapshot.ai_settings[target].api_key_configured = true; return { ok: true };
  }); }
  deleteAISecret(target: "text" | "vision"): Promise<ControlResult> { return this.mutate(() => {
    if (this.snapshot.ai_settings) this.snapshot.ai_settings[target].api_key_configured = false; return { ok: true };
  }); }
  async testAIConnection(target: "text" | "vision"): Promise<NativeAIConnectionResult> {
    const failure = await this.beforeMutation(); const endpoint = this.snapshot.ai_settings?.[target];
    if (failure || !endpoint) return { ok: false, provider: endpoint?.provider ?? "", model: endpoint?.model ?? "", latency_ms: 0, error_kind: failure?.error_kind === "timeout" ? "timeout" : "provider_unavailable" };
    return { ok: true, provider: endpoint.provider || "mock", model: endpoint.model || "mock-model", latency_ms: this.scenario === "slow" ? 900 : 42 };
  }
  async testAIProxy(): Promise<NativeAIProxyTestResult> {
    const failure = await this.beforeMutation();
    const mode = this.snapshot.ai_settings?.proxy?.mode ?? "environment";
    if (failure) return { ok: false, mode, latency_ms: 0, error_kind: failure.error_kind === "timeout" ? "timeout" : "proxy_unreachable" };
    return { ok: true, mode, latency_ms: this.scenario === "slow" ? 900 : 42 };
  }
  async generateReview(): Promise<ReviewGenerationResult> {
    const failure = await this.beforeMutation();
    if (failure) return failure;
    if (this.snapshot.review) {
      this.snapshot.review.status = "READY";
      this.snapshot.review.revision += 1;
      return { ok: true, status: "READY", generation_mode: this.snapshot.review.generation_mode };
    }
    return { ok: true, status: "READY", generation_mode: "FALLBACK" };
  }
  setDailyTarget(minutes: number): Promise<ControlResult> { return this.mutate(() => {
    if (!Number.isSafeInteger(minutes) || minutes < 1 || minutes > 1440 || !this.snapshot.motivation) return { ok: false, error_kind: "rejected" };
    this.snapshot.motivation.daily_target_minutes = minutes;
    this.snapshot.motivation.target_progress = Math.min(1, this.snapshot.motivation.today_credited_focus_minutes / minutes);
    return { ok: true };
  }); }
  createMission(title: string, description: string, dueDate?: string, linkedTaskName?: string, linkedTaskPresetId?: string): Promise<ControlResult> {
    const normalized = title.trim().replace(/\s+/g, " ");
    return this.mutate(() => {
      if (!normalized) return { ok: false, error_kind: "rejected" };
      const mission: NativeMission = { id: `mock-mission-${this.nextMission++}`, title: normalized, description: description.trim(), reward_milli_ap: 0, status: "OPEN", created_at: new Date().toISOString(), ...(dueDate ? { due_date: dueDate } : {}), ...(linkedTaskName ? { linked_task_name: linkedTaskName, link_source: "MANUAL" as const } : {}), ...(linkedTaskPresetId ? { linked_task_preset_id: linkedTaskPresetId } : {}) };
      this.snapshot.missions = [mission, ...(this.snapshot.missions ?? [])];
      return { ok: true };
    });
  }
  completeMission(id: string): Promise<ControlResult> { return this.mutate(() => {
    const mission = this.snapshot.missions?.find(item => item.id === id);
    if (!mission) return { ok: false, error_kind: "rejected" };
    if (mission.status === "OPEN") { mission.status = "COMPLETED"; mission.completed_at = new Date().toISOString(); }
    return { ok: true };
  }); }
  cancelMission(id: string): Promise<ControlResult> { return this.mutate(() => {
    const mission = this.snapshot.missions?.find(item => item.id === id);
    if (!mission || mission.status !== "OPEN") return { ok: false, error_kind: "rejected" };
    mission.status = "CANCELLED";
    return { ok: true };
  }); }
  async getAutostartState(): Promise<AutostartState> { await this.wait(30); return { enabled: this.autostart, available: this.scenario !== "offline" }; }
  async setAutostartEnabled(enabled: boolean): Promise<AutostartState> {
    const failure = await this.beforeMutation(); if (failure) return { enabled: this.autostart, available: false };
    this.autostart = enabled; return { enabled, available: true };
  }
  private async beforeMutation(): Promise<ControlResult | undefined> {
    await this.wait(this.scenario === "slow" ? 1000 : this.scenario === "rapid" ? 400 : 60);
    if (this.scenario === "offline") return { ok: false, error_kind: "unavailable" };
    if (this.scenario === "failure") return { ok: false, error_kind: "rejected" };
    return undefined;
  }
  private async mutate(operation: () => ControlResult): Promise<ControlResult> { return (await this.beforeMutation()) ?? operation(); }
  private wait(milliseconds: number): Promise<void> { return new Promise(resolve => setTimeout(resolve, milliseconds)); }
}

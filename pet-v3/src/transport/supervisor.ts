import { invoke } from "@tauri-apps/api/core";
import { mockSemantic, mockSupervisorOffline } from "../mock/semantic";
import { isCurrentActivityView, VALID_USER_MODES, type CurrentActivityView } from "../model/semantic";

export type TransportErrorKind = "timeout" | "unauthorized" | "unavailable" | "invalid_response";
export type ControlErrorKind = TransportErrorKind | "rejected";

export interface PetTransportSnapshot {
  connected: boolean;
  semantic: CurrentActivityView;
  last_success_at?: string;
  last_error_kind?: TransportErrorKind;
}

const ERROR_KINDS: TransportErrorKind[] = ["timeout", "unauthorized", "unavailable", "invalid_response"];

function neutralSemantic(): CurrentActivityView {
  return mockSemantic({ fresh: false, activity: "UNKNOWN", confidence: 0 });
}

function errorKind(value: unknown): TransportErrorKind | undefined {
  return typeof value === "string" && ERROR_KINDS.includes(value as TransportErrorKind)
    ? value as TransportErrorKind
    : undefined;
}

function disconnected(kind: TransportErrorKind = "unavailable"): PetTransportSnapshot {
  const offline = mockSupervisorOffline();
  return { connected: offline.connected, semantic: offline.semantic, last_error_kind: kind };
}

/**
 * Validate the sanitized result boundary returned by Rust. This function has
 * no access to tokens, raw HTTP errors, or arbitrary native payload fields.
 */
export function normalizeNativeSnapshot(raw: unknown): PetTransportSnapshot {
  if (!raw || typeof raw !== "object") return disconnected("invalid_response");
  const value = raw as Record<string, unknown>;
  const connected = value.connected === true;
  const semantic = value.semantic;
  if (!connected) return disconnected(errorKind(value.last_error_kind) ?? "unavailable");
  if (!isCurrentActivityView(semantic)) return disconnected("invalid_response");
  const success = typeof value.last_success_at === "string" && Number.isFinite(Date.parse(value.last_success_at))
    ? value.last_success_at
    : undefined;
  return { connected: true, semantic, ...(success ? { last_success_at: success } : {}) };
}

function classifyNativeError(error: unknown): TransportErrorKind {
  const message = error instanceof Error ? error.message : String(error ?? "");
  if (/401|unauthorized/i.test(message)) return "unauthorized";
  if (/timeout|timed out|deadline/i.test(message)) return "timeout";
  return "unavailable";
}

export interface SupervisorAdapter {
  poll(): Promise<PetTransportSnapshot>;
}

export class NativeSupervisorAdapter implements SupervisorAdapter {
  async poll(): Promise<PetTransportSnapshot> {
    try {
      const raw = await invoke<unknown>("supervisor_snapshot");
      return normalizeNativeSnapshot(raw);
    } catch (error) {
      // Do not expose the native error text to the UI. Only a bounded kind is
      // allowed across the transport boundary; tokens and paths stay native.
      return disconnected(classifyNativeError(error));
    }
  }
}

export interface NativeSupervisorStatus {
  user_mode: "STANDBY" | "STUDY" | "BREAK" | "OFF";
  interaction_state: "ACTIVE" | "IDLE_STATIC" | "IDLE_DYNAMIC" | "UNKNOWN";
  task_relation: "FOCUSED" | "DISTRACTED" | "UNKNOWN";
  privacy_state: "NORMAL" | "SENSITIVE";
  confidence: number;
  task: string;
  study_seconds: number;
  break_seconds: number;
  active_seconds: number;
  activitywatch_ok: boolean;
  screen_sensor_ok: boolean;
  last_activity_at?: string;
}

export interface NativeMotivationStatus {
  today_credited_focus_minutes: number;
  total_credited_focus_minutes: number;
  today_earned_ap_milli: number;
  today_spent_ap_milli: number;
  balance_ap_milli: number;
  checkin_completed: boolean;
  daily_target_minutes: number;
  target_progress: number;
  streak_days: number;
  last_event?: { id: number; type: string; message: string; created_at: string };
}

export interface NativeTaskPreset {
  id: string;
  name: string;
  pinned: boolean;
  sort_order: number;
  use_count: number;
  last_used_at?: string;
}

export interface NativeTaskPresetList {
  pinned: NativeTaskPreset[];
  recent: NativeTaskPreset[];
}

export interface NativeReminderSettings {
  cooldown_minutes: number;
  quiet_periods: Array<{ start: string; end: string }>;
}

export interface NativeAIEndpointSettings {
  enabled: boolean;
  provider: string;
  model: string;
  base_url: string;
  api_key_configured: boolean;
  timeout_seconds: number;
  json_mode: "auto" | "json_object" | "off";
}

export interface NativeAISettings {
  enabled: boolean;
  min_confidence: number;
  text: NativeAIEndpointSettings;
  vision: NativeAIEndpointSettings;
}

export interface NativeAIConnectionResult {
  ok: boolean;
  provider: string;
  model: string;
  latency_ms: number;
  error_kind?: "authentication_failed" | "timeout" | "network_unavailable" | "model_not_found" | "invalid_response" | "provider_unavailable" | "unavailable";
}

export interface NativeHistoryDay {
  date: string;
  focus_minutes: number;
  target_minutes: number;
  checkin_completed: boolean;
  target_completed: boolean;
}

export interface NativeAchievement {
  achievement_id: string;
  name: string;
  description: string;
  progress: number;
  unlocked: boolean;
  unlocked_at?: string;
}

export interface NativeMission {
  id: string;
  title: string;
  description: string;
  reward_milli_ap: number;
  status: "OPEN" | "COMPLETED" | "CANCELLED";
  created_at: string;
  due_date?: string;
  completed_at?: string;
}

export interface NativeReward {
  id: string;
  name: string;
  type: string;
  cost_milli_ap: number;
  description: string;
  enabled: boolean;
}

export interface NativeAIStatus {
  enabled: boolean;
  text_provider: string;
  text_configured: boolean;
  vision_enabled: boolean;
  text_model?: string;
  warning?: string;
}

export interface NativeReviewSummary {
  schema_version: 1;
  date: string;
  headline: string;
  topics: Array<{ name: string; summary: string; confidence: number }>;
  accomplishments: Array<{ text: string; confidence: number }>;
  unfinished: string[];
  difficulties: string[];
  behavior: { distraction_count: number; largest_distraction_seconds: number; average_recovery_seconds: number };
  tomorrow_priority: string;
  warnings: string[];
  status: "PENDING" | "READY" | "STALE" | "FAILED";
  generation_mode: "AI" | "FALLBACK" | "";
  provider: string;
  model: string;
  revision: number;
  attempt_count: number;
  error_code?: string;
  warnings_count: number;
}

export interface SupervisorDashboardSnapshot {
  connected: boolean;
  status?: NativeSupervisorStatus;
  motivation?: NativeMotivationStatus;
  task_presets?: NativeTaskPresetList;
  reminder_settings?: NativeReminderSettings;
  ai_settings?: NativeAISettings;
  history?: NativeHistoryDay[];
  achievements?: NativeAchievement[];
  missions?: NativeMission[];
  rewards?: NativeReward[];
  ai?: NativeAIStatus;
  review?: NativeReviewSummary;
  last_error_kind?: TransportErrorKind;
}

function record(value: unknown): value is Record<string, unknown> {
  return !!value && typeof value === "object";
}

function boundedText(value: unknown, max: number): value is string {
  return typeof value === "string" && value.length <= max;
}

function nonNegativeInteger(value: unknown): value is number {
  return typeof value === "number" && Number.isSafeInteger(value) && value >= 0;
}

function boundedRatio(value: unknown): value is number {
  return typeof value === "number" && Number.isFinite(value) && value >= 0 && value <= 1;
}

function validStatus(value: unknown): value is NativeSupervisorStatus {
  if (!record(value)) return false;
  return VALID_USER_MODES.includes(value.user_mode as NativeSupervisorStatus["user_mode"]) &&
    ["ACTIVE", "IDLE_STATIC", "IDLE_DYNAMIC", "UNKNOWN"].includes(value.interaction_state as string) &&
    ["FOCUSED", "DISTRACTED", "UNKNOWN"].includes(value.task_relation as string) &&
    ["NORMAL", "SENSITIVE"].includes(value.privacy_state as string) &&
    boundedRatio(value.confidence) && boundedText(value.task, 4096) &&
    nonNegativeInteger(value.study_seconds) && nonNegativeInteger(value.break_seconds) &&
    nonNegativeInteger(value.active_seconds) && typeof value.activitywatch_ok === "boolean" &&
    typeof value.screen_sensor_ok === "boolean" &&
    (value.last_activity_at === undefined || boundedText(value.last_activity_at, 128));
}

function validMotivation(value: unknown): value is NativeMotivationStatus {
  if (!record(value)) return false;
  const lastEvent = value.last_event;
  return nonNegativeInteger(value.today_credited_focus_minutes) &&
    nonNegativeInteger(value.total_credited_focus_minutes) &&
    nonNegativeInteger(value.today_earned_ap_milli) && nonNegativeInteger(value.today_spent_ap_milli) &&
    nonNegativeInteger(value.balance_ap_milli) && typeof value.checkin_completed === "boolean" &&
    nonNegativeInteger(value.daily_target_minutes) && boundedRatio(value.target_progress) &&
    nonNegativeInteger(value.streak_days) &&
    (lastEvent === undefined || (record(lastEvent) && nonNegativeInteger(lastEvent.id) &&
      boundedText(lastEvent.type, 64) && boundedText(lastEvent.message, 512) && boundedText(lastEvent.created_at, 128)));
}

function validAISettings(value: unknown): value is NativeAISettings {
  if (!record(value) || typeof value.enabled !== "boolean" || !boundedRatio(value.min_confidence)) return false;
  const validEndpoint = (endpoint: unknown): endpoint is NativeAIEndpointSettings => record(endpoint) && typeof endpoint.enabled === "boolean" && boundedText(endpoint.provider, 64) && boundedText(endpoint.model, 128) && boundedText(endpoint.base_url, 1024) && typeof endpoint.api_key_configured === "boolean" && Number.isSafeInteger(endpoint.timeout_seconds) && Number(endpoint.timeout_seconds) >= 1 && Number(endpoint.timeout_seconds) <= 120 && ["auto", "json_object", "off"].includes(endpoint.json_mode as string);
  return validEndpoint(value.text) && validEndpoint(value.vision);
}

function validReminderSettings(value: unknown): value is NativeReminderSettings {
  return record(value) && Number.isSafeInteger(value.cooldown_minutes) && Number(value.cooldown_minutes) >= 1 && Number(value.cooldown_minutes) <= 1440 &&
    Array.isArray(value.quiet_periods) && value.quiet_periods.length <= 12 && value.quiet_periods.every(period => record(period) && boundedText(period.start, 5) && boundedText(period.end, 5));
}

function validTaskPresets(value: unknown): value is NativeTaskPresetList {
  if (!record(value) || !Array.isArray(value.pinned) || !Array.isArray(value.recent) || value.pinned.length > 8 || value.recent.length > 6) return false;
  const valid = (row: unknown): row is NativeTaskPreset => record(row) && boundedText(row.id, 128) && boundedText(row.name, 256) &&
    typeof row.pinned === "boolean" && Number.isSafeInteger(row.sort_order) && nonNegativeInteger(row.use_count) &&
    (row.last_used_at === undefined || row.last_used_at === null || boundedText(row.last_used_at, 128));
  return value.pinned.every(valid) && value.recent.every(valid);
}

function validHistory(value: unknown): value is NativeHistoryDay[] {
  return Array.isArray(value) && value.length <= 90 && value.every(row => record(row) &&
    boundedText(row.date, 32) && nonNegativeInteger(row.focus_minutes) && nonNegativeInteger(row.target_minutes) &&
    typeof row.checkin_completed === "boolean" && typeof row.target_completed === "boolean");
}

function validAchievements(value: unknown): value is NativeAchievement[] {
  return Array.isArray(value) && value.length <= 32 && value.every(row => record(row) &&
    boundedText(row.achievement_id, 64) && boundedText(row.name, 128) && boundedText(row.description, 512) &&
    boundedRatio(row.progress) && typeof row.unlocked === "boolean" &&
    (row.unlocked_at === undefined || boundedText(row.unlocked_at, 128)));
}

function validMissions(value: unknown): value is NativeMission[] {
  return Array.isArray(value) && value.length <= 100 && value.every(row => record(row) &&
    boundedText(row.id, 128) && boundedText(row.title, 256) && boundedText(row.description, 1024) &&
    nonNegativeInteger(row.reward_milli_ap) && ["OPEN", "COMPLETED", "CANCELLED"].includes(row.status as string) &&
    boundedText(row.created_at, 128) && (row.due_date === undefined || boundedText(row.due_date, 32)) &&
    (row.completed_at === undefined || boundedText(row.completed_at, 128)));
}

function validRewards(value: unknown): value is NativeReward[] {
  return Array.isArray(value) && value.length <= 100 && value.every(row => record(row) &&
    boundedText(row.id, 128) && boundedText(row.name, 256) && boundedText(row.type, 64) &&
    nonNegativeInteger(row.cost_milli_ap) && boundedText(row.description, 1024) && typeof row.enabled === "boolean");
}

function validAI(value: unknown): value is NativeAIStatus {
  return record(value) && typeof value.enabled === "boolean" && boundedText(value.text_provider, 64) &&
    typeof value.text_configured === "boolean" && typeof value.vision_enabled === "boolean" &&
    (value.text_model === undefined || boundedText(value.text_model, 128)) &&
    (value.warning === undefined || boundedText(value.warning, 512));
}

function boundedStringList(value: unknown, maxItems: number, maxLength: number): value is string[] {
  return Array.isArray(value) && value.length <= maxItems && value.every(item => boundedText(item, maxLength));
}

function normalizedReview(value: unknown): NativeReviewSummary | undefined {
  if (!record(value) || value.schema_version !== 1 || !boundedText(value.date, 32) || !boundedText(value.headline, 512) ||
    !boundedText(value.tomorrow_priority, 512) || !boundedStringList(value.unfinished, 32, 512) ||
    !boundedStringList(value.difficulties, 32, 512) || !boundedStringList(value.warnings, 16, 512) ||
    !Array.isArray(value.topics) || value.topics.length > 16 || !Array.isArray(value.accomplishments) || value.accomplishments.length > 32 ||
    !record(value.behavior) || !nonNegativeInteger(value.behavior.distraction_count) ||
    !nonNegativeInteger(value.behavior.largest_distraction_seconds) || !nonNegativeInteger(value.behavior.average_recovery_seconds)) return undefined;
  const topics = value.topics.map(topic => record(topic) && boundedText(topic.name, 128) && boundedText(topic.summary, 512) && boundedRatio(topic.confidence)
    ? { name: topic.name, summary: topic.summary, confidence: topic.confidence } : undefined);
  const accomplishments = value.accomplishments.map(item => record(item) && boundedText(item.text, 512) && boundedRatio(item.confidence)
    ? { text: item.text, confidence: item.confidence } : undefined);
  if (topics.some(topic => !topic) || accomplishments.some(item => !item)) return undefined;
  return {
    schema_version: 1,
    date: value.date,
    headline: value.headline,
    topics: topics as Array<{ name: string; summary: string; confidence: number }>,
    accomplishments: accomplishments as Array<{ text: string; confidence: number }>,
    unfinished: value.unfinished,
    difficulties: value.difficulties,
    behavior: {
      distraction_count: value.behavior.distraction_count,
      largest_distraction_seconds: value.behavior.largest_distraction_seconds,
      average_recovery_seconds: value.behavior.average_recovery_seconds,
    },
    tomorrow_priority: value.tomorrow_priority,
    warnings: value.warnings,
    status: ["PENDING", "READY", "STALE", "FAILED"].includes(value.status as string) ? value.status as NativeReviewSummary["status"] : "FAILED",
    generation_mode: ["AI", "FALLBACK", ""].includes(value.generation_mode as string) ? value.generation_mode as NativeReviewSummary["generation_mode"] : "",
    provider: boundedText(value.provider, 64) ? value.provider : "",
    model: boundedText(value.model, 128) ? value.model : "",
    revision: nonNegativeInteger(value.revision) ? value.revision : 0,
    attempt_count: nonNegativeInteger(value.attempt_count) ? value.attempt_count : 0,
    ...(boundedText(value.error_code, 64) ? { error_code: value.error_code } : {}),
    warnings_count: nonNegativeInteger(value.warnings_count) ? value.warnings_count : value.warnings.length,
  };
}

export function normalizeNativeDashboardSnapshot(raw: unknown): SupervisorDashboardSnapshot {
  if (!record(raw) || raw.connected !== true || !validStatus(raw.status)) {
    return { connected: false, last_error_kind: record(raw) ? errorKind(raw.last_error_kind) ?? "invalid_response" : "invalid_response" };
  }
  return {
    connected: true,
    status: raw.status,
    ...(validMotivation(raw.motivation) ? { motivation: raw.motivation } : {}),
    ...(validTaskPresets(raw.task_presets) ? { task_presets: raw.task_presets } : {}),
    ...(validReminderSettings(raw.reminder_settings) ? { reminder_settings: raw.reminder_settings } : {}),
    ...(validAISettings(raw.ai_settings) ? { ai_settings: raw.ai_settings } : {}),
    ...(validHistory(raw.history) ? { history: raw.history } : {}),
    ...(validAchievements(raw.achievements) ? { achievements: raw.achievements } : {}),
    ...(validMissions(raw.missions) ? { missions: raw.missions } : {}),
    ...(validRewards(raw.rewards) ? { rewards: raw.rewards } : {}),
    ...(validAI(raw.ai) ? { ai: raw.ai } : {}),
    ...(normalizedReview(raw.review) ? { review: normalizedReview(raw.review) } : {}),
  };
}

export class NativeSupervisorDashboardAdapter {
  async poll(): Promise<SupervisorDashboardSnapshot> {
    try {
      return normalizeNativeDashboardSnapshot(await invoke<unknown>("supervisor_dashboard_snapshot"));
    } catch (error) {
      return { connected: false, last_error_kind: classifyNativeError(error) };
    }
  }
}

export interface ControlResult {
  ok: boolean;
  error_kind?: ControlErrorKind;
}

const CONTROL_ERROR_KINDS: ControlErrorKind[] = ["timeout", "unauthorized", "unavailable", "invalid_response", "rejected"];

export function normalizeControlResult(raw: unknown): ControlResult {
  if (!raw || typeof raw !== "object") return { ok: false, error_kind: "invalid_response" };
  const value = raw as Record<string, unknown>;
  if (value.ok === true) return { ok: true };
  const kind = value.error_kind;
  return {
    ok: false,
    error_kind: typeof kind === "string" && CONTROL_ERROR_KINDS.includes(kind as ControlErrorKind)
      ? kind as ControlErrorKind
      : "invalid_response",
  };
}

function classifyControlError(error: unknown): ControlErrorKind {
  const message = error instanceof Error ? error.message : String(error ?? "");
  if (/401|403|unauthorized/i.test(message)) return "unauthorized";
  if (/400|409|rejected/i.test(message)) return "rejected";
  if (/timeout|timed out|deadline/i.test(message)) return "timeout";
  return "unavailable";
}

export interface SupervisorControlAdapter {
  setModeStudy(task: string): Promise<ControlResult>;
  setModeBreak(): Promise<ControlResult>;
  setModeOff(): Promise<ControlResult>;
  setTask(task: string): Promise<ControlResult>;
  createTaskPreset(name: string, pinned: boolean): Promise<ControlResult>;
  selectTaskPreset(id: string): Promise<ControlResult>;
  updateTaskPreset(id: string, name: string, pinned: boolean, sortOrder: number): Promise<ControlResult>;
  deleteTaskPreset(id: string): Promise<ControlResult>;
  setReminderSettings(cooldownMinutes: number, quietPeriods: Array<{ start: string; end: string }>): Promise<ControlResult>;
  saveAISettings(settings: NativeAISettings): Promise<ControlResult>;
  putAISecret(target: "text" | "vision", apiKey: string): Promise<ControlResult>;
  deleteAISecret(target: "text" | "vision"): Promise<ControlResult>;
  testAIConnection(target: "text" | "vision"): Promise<NativeAIConnectionResult>;
  generateReview(): Promise<ControlResult>;
  setDailyTarget(minutes: number): Promise<ControlResult>;
}

export type AutostartState = { enabled: boolean; available: boolean };

export class NativeSystemIntegrationAdapter {
  async getAutostartState(): Promise<AutostartState> {
    try {
      const value = await invoke<unknown>("get_autostart_state");
      if (!record(value) || typeof value.enabled !== "boolean" || typeof value.available !== "boolean") {
        return { enabled: false, available: false };
      }
      return { enabled: value.enabled, available: value.available };
    } catch {
      return { enabled: false, available: false };
    }
  }

  async setAutostartEnabled(enabled: boolean): Promise<AutostartState> {
    try {
      const value = await invoke<unknown>("set_autostart_enabled", { enabled });
      if (!record(value) || typeof value.enabled !== "boolean" || typeof value.available !== "boolean") {
        return { enabled: false, available: false };
      }
      return { enabled: value.enabled, available: value.available };
    } catch {
      return { enabled: false, available: false };
    }
  }
}

export class NativeSupervisorControlAdapter implements SupervisorControlAdapter {
  setModeStudy(task: string): Promise<ControlResult> {
    return this.call("STUDY", task);
  }

  setModeBreak(): Promise<ControlResult> {
    return this.call("BREAK");
  }

  setModeOff(): Promise<ControlResult> {
    return this.call("OFF");
  }

  setTask(task: string): Promise<ControlResult> {
    return this.invokeControl("supervisor_set_task", { task });
  }

  createTaskPreset(name: string, pinned: boolean): Promise<ControlResult> {
    return this.invokeControl("supervisor_create_task_preset", { name, pinned });
  }

  selectTaskPreset(id: string): Promise<ControlResult> {
    return this.invokeControl("supervisor_select_task_preset", { id });
  }

  updateTaskPreset(id: string, name: string, pinned: boolean, sortOrder: number): Promise<ControlResult> {
    return this.invokeControl("supervisor_update_task_preset", { id, name, pinned, sortOrder });
  }

  deleteTaskPreset(id: string): Promise<ControlResult> {
    return this.invokeControl("supervisor_delete_task_preset", { id });
  }

  setReminderSettings(cooldownMinutes: number, quietPeriods: Array<{ start: string; end: string }>): Promise<ControlResult> {
    return this.invokeControl("supervisor_set_reminder_settings", { cooldownMinutes, quietPeriods });
  }

  saveAISettings(settings: NativeAISettings): Promise<ControlResult> {
    return this.invokeControl("supervisor_save_ai_settings", { settings });
  }

  putAISecret(target: "text" | "vision", apiKey: string): Promise<ControlResult> {
    return this.invokeControl("supervisor_put_ai_secret", { target, apiKey });
  }

  deleteAISecret(target: "text" | "vision"): Promise<ControlResult> {
    return this.invokeControl("supervisor_delete_ai_secret", { target });
  }

  async testAIConnection(target: "text" | "vision"): Promise<NativeAIConnectionResult> {
    try {
      const raw = await invoke<unknown>("supervisor_test_ai_connection", { target });
      if (!record(raw) || typeof raw.ok !== "boolean" || !boundedText(raw.provider, 64) || !boundedText(raw.model, 128) || !nonNegativeInteger(raw.latency_ms) || (raw.error_kind !== undefined && raw.error_kind !== null && !boundedText(raw.error_kind, 64))) return { ok: false, provider: "", model: "", latency_ms: 0, error_kind: "invalid_response" };
      return { ok: raw.ok, provider: raw.provider, model: raw.model, latency_ms: raw.latency_ms, ...(typeof raw.error_kind === "string" ? { error_kind: raw.error_kind as NativeAIConnectionResult["error_kind"] } : {}) };
    } catch { return { ok: false, provider: "", model: "", latency_ms: 0, error_kind: "unavailable" }; }
  }

  generateReview(): Promise<ControlResult> {
    return this.invokeControl("supervisor_generate_review", {});
  }

  setDailyTarget(minutes: number): Promise<ControlResult> {
    if (!Number.isSafeInteger(minutes) || minutes < 1 || minutes > 1440) {
      return Promise.resolve({ ok: false, error_kind: "rejected" });
    }
    return this.callDailyTarget(minutes);
  }

  private async call(mode: "STUDY" | "BREAK" | "OFF", task?: string): Promise<ControlResult> {
    try {
      return normalizeControlResult(await invoke<unknown>("supervisor_set_mode", { mode, task: task ?? null }));
    } catch (error) {
      // Native errors are normalized before they cross into UI state.
      return { ok: false, error_kind: classifyControlError(error) };
    }
  }

  private async invokeControl(command: string, args: Record<string, unknown>): Promise<ControlResult> {
    try {
      return normalizeControlResult(await invoke<unknown>(command, args));
    } catch (error) {
      return { ok: false, error_kind: classifyControlError(error) };
    }
  }

  private async callDailyTarget(minutes: number): Promise<ControlResult> {
    try {
      return normalizeControlResult(await invoke<unknown>("supervisor_set_daily_target", { minutes }));
    } catch (error) {
      return { ok: false, error_kind: classifyControlError(error) };
    }
  }
}

export class SupervisorPollLoop {
  private readonly intervalMs: number;
  private timer: ReturnType<typeof setTimeout> | null = null;
  private running = false;
  private inFlight = false;

  constructor(private readonly adapter: SupervisorAdapter, intervalMs = 1800) {
    this.intervalMs = Math.max(1500, Math.min(2000, intervalMs));
  }

  start(onSnapshot: (snapshot: PetTransportSnapshot) => void): void {
    if (this.running) return;
    this.running = true;
    void this.tick(onSnapshot);
  }

  stop(): void {
    this.running = false;
    if (this.timer !== null) clearTimeout(this.timer);
    this.timer = null;
  }

  isInFlight(): boolean { return this.inFlight; }

  private async tick(onSnapshot: (snapshot: PetTransportSnapshot) => void): Promise<void> {
    if (!this.running || this.inFlight) return;
    this.inFlight = true;
    try {
      onSnapshot(await this.adapter.poll());
    } finally {
      this.inFlight = false;
      if (this.running) this.timer = setTimeout(() => void this.tick(onSnapshot), this.intervalMs);
    }
  }
}

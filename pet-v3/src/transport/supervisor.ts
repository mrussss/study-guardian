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
  fallback_models: string[];
  base_url: string;
  api_key_configured: boolean;
  timeout_seconds: number;
  json_mode: "auto" | "json_object" | "off";
}

export type AIProxyMode = "environment" | "direct" | "manual";

export interface NativeAIProxySettings {
  mode: AIProxyMode;
  url: string;
}

export interface NativeAISettings {
  enabled: boolean;
  min_confidence: number;
  proxy: NativeAIProxySettings;
  text: NativeAIEndpointSettings;
  vision: NativeAIEndpointSettings;
}

export type AIConnectionErrorKind = "authentication_failed" | "model_not_found" | "model_unavailable" | "model_rate_limited" | "account_rate_limited" | "timeout" | "network_unavailable" | "proxy_unreachable" | "tls_failed" | "invalid_response" | "invalid_output" | "provider_unavailable" | "storage_unavailable" | "unavailable" | "rate_limited";

export interface NativeAIConnectionResult {
  ok: boolean;
  provider: string;
  model: string;
  latency_ms: number;
  error_kind?: AIConnectionErrorKind;
}

export interface NativeAIProxyTestResult {
  ok: boolean;
  mode: AIProxyMode;
  latency_ms: number;
  error_kind?: AIConnectionErrorKind;
}

const AI_ERROR_KINDS: AIConnectionErrorKind[] = ["authentication_failed", "model_not_found", "model_unavailable", "model_rate_limited", "account_rate_limited", "timeout", "network_unavailable", "proxy_unreachable", "tls_failed", "invalid_response", "invalid_output", "provider_unavailable", "storage_unavailable", "unavailable", "rate_limited"];

function aiErrorKind(value: unknown): AIConnectionErrorKind | undefined {
  return typeof value === "string" && AI_ERROR_KINDS.includes(value as AIConnectionErrorKind) ? value as AIConnectionErrorKind : undefined;
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
  linked_task_preset_id?: string;
  linked_task_name?: string;
  link_source?: "MANUAL" | "RULE" | "AI";
  link_confidence?: number;
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

function validReviewDate(value: string): boolean {
  if (!/^\d{4}-\d{2}-\d{2}$/.test(value)) return false;
  const [year, month, day] = value.split("-").map(Number);
  const parsed = new Date(Date.UTC(year, month - 1, day));
  return parsed.getUTCFullYear() === year && parsed.getUTCMonth() === month - 1 && parsed.getUTCDate() === day;
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

function validAIProxy(value: unknown): value is NativeAIProxySettings {
  return record(value) && ["environment", "direct", "manual"].includes(value.mode as string) && boundedText(value.url, 2048) && (value.mode === "manual" || value.url === "");
}

function normalizedAISettings(value: unknown): NativeAISettings | undefined {
  if (!record(value) || typeof value.enabled !== "boolean" || !boundedRatio(value.min_confidence)) return undefined;
  const validEndpoint = (endpoint: unknown): endpoint is NativeAIEndpointSettings => record(endpoint) && typeof endpoint.enabled === "boolean" && boundedText(endpoint.provider, 64) && boundedText(endpoint.model, 128) && Array.isArray(endpoint.fallback_models) && endpoint.fallback_models.length <= 3 && endpoint.fallback_models.every(model => boundedText(model, 128) && model.trim() !== "") && boundedText(endpoint.base_url, 1024) && typeof endpoint.api_key_configured === "boolean" && Number.isSafeInteger(endpoint.timeout_seconds) && Number(endpoint.timeout_seconds) >= 1 && Number(endpoint.timeout_seconds) <= 120 && ["auto", "json_object", "off"].includes(endpoint.json_mode as string);
  if (!validEndpoint(value.text) || !validEndpoint(value.vision)) return undefined;
  const proxy = value.proxy === undefined ? { mode: "environment" as const, url: "" } : value.proxy;
  if (!validAIProxy(proxy)) return undefined;
  return { enabled: value.enabled, min_confidence: value.min_confidence, proxy, text: value.text, vision: value.vision };
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
    (row.completed_at === undefined || boundedText(row.completed_at, 128)) &&
    (row.linked_task_preset_id === undefined || boundedText(row.linked_task_preset_id, 128)) &&
    (row.linked_task_name === undefined || boundedText(row.linked_task_name, 256)) &&
    (row.link_source === undefined || ["MANUAL", "RULE", "AI"].includes(row.link_source as string)) &&
    (row.link_confidence === undefined || boundedRatio(row.link_confidence)));
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

function optionalBoundedStringList(value: unknown, maxItems: number, maxLength: number): string[] | undefined {
  if (value === undefined || value === null) return [];
  return boundedStringList(value, maxItems, maxLength) ? value : undefined;
}

function optionalArray(value: unknown): unknown[] | undefined {
  if (value === undefined || value === null) return [];
  return Array.isArray(value) ? value : undefined;
}

function normalizedReview(value: unknown): NativeReviewSummary | undefined {
  if (!record(value) || value.schema_version !== 1 || !boundedText(value.date, 32) || !boundedText(value.headline, 512) ||
    !boundedText(value.tomorrow_priority, 512) ||
    !record(value.behavior) || !nonNegativeInteger(value.behavior.distraction_count) ||
    !nonNegativeInteger(value.behavior.largest_distraction_seconds) || !nonNegativeInteger(value.behavior.average_recovery_seconds)) return undefined;
  const unfinished = optionalBoundedStringList(value.unfinished, 32, 512);
  const difficulties = optionalBoundedStringList(value.difficulties, 32, 512);
  const warnings = optionalBoundedStringList(value.warnings, 16, 512);
  const rawTopics = optionalArray(value.topics);
  const rawAccomplishments = optionalArray(value.accomplishments);
  if (!unfinished || !difficulties || !warnings || !rawTopics || rawTopics.length > 16 || !rawAccomplishments || rawAccomplishments.length > 32) return undefined;
  const topics = rawTopics.map(topic => record(topic) && boundedText(topic.name, 128) && boundedText(topic.summary, 512) && boundedRatio(topic.confidence)
    ? { name: topic.name, summary: topic.summary, confidence: topic.confidence } : undefined);
  const accomplishments = rawAccomplishments.map(item => record(item) && boundedText(item.text, 512) && boundedRatio(item.confidence)
    ? { text: item.text, confidence: item.confidence } : undefined);
  if (topics.some(topic => !topic) || accomplishments.some(item => !item)) return undefined;
  return {
    schema_version: 1,
    date: value.date,
    headline: value.headline,
    topics: topics as Array<{ name: string; summary: string; confidence: number }>,
    accomplishments: accomplishments as Array<{ text: string; confidence: number }>,
    unfinished,
    difficulties,
    behavior: {
      distraction_count: value.behavior.distraction_count,
      largest_distraction_seconds: value.behavior.largest_distraction_seconds,
      average_recovery_seconds: value.behavior.average_recovery_seconds,
    },
    tomorrow_priority: value.tomorrow_priority,
    warnings,
    status: ["PENDING", "READY", "STALE", "FAILED"].includes(value.status as string) ? value.status as NativeReviewSummary["status"] : "FAILED",
    generation_mode: ["AI", "FALLBACK", ""].includes(value.generation_mode as string) ? value.generation_mode as NativeReviewSummary["generation_mode"] : "",
    provider: boundedText(value.provider, 64) ? value.provider : "",
    model: boundedText(value.model, 128) ? value.model : "",
    revision: nonNegativeInteger(value.revision) ? value.revision : 0,
    attempt_count: nonNegativeInteger(value.attempt_count) ? value.attempt_count : 0,
    ...(boundedText(value.error_code, 64) ? { error_code: value.error_code } : {}),
    warnings_count: nonNegativeInteger(value.warnings_count) ? value.warnings_count : warnings.length,
  };
}

export function normalizeNativeDashboardSnapshot(raw: unknown): SupervisorDashboardSnapshot {
  if (!record(raw) || raw.connected !== true || !validStatus(raw.status)) {
    return { connected: false, last_error_kind: record(raw) ? errorKind(raw.last_error_kind) ?? "invalid_response" : "invalid_response" };
  }
  const aiSettings = normalizedAISettings(raw.ai_settings);
  return {
    connected: true,
    status: raw.status,
    ...(validMotivation(raw.motivation) ? { motivation: raw.motivation } : {}),
    ...(validTaskPresets(raw.task_presets) ? { task_presets: raw.task_presets } : {}),
    ...(validReminderSettings(raw.reminder_settings) ? { reminder_settings: raw.reminder_settings } : {}),
    ...(aiSettings ? { ai_settings: aiSettings } : {}),
    ...(validHistory(raw.history) ? { history: raw.history } : {}),
    ...(validAchievements(raw.achievements) ? { achievements: raw.achievements } : {}),
    ...(validMissions(raw.missions) ? { missions: raw.missions } : {}),
    ...(validRewards(raw.rewards) ? { rewards: raw.rewards } : {}),
    ...(validAI(raw.ai) ? { ai: raw.ai } : {}),
    ...(normalizedReview(raw.review) ? { review: normalizedReview(raw.review) } : {}),
  };
}

export interface SupervisorDashboardAdapter {
  poll(): Promise<SupervisorDashboardSnapshot>;
}

export class NativeSupervisorDashboardAdapter implements SupervisorDashboardAdapter {
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
  task?: string;
}

export type ReviewGenerationState = "IDLE" | "PENDING" | "READY" | "FAILED";
export type ReviewGenerationStatus = "READY" | "STALE" | "FAILED";
export type ReviewGenerationMode = "AI" | "FALLBACK" | "";
export type ReviewGenerationErrorKind = ControlErrorKind | "provider_not_configured" | "compaction_failed" | "input_hash_failed" | "sanitizer_failed" | "validation_failed" | "network" | "http" | "invalid_json" | "schema_invalid" | "unsupported_version" | "not_configured" | "storage_unavailable" | "canceled";

export interface ReviewGenerationStatusSnapshot {
  accepted?: boolean;
  already_running?: boolean;
  generation_id?: string;
  date?: string;
  state: ReviewGenerationState;
  generation_mode?: Exclude<ReviewGenerationMode, "">;
  error_kind?: ReviewGenerationErrorKind;
  revision?: number;
}

export interface ReviewGenerationResult {
  ok: boolean;
  status?: ReviewGenerationStatus;
  generation_mode?: ReviewGenerationMode;
  error_kind?: ReviewGenerationErrorKind;
}

const CONTROL_ERROR_KINDS: ControlErrorKind[] = ["timeout", "unauthorized", "unavailable", "invalid_response", "rejected"];

export function normalizeControlResult(raw: unknown): ControlResult {
  if (!raw || typeof raw !== "object") return { ok: false, error_kind: "invalid_response" };
  const value = raw as Record<string, unknown>;
  if (value.ok === true) return { ok: true, ...(boundedText(value.task, 4096) ? { task: value.task } : {}) };
  const kind = value.error_kind;
  return {
    ok: false,
    error_kind: typeof kind === "string" && CONTROL_ERROR_KINDS.includes(kind as ControlErrorKind)
      ? kind as ControlErrorKind
      : "invalid_response",
  };
}

const REVIEW_GENERATION_ERROR_KINDS: ReviewGenerationErrorKind[] = ["timeout", "unauthorized", "unavailable", "invalid_response", "rejected", "provider_not_configured", "compaction_failed", "input_hash_failed", "sanitizer_failed", "validation_failed", "network", "http", "invalid_json", "schema_invalid", "unsupported_version", "not_configured", "storage_unavailable", "canceled"];

function normalizeReviewGenerationError(value: unknown): ReviewGenerationErrorKind | undefined {
  return typeof value === "string" && REVIEW_GENERATION_ERROR_KINDS.includes(value as ReviewGenerationErrorKind)
    ? value as ReviewGenerationErrorKind
    : undefined;
}

export function normalizeReviewGenerationStatus(raw: unknown): ReviewGenerationStatusSnapshot {
  if (!record(raw) || !["IDLE", "PENDING", "READY", "FAILED"].includes(raw.state as string)) {
    return { state: "FAILED", error_kind: "invalid_response" };
  }
  const state = raw.state as ReviewGenerationState;
  if (raw.date !== undefined && (!boundedText(raw.date, 32) || !validReviewDate(raw.date))) {
    return { state: "FAILED", error_kind: "invalid_response" };
  }
  if (raw.generation_id !== undefined && (!boundedText(raw.generation_id, 128) || raw.generation_id.trim() === "")) {
    return { state: "FAILED", error_kind: "invalid_response" };
  }
  if (raw.accepted !== undefined && typeof raw.accepted !== "boolean") return { state: "FAILED", error_kind: "invalid_response" };
  if (raw.already_running !== undefined && typeof raw.already_running !== "boolean") return { state: "FAILED", error_kind: "invalid_response" };
  const generationMode = raw.generation_mode === undefined || raw.generation_mode === null ? undefined : raw.generation_mode === "AI" || raw.generation_mode === "FALLBACK" ? raw.generation_mode : null;
  if (generationMode === null) return { state: "FAILED", error_kind: "invalid_response" };
  const errorKind = normalizeReviewGenerationError(raw.error_kind);
  if (raw.error_kind !== undefined && raw.error_kind !== null && !errorKind) return { state: "FAILED", error_kind: "invalid_response" };
  if (raw.revision !== undefined && !nonNegativeInteger(raw.revision)) return { state: "FAILED", error_kind: "invalid_response" };
  return {
    state,
    ...(typeof raw.accepted === "boolean" ? { accepted: raw.accepted } : {}),
    ...(typeof raw.already_running === "boolean" ? { already_running: raw.already_running } : {}),
    ...(typeof raw.generation_id === "string" ? { generation_id: raw.generation_id } : {}),
    ...(typeof raw.date === "string" ? { date: raw.date } : {}),
    ...(generationMode ? { generation_mode: generationMode } : {}),
    ...(errorKind ? { error_kind: errorKind } : {}),
    ...(nonNegativeInteger(raw.revision) ? { revision: raw.revision } : {}),
  };
}

export function normalizeReviewGenerationResult(raw: unknown): ReviewGenerationResult {
  if (!record(raw) || typeof raw.ok !== "boolean") return { ok: false, error_kind: "invalid_response" };
  const errorKind = raw.error_kind === undefined || raw.error_kind === null ? undefined : REVIEW_GENERATION_ERROR_KINDS.includes(raw.error_kind as ReviewGenerationErrorKind) ? raw.error_kind as ReviewGenerationErrorKind : undefined;
  if (raw.error_kind !== undefined && raw.error_kind !== null && !errorKind) return { ok: false, error_kind: "invalid_response" };
  if (!raw.ok) return { ok: false, ...(errorKind ? { error_kind: errorKind } : { error_kind: "invalid_response" }) };
  if (!["READY", "STALE", "FAILED"].includes(raw.status as string) || !["AI", "FALLBACK", ""].includes(raw.generation_mode as string)) return { ok: false, error_kind: "invalid_response" };
  return { ok: true, status: raw.status as ReviewGenerationStatus, generation_mode: raw.generation_mode as ReviewGenerationMode, ...(errorKind ? { error_kind: errorKind } : {}) };
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
  testAIProxy(): Promise<NativeAIProxyTestResult>;
  startReviewGeneration(): Promise<ReviewGenerationStatusSnapshot>;
  getReviewGenerationStatus(): Promise<ReviewGenerationStatusSnapshot>;
  generateReview(): Promise<ReviewGenerationResult>;
  setDailyTarget(minutes: number): Promise<ControlResult>;
  createMission(title: string, description: string, dueDate?: string, linkedTaskName?: string, linkedTaskPresetId?: string): Promise<ControlResult>;
  completeMission(id: string): Promise<ControlResult>;
  cancelMission(id: string): Promise<ControlResult>;
}

export type AutostartState = { enabled: boolean; available: boolean };

export interface SystemIntegrationAdapter {
  getAutostartState(): Promise<AutostartState>;
  setAutostartEnabled(enabled: boolean): Promise<AutostartState>;
}

export class NativeSystemIntegrationAdapter implements SystemIntegrationAdapter {
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
      const errorKind = record(raw) ? aiErrorKind(raw.error_kind) : undefined;
      if (!record(raw) || typeof raw.ok !== "boolean" || !boundedText(raw.provider, 64) || !boundedText(raw.model, 128) || !nonNegativeInteger(raw.latency_ms) || (raw.error_kind !== undefined && raw.error_kind !== null && !errorKind)) return { ok: false, provider: "", model: "", latency_ms: 0, error_kind: "invalid_response" };
      return { ok: raw.ok, provider: raw.provider, model: raw.model, latency_ms: raw.latency_ms, ...(errorKind ? { error_kind: errorKind } : {}) };
    } catch { return { ok: false, provider: "", model: "", latency_ms: 0, error_kind: "unavailable" }; }
  }

  async testAIProxy(): Promise<NativeAIProxyTestResult> {
    try {
      const raw = await invoke<unknown>("supervisor_test_ai_proxy");
      const errorKind = record(raw) ? aiErrorKind(raw.error_kind) : undefined;
      if (!record(raw) || typeof raw.ok !== "boolean" || !["environment", "direct", "manual"].includes(raw.mode as string) || !nonNegativeInteger(raw.latency_ms) || (raw.error_kind !== undefined && raw.error_kind !== null && !errorKind)) return { ok: false, mode: "environment", latency_ms: 0, error_kind: "invalid_response" };
      return { ok: raw.ok, mode: raw.mode as AIProxyMode, latency_ms: raw.latency_ms, ...(errorKind ? { error_kind: errorKind } : {}) };
    } catch { return { ok: false, mode: "environment", latency_ms: 0, error_kind: "unavailable" }; }
  }

  async generateReview(): Promise<ReviewGenerationResult> {
    try {
      return normalizeReviewGenerationResult(await invoke<unknown>("supervisor_generate_review"));
    } catch (error) {
      return { ok: false, error_kind: classifyControlError(error) };
    }
  }

  async startReviewGeneration(): Promise<ReviewGenerationStatusSnapshot> {
    try {
      return normalizeReviewGenerationStatus(await invoke<unknown>("supervisor_start_review_generation"));
    } catch (error) {
      return { state: "FAILED", error_kind: classifyControlError(error) };
    }
  }

  async getReviewGenerationStatus(): Promise<ReviewGenerationStatusSnapshot> {
    try {
      return normalizeReviewGenerationStatus(await invoke<unknown>("supervisor_review_generation_status"));
    } catch (error) {
      return { state: "FAILED", error_kind: classifyControlError(error) };
    }
  }

  setDailyTarget(minutes: number): Promise<ControlResult> {
    if (!Number.isSafeInteger(minutes) || minutes < 1 || minutes > 1440) {
      return Promise.resolve({ ok: false, error_kind: "rejected" });
    }
    return this.callDailyTarget(minutes);
  }

  createMission(title: string, description: string, dueDate?: string, linkedTaskName?: string, linkedTaskPresetId?: string): Promise<ControlResult> {
    return this.invokeControl("supervisor_create_mission", { title, description, dueDate: dueDate ?? null, linkedTaskName: linkedTaskName ?? null, linkedTaskPresetId: linkedTaskPresetId ?? null });
  }

  completeMission(id: string): Promise<ControlResult> {
    return this.invokeControl("supervisor_complete_mission", { id });
  }

  cancelMission(id: string): Promise<ControlResult> {
    return this.invokeControl("supervisor_cancel_mission", { id });
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

export interface SupervisorDashboardPollScheduler {
  setTimeout(callback: () => void, delayMs: number): ReturnType<typeof setTimeout>;
  clearTimeout(timer: ReturnType<typeof setTimeout>): void;
}

const defaultDashboardPollScheduler: SupervisorDashboardPollScheduler = {
  setTimeout: (callback, delayMs) => setTimeout(callback, delayMs),
  clearTimeout: timer => clearTimeout(timer),
};

/** Dashboard polling with queued refreshes and stale-snapshot protection. */
export class SupervisorDashboardPollLoop {
  private readonly intervalMs: number;
  private timer: ReturnType<typeof setTimeout> | null = null;
  private inFlight: Promise<void> | null = null;
  private inFlightRevision: number | null = null;
  private inFlightKind: "poll" | "refresh" | null = null;
  private running = false;
  private refreshQueued = false;
  private refreshWaiters: Array<() => void> = [];
  private mutationRevision = 0;
  private lifecycleRevision = 0;
  private onSnapshot: ((snapshot: SupervisorDashboardSnapshot) => void) | undefined;

  constructor(private readonly adapter: Pick<NativeSupervisorDashboardAdapter, "poll">, intervalMs = 2500,
    private readonly scheduler: SupervisorDashboardPollScheduler = defaultDashboardPollScheduler) {
    this.intervalMs = Math.max(1000, intervalMs);
  }

  start(onSnapshot: (snapshot: SupervisorDashboardSnapshot) => void): void {
    if (this.running) return;
    this.running = true;
    this.onSnapshot = onSnapshot;
    const lifecycleRevision = ++this.lifecycleRevision;
    void this.request("poll", lifecycleRevision);
  }

  stop(): void {
    this.running = false;
    this.lifecycleRevision += 1;
    this.inFlight = null;
    this.inFlightRevision = null;
    this.inFlightKind = null;
    this.refreshQueued = false;
    if (this.timer !== null) this.scheduler.clearTimeout(this.timer);
    this.timer = null;
    this.resolveRefreshWaiters();
  }

  markMutation(): void {
    this.mutationRevision += 1;
  }

  refresh(): Promise<void> {
    if (!this.running) return Promise.resolve();
    const promise = new Promise<void>(resolve => { this.refreshWaiters.push(resolve); });
    if (this.inFlight !== null) {
      if (this.inFlightKind === "poll" || this.inFlightRevision !== this.mutationRevision) this.refreshQueued = true;
      return promise;
    }
    if (this.timer !== null) this.scheduler.clearTimeout(this.timer);
    this.timer = null;
    void this.request("refresh", this.lifecycleRevision);
    return promise;
  }

  isInFlight(): boolean { return this.inFlight !== null; }

  private request(kind: "poll" | "refresh", lifecycleRevision: number): Promise<void> {
    if (!this.running || lifecycleRevision !== this.lifecycleRevision) return Promise.resolve();
    if (this.inFlight !== null) return this.inFlight;
    const requestRevision = this.mutationRevision;
    this.inFlightRevision = requestRevision;
    this.inFlightKind = kind;
    const work = Promise.resolve()
      .then(() => this.adapter.poll())
      .then(snapshot => {
        if (this.running && lifecycleRevision === this.lifecycleRevision && requestRevision === this.mutationRevision) this.onSnapshot?.(snapshot);
      })
      .catch(() => { /* adapters normalize transport errors */ })
      .finally(() => {
        if (lifecycleRevision !== this.lifecycleRevision) return;
        this.inFlight = null;
        if (!this.running) {
          this.resolveRefreshWaiters();
        } else if (this.refreshQueued) {
          this.refreshQueued = false;
          this.inFlightRevision = null;
          this.inFlightKind = null;
          void this.request("refresh", lifecycleRevision);
        } else {
          this.inFlightRevision = null;
          this.inFlightKind = null;
          this.resolveRefreshWaiters();
          this.timer = this.scheduler.setTimeout(() => {
            this.timer = null;
            if (this.running && lifecycleRevision === this.lifecycleRevision) void this.request("poll", lifecycleRevision);
          }, this.intervalMs);
        }
      });
    this.inFlight = work;
    return work;
  }

  private resolveRefreshWaiters(): void {
    const waiters = this.refreshWaiters.splice(0);
    waiters.forEach(resolve => resolve());
  }
}

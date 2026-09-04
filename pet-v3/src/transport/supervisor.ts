import { invoke } from "@tauri-apps/api/core";
import { mockSemantic, mockSupervisorOffline } from "../mock/semantic";
import { isCurrentActivityView, type CurrentActivityView } from "../model/semantic";

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

  private async call(mode: "STUDY" | "BREAK" | "OFF", task?: string): Promise<ControlResult> {
    try {
      return normalizeControlResult(await invoke<unknown>("supervisor_set_mode", { mode, task: task ?? null }));
    } catch (error) {
      // Native errors are normalized before they cross into UI state.
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

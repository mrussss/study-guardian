import { invoke } from "@tauri-apps/api/core";
import { mockSemantic, mockSupervisorOffline } from "../mock/semantic";
import { isCurrentActivityView, type CurrentActivityView } from "../model/semantic";

export type TransportErrorKind = "timeout" | "unauthorized" | "unavailable" | "invalid_response";

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

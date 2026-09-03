import type { Activity, CurrentActivityView } from "../model/semantic";

export type VisualState =
  | "IDLE"
  | "CODING"
  | "ALGORITHM"
  | "READING"
  | "WRITING"
  | "WATCHING"
  | "LEARNING"
  | "DISTRACTED"
  | "RESTING"
  | "OFFLINE"
  | "THINKING"
  | "CELEBRATE"
  | "TALKING";

export type PetEvent = { kind: "celebrate" | "reminder"; critical?: boolean };

export interface BehaviorInput {
  semantic: CurrentActivityView;
  event?: PetEvent;
  nowMs: number;
}

export interface BehaviorTiming {
  normalHysteresisMs: number;
  distractedHysteresisMs: number;
  thinkingStableMs: number;
}

export const DEFAULT_BEHAVIOR_TIMING: BehaviorTiming = {
  normalHysteresisMs: 5000,
  distractedHysteresisMs: 350,
  thinkingStableMs: 4500,
};

export function targetState(input: BehaviorInput): VisualState {
  const { semantic, event } = input;
  if (event?.kind === "celebrate" || event?.critical) return "CELEBRATE";
  if (event?.kind === "reminder") return "TALKING";
  if (!semantic.fresh) return "OFFLINE";
  if (semantic.privacy === "SENSITIVE") return "OFFLINE";
  if (semantic.user_mode === "BREAK") return "RESTING";
  if (semantic.user_mode === "OFF" || semantic.user_mode === "STANDBY") return "IDLE";
  if (semantic.user_mode !== "STUDY") return "IDLE";
  if (semantic.relation === "DISTRACTED") return "DISTRACTED";

  const activityMap: Partial<Record<Activity, VisualState>> = {
    CODING: "CODING",
    ALGORITHM: "ALGORITHM",
    READING: "READING",
    WRITING: "WRITING",
    WATCHING: "WATCHING",
    AI_ASSISTED: "LEARNING",
    GENERAL_STUDY: "LEARNING",
    BROWSING: semantic.relation === "FOCUSED" ? "LEARNING" : "IDLE",
  };
  const mapped = activityMap[semantic.activity];
  if (mapped) return mapped;

  // THINKING is a UI-only derivation. It is never sent back as semantic
  // activity and is available only after a stable focused static idle.
  if (semantic.relation === "FOCUSED" && semantic.interaction === "IDLE_STATIC") return "THINKING";
  return "IDLE";
}

export class BehaviorEngine {
  private state: VisualState = "IDLE";
  private pending: VisualState | null = null;
  private pendingSince = 0;
  private readonly timing: BehaviorTiming;

  constructor(timing: Partial<BehaviorTiming> = {}) {
    this.timing = { ...DEFAULT_BEHAVIOR_TIMING, ...timing };
  }

  update(input: BehaviorInput): VisualState {
    const next = targetState(input);
    if (next === this.state) {
      this.pending = null;
      return this.state;
    }
    const immediate = next === "CELEBRATE" || next === "TALKING" || next === "OFFLINE" || next === "RESTING";
    if (immediate) return this.commit(next);
    if (next === "DISTRACTED") {
      if (this.pending !== next) {
        this.pending = next;
        this.pendingSince = input.nowMs;
      }
      if (input.nowMs - this.pendingSince >= this.timing.distractedHysteresisMs) return this.commit(next);
      return this.state;
    }
    if (this.pending !== next) {
      this.pending = next;
      this.pendingSince = input.nowMs;
      return this.state;
    }
    const stableFor = next === "THINKING" ? this.timing.thinkingStableMs : this.timing.normalHysteresisMs;
    if (input.nowMs - this.pendingSince >= stableFor) return this.commit(next);
    return this.state;
  }

  current(): VisualState { return this.state; }

  private commit(next: VisualState): VisualState {
    this.state = next;
    this.pending = null;
    this.pendingSince = 0;
    return next;
  }
}

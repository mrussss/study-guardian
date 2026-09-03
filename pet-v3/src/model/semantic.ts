export const SEMANTIC_SCHEMA_VERSION = 1;

export type UserMode = "STANDBY" | "STUDY" | "BREAK" | "OFF";
export type Interaction = "ACTIVE" | "IDLE_STATIC" | "IDLE_DYNAMIC" | "UNKNOWN";
export type Relation = "FOCUSED" | "DISTRACTED" | "UNKNOWN";
export type Privacy = "NORMAL" | "SENSITIVE";
export type Activity =
  | "CODING"
  | "ALGORITHM"
  | "READING"
  | "WRITING"
  | "WATCHING"
  | "AI_ASSISTED"
  | "BROWSING"
  | "GENERAL_STUDY"
  | "UNKNOWN";

// This is intentionally identical to Supervisor's CurrentActivityView. P1
// uses a local mock; no token or real Supervisor HTTP client is present.
export interface CurrentActivityView {
  schema_version: number;
  observed_at: string;
  fresh: boolean;
  user_mode: UserMode;
  task: string;
  interaction: Interaction;
  relation: Relation;
  privacy: Privacy;
  activity: Activity;
  confidence: number;
}

export function isCurrentActivityView(value: unknown): value is CurrentActivityView {
  if (!value || typeof value !== "object") return false;
  const v = value as Record<string, unknown>;
  return v.schema_version === SEMANTIC_SCHEMA_VERSION &&
    typeof v.observed_at === "string" && typeof v.fresh === "boolean" &&
    typeof v.user_mode === "string" && typeof v.task === "string" &&
    typeof v.interaction === "string" && typeof v.relation === "string" &&
    typeof v.privacy === "string" && typeof v.activity === "string" &&
    typeof v.confidence === "number";
}

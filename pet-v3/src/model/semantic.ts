export const SEMANTIC_SCHEMA_VERSION = 1;

export const VALID_USER_MODES = ["STANDBY", "STUDY", "BREAK", "OFF"] as const;
export const VALID_INTERACTIONS = ["ACTIVE", "IDLE_STATIC", "IDLE_DYNAMIC", "UNKNOWN"] as const;
export const VALID_RELATIONS = ["FOCUSED", "DISTRACTED", "UNKNOWN"] as const;
export const VALID_PRIVACY = ["NORMAL", "SENSITIVE"] as const;
export const VALID_ACTIVITIES = [
  "CODING", "ALGORITHM", "READING", "WRITING", "WATCHING", "AI_ASSISTED",
  "BROWSING", "GENERAL_STUDY", "UNKNOWN",
] as const;

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
  schema_version: 1;
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
    typeof v.observed_at === "string" && v.observed_at.length > 0 &&
    Number.isFinite(Date.parse(v.observed_at)) &&
    typeof v.fresh === "boolean" &&
    typeof v.user_mode === "string" && VALID_USER_MODES.includes(v.user_mode as UserMode) &&
    typeof v.task === "string" &&
    typeof v.interaction === "string" && VALID_INTERACTIONS.includes(v.interaction as Interaction) &&
    typeof v.relation === "string" && VALID_RELATIONS.includes(v.relation as Relation) &&
    typeof v.privacy === "string" && VALID_PRIVACY.includes(v.privacy as Privacy) &&
    typeof v.activity === "string" && VALID_ACTIVITIES.includes(v.activity as Activity) &&
    typeof v.confidence === "number" && Number.isFinite(v.confidence) &&
    v.confidence >= 0 && v.confidence <= 1;
}

import type { Activity, CurrentActivityView, Interaction, Privacy, Relation, UserMode, ProgressSignal, SourceKind } from "../model/semantic";
import { SEMANTIC_SCHEMA_VERSION } from "../model/semantic";

export interface MockSemanticOverrides {
  user_mode?: UserMode;
  task?: string;
  interaction?: Interaction;
  relation?: Relation;
  activity?: Activity;
  privacy?: Privacy;
  fresh?: boolean;
  confidence?: number;
  topic?: string;
  subtopic?: string;
  action?: string;
  progress_signal?: ProgressSignal;
  source_kind?: SourceKind;
}

export function mockSemantic(overrides: MockSemanticOverrides = {}, observedAt = new Date(0).toISOString()): CurrentActivityView {
  return {
    schema_version: SEMANTIC_SCHEMA_VERSION,
    observed_at: observedAt,
    fresh: overrides.fresh ?? true,
    user_mode: overrides.user_mode ?? "STUDY",
    task: overrides.task ?? "P1 mock study",
    interaction: overrides.interaction ?? "ACTIVE",
    relation: overrides.relation ?? "FOCUSED",
    privacy: overrides.privacy ?? "NORMAL",
    activity: overrides.activity ?? "GENERAL_STUDY",
    topic: overrides.topic ?? "",
    subtopic: overrides.subtopic ?? "",
    action: overrides.action ?? "",
    progress_signal: overrides.progress_signal ?? "OBSERVING",
    source_kind: overrides.source_kind ?? "LOCAL_RULE",
    confidence: overrides.confidence ?? 0.8,
  };
}

export interface MockSupervisorConnection {
  connected: boolean;
  semantic: CurrentActivityView;
}

export function mockSupervisorOffline(): MockSupervisorConnection {
  return { connected: false, semantic: mockSemantic({ fresh: false }) };
}

export function mockActivityWatchStale(): MockSupervisorConnection {
  return { connected: true, semantic: mockSemantic({ fresh: false, activity: "UNKNOWN", confidence: 0 }) };
}

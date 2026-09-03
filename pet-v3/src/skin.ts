import type { VisualState } from "./behavior/engine";

export interface SkinManifestV1 {
  schema_version: 1;
  id: string;
  name: string;
  author?: string;
  license: string;
  frame_size: number;
  display_size: number;
  fps: number;
  pixel_art: boolean;
  states: Record<string, string>;
}

export type SkinManifest = SkinManifestV1;

const visualToLegacyState: Record<VisualState, string> = {
  IDLE: "idle",
  CODING: "coding",
  ALGORITHM: "algorithm",
  READING: "reading",
  WRITING: "writing",
  WATCHING: "watching",
  LEARNING: "learning",
  THINKING: "thinking",
  DISTRACTED: "distracted",
  RESTING: "rest",
  TALKING: "talk",
  CELEBRATE: "celebrate",
  OFFLINE: "idle",
};

const fallbackChain: Record<string, string[]> = {
  coding: ["coding", "study", "idle"],
  algorithm: ["algorithm", "study", "idle"],
  reading: ["reading", "study", "idle"],
  writing: ["writing", "study", "idle"],
  watching: ["watching", "study", "idle"],
  learning: ["learning", "study", "idle"],
  thinking: ["thinking", "study", "idle"],
  distracted: ["distracted", "idle"],
  rest: ["rest", "idle"],
  talk: ["talk", "idle"],
  celebrate: ["celebrate", "idle"],
  idle: ["idle"],
};

export function loadSkinManifest(raw: unknown): SkinManifestV1 {
  if (!raw || typeof raw !== "object") throw new Error("skin manifest must be an object");
  const value = raw as Partial<SkinManifestV1>;
  if (value.schema_version !== 1 || typeof value.id !== "string" || value.id.trim() === "" || typeof value.name !== "string" || value.name.trim() === "" || typeof value.license !== "string" || value.license.trim() === "" || typeof value.frame_size !== "number" || typeof value.display_size !== "number" || typeof value.fps !== "number" || !value.states || typeof value.states !== "object") {
    throw new Error("invalid legacy skin schema v1 manifest");
  }
  if (!Number.isFinite(value.frame_size) || value.frame_size <= 0 || !Number.isInteger(value.frame_size) || !Number.isFinite(value.display_size) || value.display_size <= 0 || !Number.isInteger(value.display_size) || !Number.isFinite(value.fps) || value.fps <= 0) {
    throw new Error("skin dimensions and fps must be positive finite values");
  }
  for (const [state, source] of Object.entries(value.states)) {
    if (state.trim() === "" || typeof source !== "string" || source.trim() === "") throw new Error("skin states must map names to non-empty asset paths");
  }
  if (!value.states.idle) throw new Error("skin v1 requires states.idle as hard fallback");
  return value as SkinManifestV1;
}

export function resolveState(manifest: SkinManifestV1, visualState: VisualState): { state: string; source: string } {
  const requested = visualToLegacyState[visualState];
  const candidates = fallbackChain[requested] ?? [requested, "idle"];
  for (const state of candidates) {
    const source = manifest.states[state];
    if (source) return { state, source };
  }
  // loadSkinManifest guarantees this branch is unreachable for valid v1
  // skins, but retaining it makes malformed runtime data fail soft.
  return { state: "idle", source: manifest.states.idle ?? "" };
}

export { visualToLegacyState };

import type { VisualState } from "./behavior/engine";

export interface SkinManifest {
  schema_version: 1;
  id: string;
  name: string;
  license: string;
  frame_size: { width: number; height: number };
  display_size: { width: number; height: number };
  fps: number;
  pixel_art: boolean;
  states: Partial<Record<VisualState, string>>;
}

const fallbackState: Partial<Record<VisualState, VisualState>> = {
  ALGORITHM: "CODING",
  LEARNING: "READING",
  THINKING: "IDLE",
  DISTRACTED: "IDLE",
  RESTING: "IDLE",
  OFFLINE: "IDLE",
  TALKING: "IDLE",
  CELEBRATE: "IDLE",
};

export function loadSkinManifest(raw: unknown): SkinManifest {
  if (!raw || typeof raw !== "object") throw new Error("skin manifest must be an object");
  const value = raw as Partial<SkinManifest>;
  if (value.schema_version !== 1 || !value.id || !value.name || !value.license || !value.frame_size || !value.display_size || !value.states) throw new Error("invalid schema v1 skin manifest");
  if (value.frame_size.width <= 0 || value.frame_size.height <= 0 || value.display_size.width <= 0 || value.display_size.height <= 0 || (value.fps ?? 0) <= 0) throw new Error("skin dimensions and fps must be positive");
  return value as SkinManifest;
}

export function resolveState(manifest: SkinManifest, state: VisualState): { state: VisualState; source: string } {
  if (manifest.states[state]) return { state, source: manifest.states[state]! };
  let fallback = fallbackState[state] ?? "IDLE";
  while (!manifest.states[fallback] && fallbackState[fallback]) fallback = fallbackState[fallback]!;
  return { state: fallback, source: manifest.states[fallback] ?? "" };
}

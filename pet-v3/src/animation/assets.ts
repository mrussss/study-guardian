import type { AnimationClip } from "./engine";
import { splitHorizontal } from "./sprite";
import { resolveState, type SkinManifestV1 } from "../skin";
import type { VisualState } from "../behavior/engine";

export interface LoadedAnimation {
  clip: AnimationClip;
  image: HTMLImageElement;
  resolvedState: string;
}

const imageCache = new Map<string, Promise<HTMLImageElement>>();

export function loadImage(url: string): Promise<HTMLImageElement> {
  const cached = imageCache.get(url);
  if (cached) return cached;
  const pending = new Promise<HTMLImageElement>((resolve, reject) => {
    if (!url) {
      reject(new Error("empty skin asset URL"));
      return;
    }
    const image = new Image();
    image.onload = () => resolve(image);
    image.onerror = () => reject(new Error(`failed to load skin asset: ${url}`));
    image.src = url;
  });
  imageCache.set(url, pending);
  return pending;
}

export async function loadSkinAnimation(manifest: SkinManifestV1, visualState: VisualState, assetURLs: Record<string, string>): Promise<LoadedAnimation | null> {
  const requested = resolveState(manifest, visualState);
  const candidates = [requested.state, ...(requested.state === "idle" ? [] : ["idle"])]
    .filter((state, index, all) => all.indexOf(state) === index);
  for (const state of candidates) {
    const url = assetURLs[state] ?? manifest.states[state] ?? "";
    try {
      const image = await loadImage(url);
      if (image.naturalWidth % manifest.frame_size !== 0) throw new Error("malformed horizontal sprite sheet");
      const frames = splitHorizontal(image.naturalWidth, image.naturalHeight, manifest.frame_size);
      if (frames.length === 0) throw new Error("sprite sheet has no complete frames");
      return {
        image,
        resolvedState: state,
        clip: { name: state, frames, fps: manifest.fps, loop: true },
      };
    } catch {
      // Missing or malformed requested assets fall back to idle. If idle also
      // fails, the UI uses its final emergency placeholder instead of blanking.
    }
  }
  return null;
}

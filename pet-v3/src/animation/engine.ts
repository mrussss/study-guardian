import type { SpriteFrame } from "./sprite";

export interface AnimationClip {
  name: string;
  frames: SpriteFrame[];
  fps: number;
  loop: boolean;
}

export type AnimationMode = "loop" | "one-shot";

export class AnimationEngine {
  private clip: AnimationClip | null = null;
  private mode: AnimationMode = "loop";
  private elapsedMs = 0;
  private frameIndex = 0;
  private complete: (() => void) | undefined;

  play(clip: AnimationClip): void { this.start(clip, "loop"); }
  loop(clip: AnimationClip): void { this.start({ ...clip, loop: true }, "loop"); }
  oneShot(clip: AnimationClip, onComplete?: () => void): void { this.complete = onComplete; this.start({ ...clip, loop: false }, "one-shot"); }
  stop(): void { this.clip = null; this.elapsedMs = 0; this.frameIndex = 0; this.complete = undefined; }

  update(deltaMs: number): void {
    if (!this.clip || this.clip.frames.length === 0 || !Number.isFinite(deltaMs) || deltaMs <= 0) return;
    const frameMs = 1000 / Math.max(1, this.clip.fps);
    this.elapsedMs += deltaMs;
    while (this.elapsedMs >= frameMs) {
      this.elapsedMs -= frameMs;
      this.frameIndex += 1;
      if (this.frameIndex < this.clip.frames.length) continue;
      if (this.mode === "one-shot" || !this.clip.loop) {
        this.frameIndex = this.clip.frames.length - 1;
        const callback = this.complete;
        this.complete = undefined;
        if (callback) callback();
        break;
      }
      this.frameIndex = 0;
    }
  }

  frame(): SpriteFrame | null { return this.clip?.frames[this.frameIndex] ?? null; }
  currentName(): string | null { return this.clip?.name ?? null; }

  private start(clip: AnimationClip, mode: AnimationMode): void {
    this.clip = clip;
    this.mode = mode;
    this.elapsedMs = 0;
    this.frameIndex = 0;
  }
}

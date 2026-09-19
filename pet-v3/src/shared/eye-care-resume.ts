import type { NativeEyeCareStatus, NativeSupervisorStatus } from "../transport/supervisor";

export type EyeCareResumeAction = "FINISH_EARLY" | "RESUME_STUDY";

export function eyeCareResumeAction(
  userMode: NativeSupervisorStatus["user_mode"] | undefined,
  modeOrigin: NativeSupervisorStatus["mode_origin"] | undefined,
  eyeCareStatus: NativeEyeCareStatus | undefined,
): EyeCareResumeAction | undefined {
  if (userMode !== "BREAK" || modeOrigin !== "EYE_CARE") return undefined;
  if (eyeCareStatus?.phase === "SHORT_BREAK" || eyeCareStatus?.phase === "LONG_BREAK") return "FINISH_EARLY";
  if (eyeCareStatus?.phase === "WAITING_RETURN") return "RESUME_STUDY";
  return undefined;
}

export function eyeCareResumeLabel(
  userMode: NativeSupervisorStatus["user_mode"] | undefined,
  modeOrigin: NativeSupervisorStatus["mode_origin"] | undefined,
  eyeCareStatus: NativeEyeCareStatus | undefined,
): string {
  if (eyeCareResumeAction(userMode, modeOrigin, eyeCareStatus) === "FINISH_EARLY") return "提前结束并继续";
  return "继续学习";
}

export function newEyeCareRequestId(): string {
  try {
    if (typeof crypto !== "undefined" && "randomUUID" in crypto) return crypto.randomUUID();
  } catch { /* use a local non-secret fallback */ }
  return `eye-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 12)}`;
}

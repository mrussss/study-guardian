const INTERACTIVE_TAGS = new Set(["BUTTON", "SELECT", "INPUT", "TEXTAREA", "A"]);

export type DragStartErrorKind = "ipc_rejected" | "window_unavailable" | "permission_denied" | "unknown";
export type DragDebugEvent =
  | "drag:down"
  | "drag:move"
  | "drag:threshold"
  | "drag:start-called"
  | "drag:start-ok"
  | `drag:start-failed:${DragStartErrorKind}`
  | "drag:click"
  | "drag:clear";

/** Convert a native drag failure into a bounded diagnostic category. */
export function classifyDragStartError(error: unknown): DragStartErrorKind {
  const message = (error instanceof Error ? error.message : String(error)).toLowerCase();
  if (message.includes("permission") || message.includes("denied")) return "permission_denied";
  if (message.includes("window") || message.includes("not found") || message.includes("closed")) return "window_unavailable";
  if (message.includes("invoke") || message.includes("ipc") || message.includes("command")) return "ipc_rejected";
  return "unknown";
}

export interface DragTargetShape {
  tagName?: string;
  ancestorTags?: readonly string[];
  inDevPanel?: boolean;
  hasNoDrag?: boolean;
}

/** Pure target classification used by the native drag gate and its tests. */
export function isInteractiveTargetShape(target: DragTargetShape | null): boolean {
  if (!target) return false;
  if (target.inDevPanel || target.hasNoDrag) return true;
  return [target.tagName, ...(target.ancestorTags ?? [])]
    .filter((tag): tag is string => typeof tag === "string")
    .some(tag => INTERACTIVE_TAGS.has(tag.toUpperCase()));
}

/** Pure event policy: only a native left-button press may start dragging. */
export function shouldStartDragging(button: number, nativeRuntime: boolean, interactiveTarget: boolean): boolean {
  return nativeRuntime && button === 0 && !interactiveTarget;
}

export interface PointerPoint {
  x: number;
  y: number;
}

export const PET_DRAG_THRESHOLD = 5;

export function movementDistance(start: PointerPoint, current: PointerPoint): number {
  return Math.hypot(current.x - start.x, current.y - start.y);
}

export function isClickGesture(start: PointerPoint, current: PointerPoint, threshold = PET_DRAG_THRESHOLD): boolean {
  return movementDistance(start, current) < threshold;
}

export function shouldBeginNativeDrag(start: PointerPoint, current: PointerPoint, threshold = PET_DRAG_THRESHOLD): boolean {
  return movementDistance(start, current) >= threshold;
}

export function isInteractiveTarget(target: EventTarget | null): boolean {
  if (typeof Element === "undefined" || !(target instanceof Element)) return false;
  const ancestorTags: string[] = [];
  let ancestor = target.parentElement;
  while (ancestor) {
    ancestorTags.push(ancestor.tagName);
    ancestor = ancestor.parentElement;
  }
  return isInteractiveTargetShape({
    tagName: target.tagName,
    ancestorTags,
    inDevPanel: target.closest("[data-dev-panel]") !== null,
    hasNoDrag: target.closest("[data-no-drag]") !== null,
  });
}

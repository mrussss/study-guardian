import { useRef, useState, type ReactElement } from "react";
import { Plus } from "lucide-react";
import type { NativeTaskPresetList } from "../transport/supervisor";
import { TaskMutationQueue, settleTaskPickerResult, type TaskPickerActionResult } from "./task-mutation";

export type { TaskPickerActionResult } from "./task-mutation";
export type TaskPickerAction = "select" | "temporary" | "save";

interface TaskPickerProps {
  currentTask: string;
  presets?: NativeTaskPresetList;
  compact?: boolean;
  variant?: "default" | "hero";
  disabled?: boolean;
  onSelect: (id: string) => Promise<TaskPickerActionResult>;
  onTemporary: (name: string) => Promise<TaskPickerActionResult>;
  onSavePinned: (name: string) => Promise<TaskPickerActionResult>;
  onOptimisticTaskChange?: (task: string | undefined) => void;
  onResult?: (result: TaskPickerActionResult, action: TaskPickerAction) => void | Promise<void>;
}

const TEMPORARY_PENDING_ID = "__temporary__";
const SAVE_PENDING_ID = "__save_pinned__";
const TASK_SELECTION_QUEUE_KEY = "task-selection";

export function TaskPicker({ currentTask, presets, compact = false, variant = "default", disabled = false, onSelect, onTemporary, onSavePinned, onOptimisticTaskChange, onResult }: TaskPickerProps): ReactElement {
  const [editing, setEditing] = useState(false);
  const [name, setName] = useState("");
  const [pendingId, setPendingId] = useState<string | null>(null);
  const requestRevision = useRef(0);
  const mutationQueue = useRef(new TaskMutationQueue()).current;
  const pinned = presets?.pinned ?? [];
  const pinnedIDs = new Set(pinned.map(item => item.id));
  const recent = (presets?.recent ?? []).filter(item => !pinnedIDs.has(item.id));
  const displayedTask = currentTask;
  const isSelected = (_id: string, taskName: string): boolean => taskName === displayedTask;
  const run = async (pendingKey: string, taskName: string, action: TaskPickerAction, operation: () => Promise<TaskPickerActionResult>): Promise<void> => {
    if (disabled || pendingId === pendingKey) return;
    const revision = requestRevision.current + 1;
    requestRevision.current = revision;
    setPendingId(pendingKey);
    onOptimisticTaskChange?.(taskName);

    const queued = await mutationQueue.enqueue(async () => {
      try {
        return await operation();
      } catch {
        return { ok: false, error_kind: "unavailable" };
      }
    }, { coalesceKey: action === "select" ? TASK_SELECTION_QUEUE_KEY : undefined });
    if (queued.status === "superseded") return;
    const result = queued.status === "completed" ? queued.value : { ok: false, error_kind: "unavailable" };
    const settled = settleTaskPickerResult(requestRevision.current, revision, result);
    if (!settled.applied) return;
    setPendingId(null);
    if (!settled.result.ok) {
      onOptimisticTaskChange?.(undefined);
    } else if (action !== "select") {
      setName("");
      setEditing(false);
    }
    void onResult?.(settled.result, action);
  };

  return <section className={`task-picker ${compact ? "is-compact" : ""} ${variant === "hero" ? "is-hero" : ""}`} aria-label="当前学习任务">
    {variant !== "hero" && <div className="task-picker-current"><span>当前任务</span><strong>{displayedTask || "未设置任务"}</strong></div>}
    <div className="task-picker-group"><span>常用</span><div className="task-chip-row">
      {pinned.map(item => <button className={isSelected(item.id, item.name) ? "task-chip is-active" : "task-chip"} type="button" aria-pressed={isSelected(item.id, item.name)} aria-busy={pendingId === item.id} disabled={disabled} key={item.id} onClick={() => void run(item.id, item.name, "select", () => onSelect(item.id))}>{item.name}</button>)}
      <button className="task-chip task-chip-add" type="button" aria-label="新建学习任务" disabled={disabled || pendingId === TEMPORARY_PENDING_ID || pendingId === SAVE_PENDING_ID} onClick={() => setEditing(value => !value)}><Plus size={14} /></button>
      {pinned.length === 0 && !editing && <em>还没有常用任务</em>}
    </div></div>
    {!compact && recent.length > 0 && <div className="task-picker-group"><span>最近使用</span><div className="task-chip-row">{recent.map(item => <button className={isSelected(item.id, item.name) ? "task-chip is-active" : "task-chip"} type="button" aria-pressed={isSelected(item.id, item.name)} aria-busy={pendingId === item.id} disabled={disabled} key={item.id} onClick={() => void run(item.id, item.name, "select", () => onSelect(item.id))}>{item.name}</button>)}</div></div>}
    {editing && <div className="task-create-row"><input autoFocus maxLength={64} value={name} placeholder="输入任务名" onChange={event => setName(event.target.value)} onKeyDown={event => { if (event.key === "Escape") setEditing(false); }} /><button type="button" disabled={disabled || pendingId === TEMPORARY_PENDING_ID || !name.trim()} onClick={() => void run(TEMPORARY_PENDING_ID, name.trim(), "temporary", () => onTemporary(name.trim()))}>仅本次</button><button className="is-primary" type="button" disabled={disabled || pendingId === SAVE_PENDING_ID || !name.trim()} onClick={() => void run(SAVE_PENDING_ID, name.trim(), "save", () => onSavePinned(name.trim()))}>保存常用</button></div>}
  </section>;
}

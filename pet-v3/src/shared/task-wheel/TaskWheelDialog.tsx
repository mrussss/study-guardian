import { useEffect, useMemo, useRef, useState, type CSSProperties, type ReactElement } from "react";
import { ArrowDown, ArrowUp, Plus, Settings2, X } from "lucide-react";
import { TaskMutationQueue, settleTaskPickerResult, type TaskPickerActionResult } from "../task-mutation";
import { TaskEditor } from "./TaskEditor";

export type TaskWheelAction = "select" | "temporary" | "save";

type WheelProps = {
  open: boolean;
  onClose: () => void;
  currentTask: string;
  presets?: { pinned: Array<{ id: string; name: string; pinned: boolean; sort_order: number; use_count: number }>; recent: Array<{ id: string; name: string; pinned: boolean; sort_order: number; use_count: number }> };
  compact?: boolean;
  disabled?: boolean;
  onSelect: (id: string) => Promise<TaskPickerActionResult>;
  onTemporary: (name: string) => Promise<TaskPickerActionResult>;
  onSavePinned: (name: string) => Promise<TaskPickerActionResult>;
  onUpdatePreset?: (id: string, name: string, pinned: boolean, sortOrder: number) => Promise<TaskPickerActionResult>;
  onDeletePreset?: (id: string) => Promise<TaskPickerActionResult>;
  onOptimisticTaskChange?: (task: string | undefined) => void;
  onTaskMutationStarted?: () => void;
  onResult?: (result: TaskPickerActionResult, action: TaskWheelAction) => void | Promise<void>;
};

export const TASK_WHEEL_ADD_SLOT = 5;
export const TASK_WHEEL_TASK_SLOTS = [0, 1, 2, 3, 4, 6, 7];

export function getTaskWheelSlotPosition(slotIndex: number, compact = false): { x: number; y: number } {
  const angle = -90 + slotIndex * 45;
  const radius = compact ? 35 : 37;
  return { x: 50 + Math.cos(angle * Math.PI / 180) * radius, y: 50 + Math.sin(angle * Math.PI / 180) * radius };
}

function polarPoint(angle: number, radius: number): { x: number; y: number } {
  return { x: 50 + Math.cos(angle * Math.PI / 180) * radius, y: 50 + Math.sin(angle * Math.PI / 180) * radius };
}

export function getTaskWheelSectorPath(slotIndex: number): string {
  const centerAngle = -90 + slotIndex * 45;
  const start = polarPoint(centerAngle - 22.5, 48);
  const end = polarPoint(centerAngle + 22.5, 48);
  return `M 50 50 L ${start.x.toFixed(3)} ${start.y.toFixed(3)} A 48 48 0 0 1 ${end.x.toFixed(3)} ${end.y.toFixed(3)} Z`;
}

export function TaskWheelDialog({ open, onClose, currentTask, presets, compact = false, disabled = false, onSelect, onTemporary, onSavePinned, onUpdatePreset, onDeletePreset, onOptimisticTaskChange, onTaskMutationStarted, onResult }: WheelProps): ReactElement | null {
  const dialogRef = useRef<HTMLDivElement>(null);
  const requestRevision = useRef(0);
  const mutationQueue = useRef(new TaskMutationQueue()).current;
  const [pendingId, setPendingId] = useState<string | null>(null);
  const [highlight, setHighlight] = useState(0);
  const [editing, setEditing] = useState<"new" | string | null>(null);
  const [editorName, setEditorName] = useState("");
  const [manage, setManage] = useState(false);
  const [notice, setNotice] = useState("");
  const pinned = useMemo(() => (presets?.pinned ?? []).slice(0, 7), [presets?.pinned]);
  const selectableSlots = useMemo(() => TASK_WHEEL_TASK_SLOTS.slice(0, pinned.length), [pinned.length]);
  const highlightedTask = pinned[selectableSlots.indexOf(highlight)] ?? pinned[0];

  useEffect(() => {
    if (!open) return;
    setNotice("");
    setEditing(null);
    setManage(false);
    dialogRef.current?.querySelector<HTMLElement>("[data-wheel-close]")?.focus();
  }, [open]);

  useEffect(() => {
    if (!open) return;
    const currentIndex = pinned.findIndex(item => item.name === currentTask);
    setHighlight(currentIndex >= 0 ? selectableSlots[currentIndex] : selectableSlots[0] ?? 0);
  }, [open, currentTask, pinned, selectableSlots]);

  useEffect(() => {
    if (!open) return;
    const onKeyDown = (event: KeyboardEvent): void => {
      if (event.key === "Escape") { event.preventDefault(); onClose(); return; }
      if (event.key === "ArrowRight" || event.key === "ArrowDown" || event.key === "ArrowLeft" || event.key === "ArrowUp") {
        if (selectableSlots.length === 0) return;
        event.preventDefault();
        const current = Math.max(0, selectableSlots.indexOf(highlight));
        const delta = event.key === "ArrowRight" || event.key === "ArrowDown" ? 1 : -1;
        setHighlight(selectableSlots[(current + delta + selectableSlots.length) % selectableSlots.length]);
      }
      if (event.key === "Enter" && highlightedTask) { event.preventDefault(); void runSelect(highlightedTask.id, highlightedTask.name); }
    };
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [open, onClose, selectableSlots, highlight, highlightedTask]);

  if (!open) return null;

  const runMutation = async (pendingKey: string, taskName: string, action: TaskWheelAction, operation: () => Promise<TaskPickerActionResult>): Promise<void> => {
    if (disabled || pendingId === pendingKey) return;
    if (action === "select" && taskName === currentTask) return;
    const revision = requestRevision.current + 1;
    requestRevision.current = revision;
    setPendingId(pendingKey);
    onTaskMutationStarted?.();
    onOptimisticTaskChange?.(taskName);
    const queued = await mutationQueue.enqueue(async () => {
      try { return await operation(); } catch { return { ok: false, error_kind: "unavailable" }; }
    }, { coalesceKey: action === "select" ? "task-selection" : undefined });
    if (queued.status === "superseded") return;
    const result = queued.status === "completed" ? queued.value : { ok: false, error_kind: "unavailable" };
    const settled = settleTaskPickerResult(requestRevision.current, revision, result);
    if (!settled.applied) return;
    setPendingId(null);
    if (!settled.result.ok) onOptimisticTaskChange?.(undefined);
    else if (action !== "select") { setEditorName(""); setEditing(null); }
    void onResult?.(settled.result, action);
    if (settled.result.ok && action === "select") onClose();
  };

  const runSelect = (id: string, name: string): Promise<void> => runMutation(id, name, "select", () => onSelect(id));
  const beginAdd = (): void => { setNotice(""); setEditing("new"); setEditorName(""); };
  const beginRename = (item: { id: string; name: string }): void => { setNotice(""); setEditing(item.id); setEditorName(item.name); };
  const saveRename = async (item: { id: string; name: string; pinned: boolean; sort_order: number }): Promise<void> => {
    if (!onUpdatePreset || !editorName.trim()) return;
    const result = await onUpdatePreset(item.id, editorName.trim(), item.pinned, item.sort_order);
    setNotice(result.ok ? "任务名称已更新" : "任务名称更新失败");
    if (result.ok) { setEditing(null); setEditorName(""); }
  };
  const deletePreset = async (item: { id: string; name: string }): Promise<void> => {
    if (!onDeletePreset) return;
    if (item.name === currentTask) { setNotice("请先选择其他任务，再删除当前任务"); return; }
    const result = await onDeletePreset(item.id);
    setNotice(result.ok ? "任务已从轮盘移除" : "删除失败，任务仍保留");
  };

  return <div className="task-wheel-overlay" role="presentation" onMouseDown={event => { if (event.target === event.currentTarget) onClose(); }}>
    <div className={`task-wheel-dialog ${compact ? "is-compact" : ""} ${manage ? "is-managing" : ""}`} role="dialog" aria-modal="true" aria-labelledby="task-wheel-title" ref={dialogRef}>
      <header className="task-wheel-header"><div><span className="task-wheel-eyebrow">快速切换</span><h2 id="task-wheel-title">选择当前专注任务</h2></div><button className="task-wheel-icon-button" type="button" data-wheel-close aria-label="关闭任务轮盘" onClick={onClose}><X size={17} /></button></header>
      <div className="task-wheel-body">
        <div className="task-wheel-stage" aria-label="常用任务轮盘">
          <svg className="task-wheel-svg" viewBox="0 0 100 100" aria-hidden="true">
            <circle className="task-wheel-disc" cx="50" cy="50" r="48" />
            {Array.from({ length: 8 }, (_, slot) => <path className={`task-wheel-sector ${highlight === slot ? "is-highlighted" : ""} ${pinned[selectableSlots.indexOf(slot)]?.name === currentTask ? "is-current" : ""}`} d={getTaskWheelSectorPath(slot)} key={slot} />)}
            <circle className="task-wheel-center-ring" cx="50" cy="50" r="16" />
          </svg>
          {Array.from({ length: 8 }, (_, slot) => {
            const itemIndex = TASK_WHEEL_TASK_SLOTS.indexOf(slot);
            const item = itemIndex >= 0 ? pinned[itemIndex] : undefined;
            const isAdd = slot === TASK_WHEEL_ADD_SLOT;
            const position = getTaskWheelSlotPosition(slot, compact);
            const slotStyle = { "--slot-x": `${position.x}%`, "--slot-y": `${position.y}%` } as CSSProperties;
            return <div className={`task-wheel-slot ${highlight === slot ? "is-highlighted" : ""}`} style={slotStyle} key={slot}>
              {isAdd ? <button className="task-wheel-add" type="button" onClick={beginAdd} aria-label="添加常用任务"><Plus size={18} /></button> : item ? <button className={`task-wheel-item ${item.name === currentTask ? "is-current" : ""} ${highlight === slot ? "is-highlighted" : ""}`} type="button" aria-pressed={item.name === currentTask} aria-busy={pendingId === item.id} onClick={() => { setHighlight(slot); void runSelect(item.id, item.name); }}><span>{item.name}</span></button> : <span className="task-wheel-empty" aria-hidden="true" />}
            </div>;
          })}
          <div className="task-wheel-center"><span>当前任务</span><strong>{currentTask.trim() || "未设置任务"}</strong></div>
        </div>
        <div className="task-wheel-toolbar"><span>{pinned.length} / 7 个常用任务</span><button type="button" onClick={() => setManage(value => !value)}><Settings2 size={14} />{manage ? "完成管理" : "管理任务"}</button></div>
        {editing === "new" && <TaskEditor name={editorName} onName={setEditorName} onTemporary={() => void runMutation("__temporary__", editorName.trim(), "temporary", () => onTemporary(editorName.trim()))} onSave={() => void runMutation("__save__", editorName.trim(), "save", () => onSavePinned(editorName.trim()))} onCancel={() => setEditing(null)} busy={pendingId !== null} />}
        {manage && <div className="task-wheel-manage">{pinned.length === 0 && <span className="task-wheel-manage-empty">还没有常用任务</span>}{pinned.map((item, index) => <div className="task-wheel-manage-row" key={item.id}><span>{item.name}</span><button type="button" aria-label={`上移 ${item.name}`} disabled={!onUpdatePreset || index === 0} onClick={() => void onUpdatePreset?.(item.id, item.name, item.pinned, Math.max(0, item.sort_order - 1))}><ArrowUp size={14} /></button><button type="button" aria-label={`下移 ${item.name}`} disabled={!onUpdatePreset || index === pinned.length - 1} onClick={() => void onUpdatePreset?.(item.id, item.name, item.pinned, item.sort_order + 1)}><ArrowDown size={14} /></button><button type="button" onClick={() => beginRename(item)}>改名</button><button type="button" onClick={() => void deletePreset(item)}>删除</button></div>)}</div>}
        {editing && editing !== "new" && pinned.find(item => item.id === editing) && <TaskEditor name={editorName} onName={setEditorName} onTemporary={() => void saveRename(pinned.find(item => item.id === editing)!)} onSave={() => void saveRename(pinned.find(item => item.id === editing)!)} onCancel={() => setEditing(null)} busy={pendingId !== null} />}
        {notice && <p className="task-wheel-notice" role="status">{notice}</p>}
      </div>
    </div>
  </div>;
}

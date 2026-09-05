import { useState, type ReactElement } from "react";
import { Plus } from "lucide-react";
import type { NativeTaskPresetList } from "../transport/supervisor";

interface TaskPickerProps {
  currentTask: string;
  presets?: NativeTaskPresetList;
  compact?: boolean;
  disabled?: boolean;
  onSelect: (id: string) => Promise<boolean>;
  onTemporary: (name: string) => Promise<boolean>;
  onSavePinned: (name: string) => Promise<boolean>;
}

export function TaskPicker({ currentTask, presets, compact = false, disabled = false, onSelect, onTemporary, onSavePinned }: TaskPickerProps): ReactElement {
  const [editing, setEditing] = useState(false);
  const [name, setName] = useState("");
  const [busy, setBusy] = useState(false);
  const pinned = presets?.pinned ?? [];
  const pinnedIDs = new Set(pinned.map(item => item.id));
  const recent = (presets?.recent ?? []).filter(item => !pinnedIDs.has(item.id));

  const run = async (operation: () => Promise<boolean>): Promise<void> => {
    if (busy || disabled) return;
    setBusy(true);
    try {
      if (await operation()) {
        setName("");
        setEditing(false);
      }
    } finally {
      setBusy(false);
    }
  };

  return <section className={`task-picker ${compact ? "is-compact" : ""}`} aria-label="当前学习任务">
    <div className="task-picker-current"><span>当前任务</span><strong>{currentTask || "未设置任务"}</strong></div>
    <div className="task-picker-group"><span>常用</span><div className="task-chip-row">
      {pinned.map(item => <button className={item.name === currentTask ? "task-chip is-active" : "task-chip"} type="button" disabled={busy || disabled} key={item.id} onClick={() => void run(() => onSelect(item.id))}>{item.name}</button>)}
      <button className="task-chip task-chip-add" type="button" aria-label="新建学习任务" disabled={busy || disabled} onClick={() => setEditing(value => !value)}><Plus size={14} /></button>
      {pinned.length === 0 && !editing && <em>还没有常用任务</em>}
    </div></div>
    {!compact && recent.length > 0 && <div className="task-picker-group"><span>最近使用</span><div className="task-chip-row">{recent.map(item => <button className="task-chip" type="button" disabled={busy || disabled} key={item.id} onClick={() => void run(() => onSelect(item.id))}>{item.name}</button>)}</div></div>}
    {editing && <div className="task-create-row"><input autoFocus maxLength={64} value={name} placeholder="输入任务名" onChange={event => setName(event.target.value)} onKeyDown={event => { if (event.key === "Escape") setEditing(false); }} /><button type="button" disabled={busy || !name.trim()} onClick={() => void run(() => onTemporary(name))}>仅本次</button><button className="is-primary" type="button" disabled={busy || !name.trim()} onClick={() => void run(() => onSavePinned(name))}>保存常用</button></div>}
  </section>;
}

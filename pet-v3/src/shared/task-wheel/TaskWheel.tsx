import { useState, type ReactElement } from "react";
import { TaskTrigger } from "./TaskTrigger";
import { TaskWheelDialog, type TaskWheelAction } from "./TaskWheelDialog";
import type { TaskPickerActionResult } from "../task-mutation";

export function TaskWheel({ currentTask, presets, compact = false, disabled = false, onSelect, onTemporary, onSavePinned, onUpdatePreset, onDeletePreset, onOptimisticTaskChange, onTaskMutationStarted, onResult }: {
  currentTask: string;
  presets?: Parameters<typeof TaskWheelDialog>[0]["presets"];
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
}): ReactElement {
  const [open, setOpen] = useState(false);
  return <div className={`task-wheel ${compact ? "is-compact" : ""}`}>
    <TaskTrigger task={currentTask} compact={compact} disabled={disabled} onClick={() => setOpen(true)} />
    <TaskWheelDialog open={open} onClose={() => setOpen(false)} currentTask={currentTask} presets={presets} compact={compact} disabled={disabled} onSelect={onSelect} onTemporary={onTemporary} onSavePinned={onSavePinned} onUpdatePreset={onUpdatePreset} onDeletePreset={onDeletePreset} onOptimisticTaskChange={onOptimisticTaskChange} onTaskMutationStarted={onTaskMutationStarted} onResult={onResult} />
  </div>;
}

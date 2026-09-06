import { ChevronDown } from "lucide-react";
import type { ReactElement } from "react";

export function TaskTrigger({ task, disabled = false, compact = false, onClick }: {
  task: string;
  disabled?: boolean;
  compact?: boolean;
  onClick: () => void;
}): ReactElement {
  return <button className={`task-wheel-trigger ${compact ? "is-compact" : ""}`} type="button" disabled={disabled} onClick={onClick} aria-haspopup="dialog">
    <span className="task-wheel-trigger-label">当前任务</span><span className="task-wheel-trigger-value">{task.trim() || "未设置任务"}</span><ChevronDown size={15} aria-hidden="true" />
  </button>;
}

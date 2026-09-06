import { Check, X } from "lucide-react";
import type { ReactElement } from "react";

export function TaskEditor({ name, onName, onTemporary, onSave, onCancel, busy = false }: {
  name: string;
  onName: (value: string) => void;
  onTemporary: () => void;
  onSave: () => void;
  onCancel?: () => void;
  busy?: boolean;
}): ReactElement {
  return <div className="task-wheel-editor">
    <label><span>任务名称</span><input autoFocus maxLength={64} value={name} placeholder="例如：Go / 算法 / 阅读" onChange={event => onName(event.target.value)} /></label>
    <div className="task-wheel-editor-actions">
      <button type="button" disabled={busy || !name.trim()} onClick={onTemporary}>仅本次使用</button>
      <button className="is-primary" type="button" disabled={busy || !name.trim()} onClick={onSave}><Check size={14} />保存到轮盘</button>
      {onCancel && <button className="is-quiet" type="button" disabled={busy} onClick={onCancel}><X size={14} />取消</button>}
    </div>
  </div>;
}

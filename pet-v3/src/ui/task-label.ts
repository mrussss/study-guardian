export function getPetTaskLabel(connected: boolean, task: string | undefined): string {
  if (!connected) return "监督离线";
  return task?.trim() || "未设置任务";
}

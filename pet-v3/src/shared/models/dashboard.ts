export interface FocusDay {
  label: string;
  minutes: number;
  target: number;
  completed: boolean;
}

export function clampProgress(value: number): number {
  return Math.max(0, Math.min(1, Number.isFinite(value) ? value : 0));
}

export function formatFocusMinutes(minutes: number): string {
  const safe = Math.max(0, Math.round(minutes));
  const hours = Math.floor(safe / 60);
  const rest = safe % 60;
  return hours > 0 ? `${hours}h ${rest.toString().padStart(2, "0")}m` : `${rest}m`;
}

export function totalFocusMinutes(days: readonly FocusDay[]): number {
  return days.reduce((total, day) => total + Math.max(0, day.minutes), 0);
}

import { JSDOM } from "jsdom";
import { strict as assert } from "node:assert";
import test, { afterEach } from "node:test";
import { EyeCareSettingsCard, EyeCareWidget } from "./EyeCare";
import type { NativeEyeCareSettings, NativeEyeCareStatus, NativeSupervisorStatus, SupervisorDashboardSnapshot } from "../transport/supervisor";
import { getSupervisorControlAdapter } from "../runtime/adapters";

const dom = new JSDOM("<!doctype html><html><body></body></html>", { url: "http://localhost" });
Object.defineProperty(globalThis, "window", { value: dom.window, configurable: true });
Object.defineProperty(globalThis, "document", { value: dom.window.document, configurable: true });
Object.defineProperty(globalThis, "navigator", { value: dom.window.navigator, configurable: true });
Object.defineProperty(globalThis, "HTMLElement", { value: dom.window.HTMLElement, configurable: true });
Object.defineProperty(globalThis, "Node", { value: dom.window.Node, configurable: true });
Object.defineProperty(globalThis, "MutationObserver", { value: dom.window.MutationObserver, configurable: true });
Object.defineProperty(globalThis, "getComputedStyle", { value: dom.window.getComputedStyle.bind(dom.window), configurable: true });
(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

const { cleanup, fireEvent, render, screen, waitFor } = await import("@testing-library/react");
afterEach(() => cleanup());

const settings: NativeEyeCareSettings = {
  enabled: true, focus_minutes: 40, short_break_minutes: 5,
  long_break_after_focus_minutes: 120, long_break_minutes: 20, snooze_minutes: 5, max_snoozes: 2,
};

function eyeStatus(phase: NativeEyeCareStatus["phase"]): NativeEyeCareStatus {
  return {
    enabled: true, phase, local_date: "2026-09-13", focus_segment_seconds: phase === "SHORT_BREAK_DUE" ? 2400 : 0,
    focus_since_long_break_seconds: phase === "LONG_BREAK_DUE" ? 7200 : 2400,
    ...(phase === "SHORT_BREAK" ? { break_started_at: "2026-09-13T10:00:00Z", planned_break_end_at: "2026-09-13T10:05:00Z" } : {}),
    snooze_count: 0, completed_short_breaks: 0, completed_long_breaks: 0, retry_focus_after_seconds: 0,
    revision: 3, updated_at: "2026-09-13T10:00:00Z", notification_suppressed: false,
  };
}

const supervisorStatus: NativeSupervisorStatus = {
  user_mode: "STUDY", interaction_state: "ACTIVE", task_relation: "FOCUSED", privacy_state: "NORMAL",
  confidence: .9, task: "Go", study_seconds: 2400, break_seconds: 0, active_seconds: 2400,
  activitywatch_ok: true, screen_sensor_ok: true,
};

function fakeControl(overrides: Record<string, unknown> = {}): ReturnType<typeof getSupervisorControlAdapter> {
  return {
    saveEyeCareSettings: async () => ({ ok: true }), eyeCareAction: async () => ({ ok: true }), ...overrides,
  } as unknown as ReturnType<typeof getSupervisorControlAdapter>;
}

test("due widget uses Chinese copy and starts a break only through the canonical action", async () => {
  let action = "";
  let refreshes = 0;
  const view = render(<EyeCareWidget connected settings={settings} eyeCareStatus={eyeStatus("SHORT_BREAK_DUE")} supervisorStatus={supervisorStatus} userMode="STUDY" control={fakeControl({ eyeCareAction: async (value: string) => { action = value; return { ok: true }; } })} onRefresh={async () => { refreshes += 1; }} />);
  assert.ok(screen.getByText("该远眺休息 5 分钟了"));
  assert.equal(view.container.textContent?.includes("SHORT_BREAK_DUE"), false);
  fireEvent.click(screen.getByRole("button", { name: "开始远眺" }));
  await waitFor(() => assert.equal(refreshes, 1));
  assert.equal(action, "START_SHORT_BREAK");
  // No optimistic state transition: the next canonical snapshot owns the phase.
  assert.ok(screen.getByText("该远眺休息 5 分钟了"));
});

test("computer-use due widget is actionable outside study and uses basis-specific copy", async () => {
  let action = "";
  const computerSettings = { ...settings, counting_basis: "COMPUTER_USAGE" as const };
  const standby = { ...supervisorStatus, user_mode: "STANDBY" as const, task_relation: "UNKNOWN" as const };
  render(<EyeCareWidget connected settings={computerSettings} eyeCareStatus={eyeStatus("SHORT_BREAK_DUE")} supervisorStatus={standby} userMode="STANDBY" control={fakeControl({ eyeCareAction: async (value: string) => { action = value; return { ok: true }; } })} />);
  assert.ok(screen.getByText("该远眺休息 5 分钟了"));
  assert.ok(screen.getByText(/已经连续使用电脑/));
  fireEvent.click(screen.getByRole("button", { name: "开始休息" }));
  await waitFor(() => assert.equal(action, "START_SHORT_BREAK"));
});

test("break, waiting-return and degraded-data states have safe Chinese copy", () => {
  const view = render(<EyeCareWidget connected settings={settings} eyeCareStatus={eyeStatus("SHORT_BREAK")} supervisorStatus={supervisorStatus} userMode="BREAK" control={fakeControl()} />);
  assert.ok(screen.getByText("远眺休息中"));
  view.rerender(<EyeCareWidget connected settings={settings} eyeCareStatus={eyeStatus("WAITING_RETURN")} supervisorStatus={supervisorStatus} userMode="BREAK" control={fakeControl()} />);
  assert.ok(screen.getByText("本轮休息计时完成"));
  view.rerender(<EyeCareWidget connected settings={settings} eyeCareStatus={eyeStatus("FOCUSING")} supervisorStatus={{ ...supervisorStatus, activitywatch_ok: false }} userMode="STUDY" control={fakeControl()} />);
  assert.ok(screen.getByText("等待有效专注数据"));
});

test("disconnection retains last known phase without continuing to offer actions", () => {
  const view = render(<EyeCareWidget connected settings={settings} eyeCareStatus={eyeStatus("SHORT_BREAK_DUE")} supervisorStatus={supervisorStatus} userMode="STUDY" control={fakeControl()} />);
  view.rerender(<EyeCareWidget connected={false} control={fakeControl()} />);
  assert.ok(screen.getByText("护眼状态暂不可用"));
  assert.ok(screen.getByText("连接恢复后将继续显示最新状态"));
  assert.equal(screen.queryByRole("button", { name: "开始远眺" }), null);
});

test("snoozed reminders hide active actions and degraded persistence is visible", () => {
  const snoozed = { ...eyeStatus("SHORT_BREAK_DUE"), snooze_until: new Date(Date.now() + 60_000).toISOString(), snooze_count: 1 };
  const view = render(<EyeCareWidget connected settings={settings} eyeCareStatus={snoozed} supervisorStatus={supervisorStatus} userMode="STUDY" control={fakeControl()} />);
  assert.ok(screen.getByText("护眼提醒已延后"));
  assert.equal(screen.queryByRole("button", { name: "开始远眺" }), null);
  view.rerender(<EyeCareWidget connected settings={settings} eyeCareStatus={{ ...eyeStatus("SHORT_BREAK_DUE"), storage_degraded: true }} supervisorStatus={supervisorStatus} userMode="STUDY" control={fakeControl()} />);
  assert.ok(screen.getByText("护眼状态暂未保存，服务正在重试"));
});

test("settings protect a dirty draft from stale snapshots and retain it on failure", async () => {
  const control = fakeControl({ saveEyeCareSettings: async () => ({ ok: false, error_kind: "rejected" }) });
  const view = render(<EyeCareSettingsCard connected settings={settings} control={control} />);
  const focus = screen.getByRole("spinbutton", { name: "有效专注间隔（分钟）" });
  fireEvent.change(focus, { target: { value: "55" } });
  view.rerender(<EyeCareSettingsCard connected settings={{ ...settings, focus_minutes: 45 }} control={control} />);
  assert.equal((focus as HTMLInputElement).value, "55");
  fireEvent.click(screen.getByRole("button", { name: "保存护眼设置" }));
  await waitFor(() => assert.ok(screen.getByText("设置未通过校验，请检查数值")));
  assert.equal((focus as HTMLInputElement).value, "55");
});

test("settings keep a dirty draft when refresh still returns a stale value", async () => {
  const stale: SupervisorDashboardSnapshot = { connected: true, eye_care_settings: { ...settings, focus_minutes: 45 } };
  const control = fakeControl({ saveEyeCareSettings: async () => ({ ok: true }) });
  render(<EyeCareSettingsCard connected settings={settings} control={control} onRefresh={async () => stale} />);
  const focus = screen.getByRole("spinbutton", { name: "有效专注间隔（分钟）" });
  fireEvent.change(focus, { target: { value: "55" } });
  fireEvent.click(screen.getByRole("button", { name: "保存护眼设置" }));
  await waitFor(() => assert.ok(screen.getByText("已提交，等待服务确认")));
  assert.equal((focus as HTMLInputElement).value, "55");
  assert.equal((screen.getByRole("button", { name: "保存护眼设置" }) as HTMLButtonElement).disabled, false);
});

test("later canonical confirmation clears dirty only when it matches the submitted draft", async () => {
  const control = fakeControl({ saveEyeCareSettings: async () => ({ ok: true }) });
  const view = render(<EyeCareSettingsCard connected settings={settings} control={control} onRefresh={async () => ({ connected: true, eye_care_settings: { ...settings, focus_minutes: 55 } })} />);
  const focus = screen.getByRole("spinbutton", { name: "有效专注间隔（分钟）" });
  fireEvent.change(focus, { target: { value: "55" } });
  fireEvent.click(screen.getByRole("button", { name: "保存护眼设置" }));
  await waitFor(() => assert.ok(screen.getByText("护眼设置已保存")));
  assert.equal((screen.getByRole("button", { name: "保存护眼设置" }) as HTMLButtonElement).disabled, true);
  view.rerender(<EyeCareSettingsCard connected settings={{ ...settings, focus_minutes: 60 }} control={control} />);
  await waitFor(() => assert.equal((focus as HTMLInputElement).value, "60"));
});

test("a late canonical response cannot overwrite a newer edit made during save", async () => {
  let completeSave: ((value: { ok: boolean }) => void) | undefined;
  const control = fakeControl({ saveEyeCareSettings: async () => new Promise(resolve => { completeSave = resolve; }) });
  render(<EyeCareSettingsCard connected settings={settings} control={control} onRefresh={async () => ({ connected: true, eye_care_settings: { ...settings, focus_minutes: 55 } })} />);
  const focus = screen.getByRole("spinbutton", { name: "有效专注间隔（分钟）" });
  fireEvent.change(focus, { target: { value: "55" } });
  fireEvent.click(screen.getByRole("button", { name: "保存护眼设置" }));
  fireEvent.change(focus, { target: { value: "60" } });
  completeSave?.({ ok: true });
  await waitFor(() => assert.ok(screen.getByText("服务已确认先前提交；当前草稿仍待保存")));
  assert.equal((focus as HTMLInputElement).value, "60");
  assert.equal((screen.getByRole("button", { name: "保存护眼设置" }) as HTMLButtonElement).disabled, false);
});

test("disconnect preserves an unsaved settings draft and disables saving", () => {
  const control = fakeControl();
  const view = render(<EyeCareSettingsCard connected settings={settings} control={control} />);
  const focus = screen.getByRole("spinbutton", { name: "有效专注间隔（分钟）" });
  fireEvent.change(focus, { target: { value: "55" } });
  view.rerender(<EyeCareSettingsCard connected={false} settings={{ ...settings, focus_minutes: 45 }} control={control} />);
  assert.equal((focus as HTMLInputElement).value, "55");
  assert.equal((screen.getByRole("button", { name: "保存护眼设置" }) as HTMLButtonElement).disabled, true);
});

import { JSDOM } from "jsdom";
import { strict as assert } from "node:assert";
import test, { afterEach } from "node:test";
import { AutomationSettingsCard } from "./App";
import type { NativeAutomationSettings, SupervisorControlAdapter } from "../transport/supervisor";

const dom = new JSDOM("<!doctype html><html><body></body></html>", { url: "http://localhost" });
Object.defineProperty(globalThis, "window", { value: dom.window, configurable: true });
Object.defineProperty(globalThis, "document", { value: dom.window.document, configurable: true });
Object.defineProperty(globalThis, "navigator", { value: dom.window.navigator, configurable: true });
Object.defineProperty(globalThis, "HTMLElement", { value: dom.window.HTMLElement, configurable: true });
Object.defineProperty(globalThis, "Node", { value: dom.window.Node, configurable: true });
Object.defineProperty(globalThis, "MutationObserver", { value: dom.window.MutationObserver, configurable: true });
Object.defineProperty(globalThis, "getComputedStyle", { value: dom.window.getComputedStyle.bind(dom.window), configurable: true });
(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

const { cleanup, render, screen, waitFor } = await import("@testing-library/react");
const userEvent = (await import("@testing-library/user-event")).default;

afterEach(() => cleanup());

const settings: NativeAutomationSettings = {
  enabled: false,
  auto_start: { enabled: true, focused_stable_seconds: 90, min_confidence: .8, allow_unclassified: true, confirm: false },
  auto_pause: { enabled: true, idle_static_seconds: 300, idle_dynamic_seconds: 900, locked_seconds: 15, confirm: false },
  auto_resume: { enabled: true, focused_stable_seconds: 45 },
  transition_cooldown_seconds: 30,
  manual_override_minutes: 30,
};

function controlAdapter(overrides: Partial<SupervisorControlAdapter> = {}): SupervisorControlAdapter {
  const ok = async () => ({ ok: true as const });
  return {
    setModeStudy: ok, setModeBreak: ok, setModeOff: ok, setTask: ok,
    createTaskPreset: ok, selectTaskPreset: ok, updateTaskPreset: ok, deleteTaskPreset: ok,
    setReminderSettings: ok, saveAISettings: ok, putAISecret: ok, deleteAISecret: ok,
    testAIConnection: async () => ({ ok: true, provider: "none", model: "", latency_ms: 0 }),
    testAIProxy: async () => ({ ok: true, mode: "environment" as const, latency_ms: 0 }),
    generateReview: ok, setDailyTarget: ok, createMission: ok, completeMission: ok, cancelMission: ok,
    ...overrides,
  } as SupervisorControlAdapter;
}

test("automation draft survives a stale dashboard source until save", async () => {
  let saved: NativeAutomationSettings | undefined;
  const control = controlAdapter({ saveAutomationSettings: async value => { saved = value; return { ok: true }; } });
  const view = render(<AutomationSettingsCard settings={settings} control={control} onRefresh={() => Promise.resolve({ automation_settings: { ...settings, enabled: true } } as never)} />);
  const user = userEvent.setup();
  const enabled = screen.getAllByRole("checkbox")[0] as HTMLInputElement;
  await user.click(enabled);
  assert.equal(enabled.checked, true);
  assert.ok(screen.getByText("有未保存的修改"));
  view.rerender(<AutomationSettingsCard settings={{ ...settings }} control={control} onRefresh={() => Promise.resolve({ automation_settings: { ...settings, enabled: true } } as never)} />);
  assert.equal((screen.getAllByRole("checkbox")[0] as HTMLInputElement).checked, true);
  await user.click(screen.getByRole("button", { name: /保存自动计时/ }));
  await waitFor(() => assert.equal(saved?.enabled, true));
  await waitFor(() => assert.equal(screen.queryByText("有未保存的修改"), null));
  assert.ok(screen.getByRole("status").textContent?.includes("已保存"));
});

test("automation save failure keeps the draft dirty for retry", async () => {
  const control = controlAdapter({ saveAutomationSettings: async () => ({ ok: false as const, error_kind: "unavailable" as const }) });
  render(<AutomationSettingsCard settings={settings} control={control} />);
  const user = userEvent.setup();
  await user.click(screen.getAllByRole("checkbox")[0]);
  await user.click(screen.getByRole("button", { name: /保存自动计时/ }));
  await waitFor(() => assert.ok(screen.getAllByRole("status").some(item => item.textContent?.includes("暂时无法保存"))));
  assert.ok(screen.getByText("有未保存的修改"));
  assert.equal((screen.getAllByRole("checkbox")[0] as HTMLInputElement).checked, true);
});

test("successful save keeps the draft dirty when refresh still returns old settings", async () => {
  let saves = 0;
  const control = controlAdapter({ saveAutomationSettings: async () => { saves += 1; return { ok: true }; } });
  render(<AutomationSettingsCard settings={settings} control={control} onRefresh={() => Promise.resolve({ automation_settings: settings } as never)} />);
  const user = userEvent.setup();
  await user.click(screen.getAllByRole("checkbox")[0]);
  await user.click(screen.getByRole("button", { name: /保存自动计时/ }));
  await waitFor(() => assert.equal(saves, 1));
  assert.ok(screen.getByText("有未保存的修改"));
  assert.ok(screen.getAllByRole("status").some(item => item.textContent?.includes("等待后台配置确认")));
  assert.equal(screen.queryByText("自动学习计时设置已保存"), null);
});

test("a later canonical refresh clears dirty after a previously stale refresh", async () => {
  const control = controlAdapter({ saveAutomationSettings: async () => ({ ok: true }) });
  const view = render(<AutomationSettingsCard settings={settings} control={control} onRefresh={() => Promise.resolve({ automation_settings: settings } as never)} />);
  const user = userEvent.setup();
  await user.click(screen.getAllByRole("checkbox")[0]);
  await user.click(screen.getByRole("button", { name: /保存自动计时/ }));
  await waitFor(() => assert.ok(screen.getByText("有未保存的修改")));
  const canonical = { ...settings, enabled: true };
  view.rerender(<AutomationSettingsCard settings={canonical} control={control} onRefresh={() => Promise.resolve({ automation_settings: canonical } as never)} />);
  await waitFor(() => assert.equal(screen.queryByText("有未保存的修改"), null));
  assert.ok(screen.getAllByRole("status").some(item => item.textContent?.includes("已保存")));
});

test("a late canonical refresh cannot overwrite a newer user edit", async () => {
  const control = controlAdapter({ saveAutomationSettings: async () => ({ ok: true }) });
  const view = render(<AutomationSettingsCard settings={settings} control={control} onRefresh={() => Promise.resolve({ automation_settings: settings } as never)} />);
  const user = userEvent.setup();
  await user.click(screen.getAllByRole("checkbox")[0]);
  await user.click(screen.getByRole("button", { name: /保存自动计时/ }));
  await waitFor(() => assert.ok(screen.getByText("有未保存的修改")));
  const numberInput = screen.getByDisplayValue("45") as HTMLInputElement;
  await user.clear(numberInput);
  await user.type(numberInput, "120");
  const lateCanonical = { ...settings, enabled: true };
  view.rerender(<AutomationSettingsCard settings={lateCanonical} control={control} onRefresh={() => Promise.resolve({ automation_settings: lateCanonical } as never)} />);
  assert.equal((screen.getByDisplayValue("120") as HTMLInputElement).value, "120");
  assert.ok(screen.getByText("有未保存的修改"));
});

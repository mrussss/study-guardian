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
  auto_start: { enabled: true, focused_stable_seconds: 90, unclassified_stable_seconds: 180, evidence_grace_seconds: 20, min_confidence: .8, allow_unclassified: true, confirm: false },
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


test("turning automation off saves immediately and stays off after remount", async () => {
 let canonical={...settings,enabled:true};let writes=0;
 const control=controlAdapter({saveAutomationSettings:async value=>{canonical=value;writes++;return {ok:true}}});
 const refresh=async()=>({automation_settings:canonical} as never);
 const view=render(<AutomationSettingsCard settings={canonical} control={control} onRefresh={refresh}/>);
 await userEvent.setup().click(screen.getAllByRole("checkbox")[0]);
 await waitFor(()=>assert.equal(writes,1));
 await waitFor(()=>assert.equal(screen.queryByText("有未保存的修改"),null));
 assert.equal(canonical.enabled,false);view.unmount();
 render(<AutomationSettingsCard settings={canonical} control={control} onRefresh={refresh}/>);
 assert.equal((screen.getAllByRole("checkbox")[0] as HTMLInputElement).checked,false);
});
test("failed immediate save retains draft and reports failure",async()=>{
 render(<AutomationSettingsCard settings={settings} control={controlAdapter({saveAutomationSettings:async()=>({ok:false,error_kind:"unavailable"})})}/>);
 await userEvent.setup().click(screen.getAllByRole("checkbox")[0]);
 await waitFor(()=>assert.ok(screen.getAllByRole("status").some(x=>x.textContent?.includes("暂时无法保存"))));
 assert.ok(screen.getByText("有未保存的修改"));
});
test("late stale source cannot undo a switch awaiting confirmation",async()=>{
 const control=controlAdapter({saveAutomationSettings:async()=>({ok:true})});
 const view=render(<AutomationSettingsCard settings={settings} control={control} onRefresh={async()=>({automation_settings:settings} as never)}/>);
 await userEvent.setup().click(screen.getAllByRole("checkbox")[0]);
 await waitFor(()=>assert.ok(screen.getAllByRole("status").some(x=>x.textContent?.includes("等待后台配置确认"))));
 view.rerender(<AutomationSettingsCard settings={{...settings}} control={control}/>);
 assert.equal((screen.getAllByRole("checkbox")[0] as HTMLInputElement).checked,true);
 view.rerender(<AutomationSettingsCard settings={{...settings,enabled:true}} control={control}/>);
 await waitFor(()=>assert.equal(screen.queryByText("有未保存的修改"),null));
});
test("numeric edits remain explicit drafts until saved",async()=>{
 let saved:NativeAutomationSettings|undefined;
 const control=controlAdapter({saveAutomationSettings:async value=>{saved=value;return {ok:true}}});
 render(<AutomationSettingsCard settings={settings} control={control} onRefresh={async()=>({automation_settings:saved} as never)}/>);
 const user=userEvent.setup();const input=screen.getByDisplayValue("45");
 await user.clear(input);await user.type(input,"120");assert.equal(saved,undefined);
 await user.click(screen.getByRole("button",{name:/保存自动计时/}));
 await waitFor(()=>assert.equal(saved?.auto_resume.focused_stable_seconds,120));
});

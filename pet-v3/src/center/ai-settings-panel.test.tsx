import { JSDOM } from "jsdom";
import { strict as assert } from "node:assert";
import test, { afterEach } from "node:test";
import { AISettingsPanel } from "./App";
import type { NativeAISettings, SupervisorControlAdapter } from "../transport/supervisor";

const dom = new JSDOM("<!doctype html><html><body></body></html>", { url: "http://localhost" });
Object.defineProperty(globalThis, "window", { value: dom.window, configurable: true });
Object.defineProperty(globalThis, "document", { value: dom.window.document, configurable: true });
Object.defineProperty(globalThis, "navigator", { value: dom.window.navigator, configurable: true });
Object.defineProperty(globalThis, "HTMLElement", { value: dom.window.HTMLElement, configurable: true });
Object.defineProperty(globalThis, "Node", { value: dom.window.Node, configurable: true });
Object.defineProperty(globalThis, "MutationObserver", { value: dom.window.MutationObserver, configurable: true });
Object.defineProperty(globalThis, "getComputedStyle", { value: dom.window.getComputedStyle.bind(dom.window), configurable: true });
(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

const { cleanup, render, screen, waitFor, within } = await import("@testing-library/react");
const userEvent = (await import("@testing-library/user-event")).default;

afterEach(() => cleanup());

type Deferred<T> = { promise: Promise<T>; resolve: (value: T) => void };
function deferred<T>(): Deferred<T> {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>(nextResolve => { resolve = nextResolve; });
  return { promise, resolve };
}

const initialSettings: NativeAISettings = {
  enabled: false,
  min_confidence: .75,
  proxy: { mode: "environment", url: "" },
  text: { enabled: false, provider: "none", model: "", fallback_models: [], base_url: "", api_key_configured: false, timeout_seconds: 6, json_mode: "auto" },
  vision: { enabled: false, provider: "none", model: "", fallback_models: [], base_url: "", api_key_configured: false, timeout_seconds: 8, json_mode: "auto" },
};

function controlAdapter(overrides: Partial<SupervisorControlAdapter> = {}): SupervisorControlAdapter {
  const ok = async () => ({ ok: true as const });
  return {
    setModeStudy: ok, setModeBreak: ok, setModeOff: ok, setTask: ok,
    createTaskPreset: ok, selectTaskPreset: ok, updateTaskPreset: ok, deleteTaskPreset: ok,
    setReminderSettings: ok, saveAISettings: ok, putAISecret: ok, deleteAISecret: ok,
    testAIConnection: async () => ({ ok: true, provider: "aihubmix", model: "coding-glm-5.3-free", latency_ms: 42 }),
    testAIProxy: async () => ({ ok: true, mode: "environment", latency_ms: 42 }),
    generateReview: ok, setDailyTarget: ok, createMission: ok, completeMission: ok, cancelMission: ok,
    ...overrides,
  } as SupervisorControlAdapter;
}

test("saving a Key persists the visible endpoint first and immediately shows configured", async () => {
  const calls: string[] = [];
  let savedSettings: NativeAISettings | undefined;
  const refresh = deferred<void>();
  const control = controlAdapter({
    saveAISettings: async settings => { calls.push("settings"); savedSettings = settings; return { ok: true }; },
    putAISecret: async (target, key) => { calls.push(`secret:${target}:${key}`); return { ok: true }; },
  });
  const view = render(<AISettingsPanel settings={initialSettings} control={control} onRefresh={() => refresh.promise} />);
  const user = userEvent.setup();
  await user.selectOptions(screen.getAllByLabelText("服务商")[0], "aihubmix");
  await user.type(screen.getAllByPlaceholderText("输入新 Key（不会回显）")[0], "test-secret");
  await user.click(screen.getAllByRole("button", { name: "保存 Key" })[0]);

  await waitFor(() => assert.deepEqual(calls, ["settings", "secret:text:test-secret"]));
  assert.equal(savedSettings?.text.provider, "aihubmix");
  assert.equal(savedSettings?.text.model, "coding-glm-5.3-free");
  assert.equal(savedSettings?.text.timeout_seconds, 20);
  assert.ok(screen.getAllByText("API Key 已配置").length >= 1);
  assert.equal((screen.getAllByPlaceholderText("输入新 Key（不会回显）")[0] as HTMLInputElement).value, "");

  const canonical = { ...savedSettings!, text: { ...savedSettings!.text, api_key_configured: true } };
  view.rerender(<AISettingsPanel settings={canonical} control={control} onRefresh={() => refresh.promise} />);
  refresh.resolve();
  await waitFor(() => assert.equal(screen.getByRole("status").textContent, "文字 API Key 已安全保存"));
});

test("testing a connection persists the visible draft before calling the provider", async () => {
  const calls: string[] = [];
  const control = controlAdapter({
    saveAISettings: async settings => { calls.push(`settings:${settings.text.provider}`); return { ok: true }; },
    testAIConnection: async target => { calls.push(`test:${target}`); return { ok: true, provider: "aihubmix", model: "coding-glm-5.3-free", latency_ms: 42 }; },
  });
  render(<AISettingsPanel settings={initialSettings} control={control} />);
  const user = userEvent.setup();
  await user.selectOptions(screen.getAllByLabelText("服务商")[0], "aihubmix");
  await user.click(screen.getAllByRole("button", { name: "测试连接" })[0]);
  await waitFor(() => assert.deepEqual(calls, ["settings:aihubmix", "test:text"]));
  assert.match(screen.getByRole("status").textContent ?? "", /连接正常.*coding-glm-5\.3-free/);
});

test("a rejected draft is not followed by a secret write and keeps the Key for retry", async () => {
  let secretCalls = 0;
  const control = controlAdapter({
    saveAISettings: async () => ({ ok: false, error_kind: "rejected" }),
    putAISecret: async () => { secretCalls += 1; return { ok: true }; },
  });
  render(<AISettingsPanel settings={initialSettings} control={control} />);
  const user = userEvent.setup();
  const keyInput = screen.getAllByPlaceholderText("输入新 Key（不会回显）")[0] as HTMLInputElement;
  await user.type(keyInput, "retry-secret");
  await user.click(screen.getAllByRole("button", { name: "保存 Key" })[0]);
  await waitFor(() => assert.equal(screen.getByRole("status").textContent, "AI 设置未通过验证或暂时无法保存"));
  assert.equal(secretCalls, 0);
  assert.equal(keyInput.value, "retry-secret");
});

test("a provider rate limit is explained instead of reported as an invalid response", async () => {
  const configured = { ...initialSettings, text: { ...initialSettings.text, enabled: true, provider: "aihubmix", model: "coding-glm-5.3-free", api_key_configured: true } };
  const control = controlAdapter({
    testAIConnection: async () => ({ ok: false, provider: "aihubmix", model: "coding-glm-5.3-free", latency_ms: 100, error_kind: "model_rate_limited" }),
  });
  render(<AISettingsPanel settings={configured} control={control} />);
  await userEvent.setup().click(screen.getAllByRole("button", { name: "测试连接" })[0]);
  await waitFor(() => assert.equal(screen.getByRole("status").textContent, "连接失败 · 当前模型限流，请稍后重试"));
});

test("proxy test saves the selected draft before testing and supports manual mode", async () => {
  const calls: string[] = [];
  const control = controlAdapter({
    saveAISettings: async settings => { calls.push(`save:${settings.proxy.mode}:${settings.proxy.url}`); return { ok: true }; },
    testAIProxy: async () => { calls.push("proxy-test"); return { ok: true, mode: "manual", latency_ms: 17 }; },
  });
  render(<AISettingsPanel settings={initialSettings} control={control} />);
  const user = userEvent.setup();
  await user.selectOptions(screen.getByLabelText("代理模式"), "manual");
  await user.type(screen.getByPlaceholderText("http://127.0.0.1:7890"), "http://127.0.0.1:7890");
  await user.click(screen.getByRole("button", { name: "测试网络" }));
  await waitFor(() => assert.deepEqual(calls, ["save:manual:http://127.0.0.1:7890", "proxy-test"]));
  const networkCard = screen.getByText("网络连接").closest(".ai-network-card") as HTMLElement;
  assert.ok(networkCard);
  assert.match(within(networkCard).getByRole("status").textContent ?? "", /网络可达.*17ms/);
});

test("fallback model editor keeps punctuation while typing and parses supported separators on save", async () => {
  let saved: NativeAISettings | undefined;
  const control = controlAdapter({ saveAISettings: async settings => { saved = settings; return { ok: true }; } });
  render(<AISettingsPanel settings={initialSettings} control={control} />);
  const user = userEvent.setup();
  const input = screen.getAllByRole("textbox", { name: "备用模型（按顺序）" })[0] as HTMLTextAreaElement;

  await user.type(input, "model-one, ");
  assert.equal(input.value, "model-one, ");
  await user.type(input, "model-two，model-one; model-three\n");
  assert.equal(input.value, "model-one, model-two，model-one; model-three\n");
  await user.click(screen.getByRole("button", { name: "保存并应用 AI 设置" }));

  await waitFor(() => assert.ok(saved));
  assert.deepEqual(saved?.text.fallback_models, ["model-one", "model-two", "model-three"]);
});

test("fallback model editor rejects more than three models instead of truncating", async () => {
  let saveCalls = 0;
  const control = controlAdapter({ saveAISettings: async () => { saveCalls += 1; return { ok: true }; } });
  render(<AISettingsPanel settings={initialSettings} control={control} />);
  const user = userEvent.setup();
  const input = screen.getAllByRole("textbox", { name: "备用模型（按顺序）" })[0] as HTMLTextAreaElement;
  await user.type(input, "one,two,three,four");
  await user.click(screen.getByRole("button", { name: "保存并应用 AI 设置" }));

  await waitFor(() => assert.match(screen.getByRole("alert").textContent ?? "", /最多只能设置 3 个/));
  assert.equal(saveCalls, 0);
  assert.equal(input.value, "one,two,three,four");
});

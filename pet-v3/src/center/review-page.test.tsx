import { JSDOM } from "jsdom";
import { strict as assert } from "node:assert";
import test, { afterEach } from "node:test";
import { ReviewPage } from "./App";
import type { NativeReviewSummary, SupervisorControlAdapter } from "../transport/supervisor";

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

const review: NativeReviewSummary = {
  schema_version: 1,
  date: "2026-09-07",
  headline: "今天完成了一小步",
  topics: [{ name: "Go", summary: "复习 context", confidence: .8 }],
  accomplishments: [],
  unfinished: ["整理笔记"],
  difficulties: [],
  behavior: { distraction_count: 0, largest_distraction_seconds: 0, average_recovery_seconds: 0 },
  tomorrow_priority: "继续练习",
  warnings: [],
  status: "STALE",
  generation_mode: "FALLBACK",
  provider: "",
  model: "",
  revision: 1,
  attempt_count: 1,
  error_code: "timeout",
  warnings_count: 0,
};

function control(overrides: Partial<SupervisorControlAdapter> = {}): SupervisorControlAdapter {
  return {
    setModeStudy: async () => ({ ok: true }), setModeBreak: async () => ({ ok: true }), setModeOff: async () => ({ ok: true }), setTask: async () => ({ ok: true }),
    createTaskPreset: async () => ({ ok: true }), selectTaskPreset: async () => ({ ok: true }), updateTaskPreset: async () => ({ ok: true }), deleteTaskPreset: async () => ({ ok: true }),
    setReminderSettings: async () => ({ ok: true }), saveAISettings: async () => ({ ok: true }), putAISecret: async () => ({ ok: true }), deleteAISecret: async () => ({ ok: true }),
    testAIConnection: async () => ({ ok: true, provider: "test", model: "test", latency_ms: 1 }), testAIProxy: async () => ({ ok: true, mode: "direct", latency_ms: 1 }),
    startReviewGeneration: async () => ({ accepted: true, date: review.date, state: "PENDING", generation_id: "mock-generation" }),
    getReviewGenerationStatus: async () => ({ date: review.date, state: "READY", generation_id: "mock-generation", generation_mode: "AI" }),
    generateReview: async () => ({ ok: true, status: "READY", generation_mode: "AI" }), setDailyTarget: async () => ({ ok: true }), createMission: async () => ({ ok: true }),
    completeMission: async () => ({ ok: true }), cancelMission: async () => ({ ok: true }), ...overrides,
  };
}

test("STALE review remains visible and immediate generation refreshes with fallback status", async () => {
  let refreshes = 0;
  render(<ReviewPage review={review} onRefresh={async () => { refreshes += 1; }} control={control({
    startReviewGeneration: async () => ({ accepted: true, date: review.date, state: "PENDING", generation_id: "fallback-generation" }),
    getReviewGenerationStatus: async () => ({ date: review.date, state: "READY", generation_id: "fallback-generation", generation_mode: "FALLBACK", error_kind: "timeout" }),
  })} pollIntervalMs={5} />);

  assert.ok(screen.getByText("今天完成了一小步"));
  assert.ok(screen.getByText("生成后又有新的学习记录，当前内容仍可查看。"));
  const button = screen.getByRole("button", { name: "更新今日总结" });
  await userEvent.setup().click(button);
  await waitFor(() => assert.equal(refreshes, 1));
  await waitFor(() => assert.equal(screen.getByRole("status").textContent, "AI 响应超时，已生成本地总结"));
  assert.match(button.className, /primary-button/);
});

test("review generation failure is not styled as a success", async () => {
  render(<ReviewPage control={control({ startReviewGeneration: async () => ({ state: "FAILED", error_kind: "unavailable" }) })} />);
  await userEvent.setup().click(screen.getByRole("button", { name: "立即生成" }));
  await waitFor(() => assert.equal(screen.getByRole("status").textContent, "今日总结暂时无法生成"));
  assert.match(screen.getByRole("status").className, /is-warning/);
});

test("generation stays pending while polling and ignores a stale generation id", async () => {
  let statusCalls = 0;
  const controlAdapter = control({
    startReviewGeneration: async () => ({ accepted: true, date: review.date, state: "PENDING", generation_id: "current" }),
    getReviewGenerationStatus: async () => {
      statusCalls += 1;
      return statusCalls === 1
        ? { date: review.date, state: "PENDING", generation_id: "stale" }
        : { date: review.date, state: "READY", generation_id: "current", generation_mode: "AI" };
    },
  });
  render(<ReviewPage control={controlAdapter} pollIntervalMs={5} />);
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "立即生成" }));
  assert.equal((screen.getByRole("button", { name: "正在生成…" }) as HTMLButtonElement).disabled, true);
  await waitFor(() => assert.equal(screen.getByRole("status").textContent, "AI 总结已生成"));
  assert.ok(statusCalls >= 2);
});

test("local wait deadline makes one final status query and refreshes before yielding", async () => {
  let statusCalls = 0;
  let refreshes = 0;
  const controlAdapter = control({
    startReviewGeneration: async () => ({ accepted: true, date: review.date, state: "PENDING", generation_id: "slow" }),
    getReviewGenerationStatus: async () => { statusCalls += 1; return { date: review.date, state: "PENDING", generation_id: "slow" }; },
  });
  render(<ReviewPage control={controlAdapter} onRefresh={async () => { refreshes += 1; }} pollIntervalMs={5} maxWaitMs={10} />);
  await userEvent.setup().click(screen.getByRole("button", { name: "立即生成" }));
  await waitFor(() => assert.equal(screen.getByRole("status").textContent, "正在生成，你可以继续使用 StudyGuardian"));
  assert.equal(refreshes, 1);
  assert.ok(statusCalls >= 2);
});

test("unmount stops daily review polling", async () => {
  let statusCalls = 0;
  const controlAdapter = control({
    startReviewGeneration: async () => ({ accepted: true, date: review.date, state: "PENDING", generation_id: "unmounted" }),
    getReviewGenerationStatus: async () => { statusCalls += 1; return { date: review.date, state: "PENDING", generation_id: "unmounted" }; },
  });
  const view = render(<ReviewPage control={controlAdapter} pollIntervalMs={5} maxWaitMs={1000} />);
  await userEvent.setup().click(screen.getByRole("button", { name: "立即生成" }));
  await waitFor(() => assert.ok(statusCalls >= 1));
  view.unmount();
  const callsAtUnmount = statusCalls;
  await new Promise(resolve => setTimeout(resolve, 20));
  assert.equal(statusCalls, callsAtUnmount);
});

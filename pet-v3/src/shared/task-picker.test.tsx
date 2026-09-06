import { JSDOM } from "jsdom";
import { strict as assert } from "node:assert";
import test, { afterEach } from "node:test";
import { useState, type ReactElement } from "react";
import { TaskPicker, type TaskPickerActionResult } from "./TaskPicker";
import type { NativeTaskPreset, NativeTaskPresetList } from "../transport/supervisor";

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

type Deferred<T> = { promise: Promise<T>; resolve: (value: T) => void; reject: (error: unknown) => void };
function deferred<T>(): Deferred<T> {
  let resolve!: (value: T) => void;
  let reject!: (error: unknown) => void;
  const promise = new Promise<T>((nextResolve, nextReject) => { resolve = nextResolve; reject = nextReject; });
  return { promise, resolve, reject };
}

function preset(id: string, name: string): NativeTaskPreset {
  return { id, name, pinned: true, sort_order: 0, use_count: 1 };
}

const presets: NativeTaskPresetList = {
  pinned: [preset("go", "Go"), preset("ba", "八股"), preset("algo", "算法")],
  recent: [],
};

type HarnessProps = {
  initialTask?: string;
  onSelect?: (id: string) => Promise<TaskPickerActionResult>;
  variant?: "default" | "hero";
};

function PickerHarness({ initialTask = "八股", onSelect = async () => ({ ok: true }), variant = "hero" }: HarnessProps): ReactElement {
  const [optimisticTask, setOptimisticTask] = useState<string>();
  const [notice, setNotice] = useState("");
  const currentTask = optimisticTask ?? initialTask;
  return <>
    <div data-testid="hero-current-task">{currentTask}</div>
    <TaskPicker
      currentTask={currentTask}
      presets={presets}
      variant={variant}
      onOptimisticTaskChange={setOptimisticTask}
      onResult={result => { if (!result.ok) setNotice("当前任务暂时无法更新"); }}
      onSelect={onSelect}
      onTemporary={async () => ({ ok: true })}
      onSavePinned={async () => ({ ok: true })}
    />
    {notice && <div role="status">{notice}</div>}
  </>;
}

test("clicking a task immediately updates the Hero task and selected chip", async () => {
  const request = deferred<TaskPickerActionResult>();
  render(<PickerHarness onSelect={async () => request.promise} />);
  const algorithm = screen.getByRole("button", { name: "算法" });
  await userEvent.setup().click(algorithm);
  assert.equal(screen.getByTestId("hero-current-task").textContent, "算法");
  assert.equal(algorithm.classList.contains("is-active"), true);
  assert.equal(algorithm.getAttribute("aria-pressed"), "true");
  assert.equal(algorithm.getAttribute("aria-busy"), "true");
  request.resolve({ ok: true });
  await waitFor(() => assert.equal(algorithm.getAttribute("aria-busy"), "false"));
});

test("clicking the current task is a no-op", async () => {
  let calls = 0;
  render(<PickerHarness onSelect={async () => { calls += 1; return { ok: true }; }} />);
  const current = screen.getByRole("button", { name: "八股" });
  await userEvent.setup().click(current);
  assert.equal(calls, 0);
  assert.equal(current.getAttribute("aria-pressed"), "true");
  assert.equal(current.getAttribute("aria-busy"), "false");
});

test("only the selected task is pending without disabling the chip", async () => {
  const request = deferred<TaskPickerActionResult>();
  render(<PickerHarness onSelect={async () => request.promise} />);
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "算法" }));
  const go = screen.getByRole("button", { name: "Go" }) as HTMLButtonElement;
  const ba = screen.getByRole("button", { name: "八股" }) as HTMLButtonElement;
  const algorithm = screen.getByRole("button", { name: "算法" }) as HTMLButtonElement;
  assert.equal(algorithm.disabled, false);
  assert.equal(go.disabled, false);
  assert.equal(ba.disabled, false);
  assert.equal(go.getAttribute("aria-busy"), "false");
  assert.equal(ba.getAttribute("aria-busy"), "false");
  request.resolve({ ok: true });
  await waitFor(() => assert.equal(algorithm.disabled, false));
});

test("rapid Go to 八股 to 算法 clicks coalesce pending work and keep 算法", async () => {
  const first = deferred<TaskPickerActionResult>();
  const calls: string[] = [];
  const onSelect = async (id: string): Promise<TaskPickerActionResult> => {
    calls.push(id);
    return calls.length === 1 ? first.promise : { ok: true };
  };
  render(<PickerHarness onSelect={onSelect} />);
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "Go" }));
  await user.click(screen.getByRole("button", { name: "八股" }));
  await user.click(screen.getByRole("button", { name: "算法" }));
  assert.equal(screen.getByTestId("hero-current-task").textContent, "算法");
  assert.deepEqual(calls, ["go"]);
  first.resolve({ ok: true });
  await waitFor(() => assert.deepEqual(calls, ["go", "algo"]));
  assert.equal(screen.getByTestId("hero-current-task").textContent, "算法");
  assert.equal(screen.getByRole("button", { name: "算法" }).classList.contains("is-active"), true);
});

test("a failed request restores the authoritative task and shows an error", async () => {
  const request = deferred<TaskPickerActionResult>();
  render(<PickerHarness onSelect={async () => request.promise} />);
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "算法" }));
  assert.equal(screen.getByTestId("hero-current-task").textContent, "算法");
  request.resolve({ ok: false, error_kind: "unavailable" });
  await waitFor(() => {
    assert.equal(screen.getByTestId("hero-current-task").textContent, "八股");
    assert.equal(screen.getByRole("status").textContent, "当前任务暂时无法更新");
  });
});

test("two failed requests return to the original authoritative task", async () => {
  const first = deferred<TaskPickerActionResult>();
  const second = deferred<TaskPickerActionResult>();
  const calls: string[] = [];
  const onSelect = async (id: string): Promise<TaskPickerActionResult> => {
    calls.push(id);
    return calls.length === 1 ? first.promise : second.promise;
  };
  render(<PickerHarness onSelect={onSelect} />);
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "Go" }));
  await user.click(screen.getByRole("button", { name: "算法" }));
  assert.equal(screen.getByTestId("hero-current-task").textContent, "算法");
  first.resolve({ ok: false, error_kind: "unavailable" });
  await waitFor(() => assert.deepEqual(calls, ["go", "algo"]));
  second.resolve({ ok: false, error_kind: "unavailable" });
  await waitFor(() => assert.equal(screen.getByTestId("hero-current-task").textContent, "八股"));
  assert.equal(screen.getByRole("status").textContent, "当前任务暂时无法更新");
});

test("Hero hides the duplicate current-task block but keeps task controls", () => {
  const { container } = render(<PickerHarness />);
  const picker = container.querySelector(".task-picker");
  assert.ok(picker?.classList.contains("is-hero"));
  assert.equal(picker?.querySelector(".task-picker-current"), null);
  assert.ok(screen.getByRole("button", { name: "算法" }));
  assert.ok(screen.getByRole("button", { name: "新建学习任务" }));
});

test("default TaskPicker keeps its light current-task block", () => {
  const { container } = render(<PickerHarness variant="default" />);
  const picker = container.querySelector(".task-picker");
  assert.ok(picker);
  assert.equal(picker?.classList.contains("is-hero"), false);
  assert.equal(picker?.querySelector(".task-picker-current strong")?.textContent, "八股");
});

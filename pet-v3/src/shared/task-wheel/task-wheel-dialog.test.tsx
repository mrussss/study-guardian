import { JSDOM } from "jsdom";
import { strict as assert } from "node:assert";
import test, { afterEach } from "node:test";
import { TaskWheelDialog } from "./TaskWheelDialog";

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

function preset(id: string, name: string) {
  return { id, name, pinned: true, sort_order: 0, use_count: 1 };
}

function renderWheel(onSelect: (id: string) => Promise<{ ok: boolean }> = async () => ({ ok: true })) {
  return render(<TaskWheelDialog
    open
    onClose={() => undefined}
    currentTask="Go"
    presets={{ pinned: [preset("go", "Go"), preset("ba", "八股")], recent: [] }}
    onSelect={onSelect}
    onTemporary={async () => ({ ok: true })}
    onSavePinned={async () => ({ ok: true })}
  />);
}

function sectors(): NodeListOf<SVGPathElement> {
  return screen.getByLabelText("常用任务轮盘").querySelectorAll<SVGPathElement>(".task-wheel-sector");
}

test("pointer highlight covers the sector and leaving restores the current task", () => {
  renderWheel();
  const stage = screen.getByLabelText("常用任务轮盘");
  const paths = sectors();
  fireEvent.pointerEnter(paths[1]);
  assert.equal(paths[1].classList.contains("is-highlighted"), true);
  fireEvent.pointerLeave(stage);
  assert.equal(paths[0].classList.contains("is-highlighted"), true);
  assert.equal(paths[1].classList.contains("is-highlighted"), false);
});

test("clicking a blank sector selects its task, while current and empty sectors are no-ops", async () => {
  const calls: string[] = [];
  renderWheel(async id => { calls.push(id); return { ok: true }; });
  const paths = sectors();
  fireEvent.click(paths[0]);
  fireEvent.click(paths[2]);
  fireEvent.click(paths[1]);
  await waitFor(() => assert.deepEqual(calls, ["ba"]));
});

test("clicking the add sector opens the editor without selecting a task or showing directions", () => {
  const calls: string[] = [];
  renderWheel(async id => { calls.push(id); return { ok: true }; });
  fireEvent.pointerEnter(sectors()[5]);
  assert.equal(sectors()[5].classList.contains("is-highlighted"), true);
  fireEvent.click(sectors()[5]);
  assert.ok(screen.getByPlaceholderText("例如：Go / 算法 / 阅读"));
  assert.deepEqual(calls, []);
  assert.equal(screen.queryByText("方向键选择 · Enter 确认"), null);
});

test("keyboard navigation and Enter still select the highlighted task", async () => {
  const calls: string[] = [];
  renderWheel(async id => { calls.push(id); return { ok: true }; });
  fireEvent.keyDown(window, { key: "ArrowRight" });
  assert.equal(sectors()[1].classList.contains("is-highlighted"), true);
  fireEvent.keyDown(window, { key: "Enter" });
  await waitFor(() => assert.deepEqual(calls, ["ba"]));
});

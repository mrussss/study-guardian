import { strict as assert } from "node:assert";
import test, { afterEach } from "node:test";
import { JSDOM } from "jsdom";
import { createElement, type ReactElement } from "react";
import { useTaskSelectionState } from "./use-task-selection-state";
import { taskSelectionReducer, type TaskSelectionState } from "./use-task-selection-state";

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

const initial: TaskSelectionState = { authoritativeTask: "Go", pending: false };

test("snapshot cannot overwrite the latest optimistic task before confirmation", () => {
  const optimistic = taskSelectionReducer(initial, { type: "OPTIMISTIC_SELECTED", task: "算法" });
  const stale = taskSelectionReducer(optimistic, { type: "SNAPSHOT_RECEIVED", task: "Go" });
  assert.equal(stale.optimisticTask, "算法");
  assert.equal(stale.pending, true);
  const confirmed = taskSelectionReducer(stale, { type: "SNAPSHOT_RECEIVED", task: "算法" });
  assert.deepEqual(confirmed, { authoritativeTask: "算法", pending: false });
});

test("success commits atomically and failure restores the last authoritative task", () => {
  const optimistic = taskSelectionReducer(initial, { type: "OPTIMISTIC_SELECTED", task: "八股" });
  assert.deepEqual(taskSelectionReducer(optimistic, { type: "MUTATION_SUCCEEDED", task: "八股" }), { authoritativeTask: "八股", pending: false });
  const confirmedWithoutEcho = taskSelectionReducer(optimistic, { type: "MUTATION_SUCCEEDED" });
  assert.deepEqual(confirmedWithoutEcho, { authoritativeTask: "八股", pending: false });
  assert.deepEqual(taskSelectionReducer(confirmedWithoutEcho, { type: "SNAPSHOT_RECEIVED", task: "算法" }), { authoritativeTask: "算法", pending: false });
  assert.deepEqual(taskSelectionReducer(optimistic, { type: "MUTATION_FAILED" }), { authoritativeTask: "Go", pending: false });
});

function ParentHarness({ snapshotTask }: { snapshotTask: string }): ReactElement {
  const selection = useTaskSelectionState(snapshotTask);
  return createElement("div", null,
    createElement("output", { "data-testid": "selected-task" }, selection.task),
    createElement("button", { type: "button", onClick: () => selection.selectOptimistically("算法") }, "选择算法"),
  );
}

test("the parent hook keeps a new selection through a stale dashboard snapshot", async () => {
  const view = render(createElement(ParentHarness, { snapshotTask: "Go" }));
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "选择算法" }));
  assert.equal(screen.getByTestId("selected-task").textContent, "算法");
  view.rerender(createElement(ParentHarness, { snapshotTask: "Go" }));
  assert.equal(screen.getByTestId("selected-task").textContent, "算法");
  view.rerender(createElement(ParentHarness, { snapshotTask: "算法" }));
  await waitFor(() => assert.equal(screen.getByTestId("selected-task").textContent, "算法"));
});

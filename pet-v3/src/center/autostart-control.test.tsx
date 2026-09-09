import { JSDOM } from "jsdom";
import { strict as assert } from "node:assert";
import test, { afterEach } from "node:test";
import { AutostartControl } from "./App";
import type { SystemIntegrationAdapter } from "../transport/supervisor";

const dom = new JSDOM("<!doctype html><html><body></body></html>", { url: "http://localhost" });
Object.defineProperty(globalThis, "window", { value: dom.window, configurable: true });
Object.defineProperty(globalThis, "document", { value: dom.window.document, configurable: true });
Object.defineProperty(globalThis, "navigator", { value: dom.window.navigator, configurable: true });
Object.defineProperty(globalThis, "HTMLElement", { value: dom.window.HTMLElement, configurable: true });
Object.defineProperty(globalThis, "Node", { value: dom.window.Node, configurable: true });
Object.defineProperty(globalThis, "MutationObserver", { value: dom.window.MutationObserver, configurable: true });
(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

const { cleanup, render, screen, waitFor } = await import("@testing-library/react");
const userEvent = (await import("@testing-library/user-event")).default;
afterEach(() => cleanup());

function deferred<T>(): { promise: Promise<T>; resolve: (value: T) => void } {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>(next => { resolve = next; });
  return { promise, resolve };
}

test("keeps the autostart switch disabled until the initial canonical read completes", async () => {
  const initial = deferred<{ enabled: boolean; available: boolean }>();
  let writes = 0;
  const adapter: SystemIntegrationAdapter = {
    getAutostartState: () => initial.promise,
    setAutostartEnabled: async () => { writes += 1; return { enabled: true, available: true }; },
  };
  render(<AutostartControl adapter={adapter} />);
  const checkbox = screen.getByRole("checkbox") as HTMLInputElement;
  assert.equal(checkbox.disabled, true);
  assert.equal(screen.getByText("正在读取").textContent, "正在读取");
  await userEvent.setup().click(checkbox);
  assert.equal(writes, 0);
  initial.resolve({ enabled: false, available: true });
  await waitFor(() => assert.equal((screen.getByRole("checkbox") as HTMLInputElement).disabled, false));
});

test("confirms the written autostart state and restores it when canonical read mismatches", async () => {
  const canonical = deferred<{ enabled: boolean; available: boolean }>();
  let reads = 0;
  const adapter: SystemIntegrationAdapter = {
    getAutostartState: async () => { reads += 1; return reads === 1 ? { enabled: false, available: true } : canonical.promise; },
    setAutostartEnabled: async () => ({ enabled: true, available: true }),
  };
  render(<AutostartControl adapter={adapter} />);
  await waitFor(() => assert.equal((screen.getByRole("checkbox") as HTMLInputElement).disabled, false));
  await userEvent.setup().click(screen.getByRole("checkbox"));
  canonical.resolve({ enabled: false, available: true });
  await waitFor(() => assert.equal(screen.getByText("启动项写入后未能确认").textContent, "启动项写入后未能确认"));
  assert.equal((screen.getByRole("checkbox") as HTMLInputElement).checked, false);
});

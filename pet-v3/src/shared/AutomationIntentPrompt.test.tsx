import { JSDOM } from "jsdom";
import { strict as assert } from "node:assert";
import test, { afterEach } from "node:test";
import { AutomationIntentPrompt } from "./AutomationIntentPrompt";
import type { NativeAutomationIntent } from "../transport/supervisor";

const dom = new JSDOM("<!doctype html><html><body></body></html>", { url: "http://localhost" });
Object.defineProperty(globalThis, "window", { value: dom.window, configurable: true });
Object.defineProperty(globalThis, "document", { value: dom.window.document, configurable: true });
Object.defineProperty(globalThis, "navigator", { value: dom.window.navigator, configurable: true });
Object.defineProperty(globalThis, "HTMLElement", { value: dom.window.HTMLElement, configurable: true });
Object.defineProperty(globalThis, "Node", { value: dom.window.Node, configurable: true });
Object.defineProperty(globalThis, "MutationObserver", { value: dom.window.MutationObserver, configurable: true });
(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

const { cleanup, render, screen } = await import("@testing-library/react");
afterEach(() => cleanup());

function intent(transition: NativeAutomationIntent["transition"], expiresAt = Date.now() + 5000): NativeAutomationIntent {
  return {
    intent_id: "intent-1",
    transition,
    reason: transition === "AUTO_PAUSE" ? "IDLE" : "NONE",
    task: "Go",
    created_at: new Date(Date.now() - 1000).toISOString(),
    expires_at: new Date(expiresAt).toISOString(),
    requires_confirmation: true,
  };
}

test("pause prompt exposes countdown and both explicit choices", () => {
  let accepted = 0;
  let rejected = 0;
  render(<AutomationIntentPrompt pending={intent("AUTO_PAUSE")} onAccept={() => { accepted += 1; }} onReject={() => { rejected += 1; }} />);
  assert.match(screen.getByRole("alert").textContent ?? "", /检测到你可能已离开/);
  assert.match(screen.getByRole("alert").textContent ?? "", /将在 [0-9]+ 秒后暂停计时/);
  screen.getByRole("button", { name: "继续计时" }).click();
  screen.getByRole("button", { name: "立即暂停" }).click();
  assert.equal(rejected, 1);
  assert.equal(accepted, 1);
});

test("auto start expiry copy distinguishes start from pause", () => {
  render(<AutomationIntentPrompt pending={intent("AUTO_START")} />);
  assert.match(screen.getByRole("alert").textContent ?? "", /检测到你正在学习/);
  assert.match(screen.getByRole("alert").textContent ?? "", /保持待机/);
  assert.match(screen.getByRole("alert").textContent ?? "", /开始计时/);
});
test("expired prompt disables decisions and refreshes canonical automation status", async () => {
  let expired = 0;
  render(<AutomationIntentPrompt pending={intent("AUTO_PAUSE", Date.now() - 1000)} onExpired={() => { expired += 1; }} />);
  await new Promise(resolve => setTimeout(resolve, 0));
  assert.match(screen.getByRole("alert").textContent ?? "", /正在应用自动暂停/);
  assert.equal(screen.getByRole("button", { name: "继续计时" }).hasAttribute("disabled"), true);
  assert.equal(screen.getByRole("button", { name: "立即暂停" }).hasAttribute("disabled"), true);
  assert.equal(expired, 1);
});

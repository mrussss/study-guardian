import { invoke } from "@tauri-apps/api/core";

const counter = document.querySelector("#count");
const status = document.querySelector("#status");
let clicks = 0;
let requestSequence = 0;
let busy = false;

// This timeout only bounds diagnostics; it never controls native dragging.
const withTimeout = async (pending) => {
  let timer;
  try {
    return await Promise.race([
      pending,
      new Promise((_, reject) => {
        timer = window.setTimeout(() => reject(new Error("timeout")), 6000);
      }),
    ]);
  } finally {
    window.clearTimeout(timer);
  }
};

const nativeCommand = (command, args) => withTimeout(invoke(command, args));

const assertWindow = (snapshot, label, visible) => {
  const item = snapshot?.windows?.find(window => window.label === label);
  if (!item?.exists || item.visible !== visible) throw new Error("native_state");
};

const setBusy = (next) => {
  busy = next;
  for (const button of document.querySelectorAll("button")) {
    if (button !== counter) button.disabled = next;
  }
};

counter.addEventListener("click", () => {
  counter.textContent = `独立单击计数：${++clicks}`;
});

const runCheck = async (label, check) => {
  if (busy) return;
  setBusy(true);
  const sequence = ++requestSequence;
  status.textContent = `${sequence} · ${label}：等待返回`;
  try {
    const result = await check();
    status.textContent = `${sequence} · ${result}`;
  } catch (error) {
    const reason = error instanceof Error && error.message === "timeout" ? "超时，可能卡死"
      : error instanceof Error && error.message === "native_state" ? "原生状态不符" : "命令失败";
    status.textContent = `${sequence} · ${label}：${reason}`;
  } finally {
    setBusy(false);
  }
};

document.querySelector("#open-panel").addEventListener("click", () => {
  void runCheck("打开面板", async () => {
    await nativeCommand("open_quick_panel");
    assertWindow(await nativeCommand("pet_window_diagnostics"), "quick-panel", true);
    return "面板原生可见：通过";
  });
});
document.querySelector("#hide-panel").addEventListener("click", () => {
  void runCheck("隐藏面板", async () => {
    await nativeCommand("hide_quick_panel");
    const snapshot = await nativeCommand("pet_window_diagnostics");
    const panel = snapshot?.windows?.find(window => window.label === "quick-panel");
    if (!panel || panel.visible) throw new Error("native_state");
    return "面板原生隐藏：通过";
  });
});

for (const [id, route, label] of [["open-settings", "settings", "设置"], ["open-review", "review", "复盘"]]) {
  document.getElementById(id).addEventListener("click", () => {
    void runCheck(`打开${label}`, async () => {
      await nativeCommand("open_control_center", { route });
      const snapshot = await nativeCommand("pet_window_diagnostics");
      assertWindow(snapshot, "control-center", true);
      if (snapshot.control_center_route !== route) throw new Error("native_state");
      return `${label}原生路由通过；请核对实际页面`;
    });
  });
}

document.querySelector("#cycle-panel").addEventListener("click", () => {
  void runCheck("自动开关", async () => {
    for (let completed = 0; completed < 20; completed += 1) {
      await nativeCommand("open_quick_panel");
      assertWindow(await nativeCommand("pet_window_diagnostics"), "quick-panel", true);
      await nativeCommand("hide_quick_panel");
      assertWindow(await nativeCommand("pet_window_diagnostics"), "quick-panel", false);
      status.textContent = `开关面板：${completed + 1}/20，原生可见/隐藏已确认`;
    }
    return "20/20 通过：每次均确认原生可见/隐藏";
  });
});

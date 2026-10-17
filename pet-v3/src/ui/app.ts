import { invoke } from "@tauri-apps/api/core";
import { MockSupervisorRuntime, parseMockScenario } from "../mock/supervisor";
import { mockSemantic } from "../mock/semantic";
import { NativeSupervisorAdapter, SupervisorPollLoop, type PetTransportSnapshot } from "../transport/supervisor";
import { clockText } from "./clock";

export function mountApp(root: HTMLElement): void {
  const native = Object.prototype.hasOwnProperty.call(window, "__TAURI_INTERNALS__");
  root.innerHTML = `<section class="digital-clock" title="学习计时 · 按住拖动；设置请使用托盘">
    <strong class="clock-study" data-study aria-label="今日学习时间">--:--</strong>
    <div class="clock-eye" data-eye><span>护眼</span><span data-eye-time>--:--</span></div>
  </section>`;
  const shell = root.querySelector<HTMLElement>(".digital-clock")!;
  const study = root.querySelector<HTMLElement>("[data-study]")!;
  const eye = root.querySelector<HTMLElement>("[data-eye]")!;
  const eyeTime = root.querySelector<HTMLElement>("[data-eye-time]")!;
  let claimInFlight = false;
  const render = (snapshot: PetTransportSnapshot): void => {
    study.textContent = clockText(snapshot.connected ? snapshot.study_seconds : undefined);
    const current = snapshot.eye_care_status;
    const settings = snapshot.eye_care_settings;
    const paused = !snapshot.connected || snapshot.semantic.interaction !== "ACTIVE" || snapshot.semantic.user_mode !== "STUDY";
    shell.classList.toggle("is-paused", paused);
    study.title = `今日学习时间${paused ? "（已暂停）" : ""}`;
    eye.hidden = current?.enabled === false;
    let remaining: number | undefined;
    if (snapshot.connected && current && current.enabled) {
      remaining = current.phase === "FOCUSING" ? Math.max(0, (settings?.focus_minutes ?? 40) * 60 - current.focus_segment_seconds) : 0;
      if ((current.phase === "SHORT_BREAK" || current.phase === "LONG_BREAK") && current.planned_break_end_at) {
        remaining = Math.max(0, Math.ceil((Date.parse(current.planned_break_end_at) - Date.now()) / 1000));
      }
    }
    eyeTime.textContent = clockText(remaining);
    eye.title = current?.phase === "WAITING_RETURN" ? "休息完成，请在托盘中继续" : "护眼倒计时；无输入时暂停";
    if (native && snapshot.connected && current?.enabled &&
      (current.phase === "SHORT_BREAK_DUE" || current.phase === "LONG_BREAK_DUE") &&
      !current.notification_suppressed && !claimInFlight) {
      claimInFlight = true;
      void invoke("supervisor_claim_eye_care_notification").catch(() => undefined).finally(() => { claimInFlight = false; });
    }
  };
  if (native) {
    const loop = new SupervisorPollLoop(new NativeSupervisorAdapter());
    loop.start(render);
    window.addEventListener("beforeunload", () => loop.stop(), { once: true });
  } else {
    const mock = new MockSupervisorRuntime(parseMockScenario(window.location.search));
    void mock.poll().then(snapshot => render({ connected: snapshot.connected,
      semantic: snapshot.semantic ?? mockSemantic({ user_mode: snapshot.status?.user_mode ?? "STANDBY" }),
      study_seconds: snapshot.status?.study_seconds, eye_care_settings: snapshot.eye_care_settings, eye_care_status: snapshot.eye_care_status,
    }));
  }
}

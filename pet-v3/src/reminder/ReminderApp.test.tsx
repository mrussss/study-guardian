import { JSDOM } from "jsdom";
import { strict as assert } from "node:assert";
import test, { afterEach } from "node:test";
import { ReminderApp } from "./ReminderApp";
import { mockSemantic } from "../mock/semantic";
import type { PetTransportSnapshot, NativeEyeCareStatus, EyeCareAction } from "../transport/supervisor";

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


function sample(): PetTransportSnapshot {
 return {connected:true,semantic:mockSemantic({}),eye_care_settings:{enabled:true,focus_minutes:40,short_break_minutes:5,long_break_after_focus_minutes:120,long_break_minutes:20,snooze_minutes:5,max_snoozes:2},eye_care_status:{enabled:true,phase:"SHORT_BREAK_DUE",local_date:"2026-10-17",focus_segment_seconds:2400,focus_since_long_break_seconds:2400,snooze_count:0,completed_short_breaks:0,completed_long_breaks:0,retry_focus_after_seconds:0,revision:1,updated_at:"2026-10-17T10:00:00+08:00",notification_suppressed:false}};
}

test("all reminder actions remain usable on the next reminder",async()=>{
 for(const [label,action] of [["开始休息","START_SHORT_BREAK"],["延后 5 分钟","SNOOZE"],["跳过本次","SKIP"]] as const) {
  let current=sample();let hidden=0;const actions:EyeCareAction[]=[];
  const adapter={poll:async()=>current};const hide=async()=>{hidden++};
  const control={eyeCareAction:async(value:EyeCareAction)=>{actions.push(value);current={...current,eye_care_status:{...current.eye_care_status!,phase:"FOCUSING"}};return {ok:true as const}}};
  const view=render(<ReminderApp adapter={adapter} control={control} hide={hide} pollMilliseconds={20}/>);
  const user=userEvent.setup();
  await waitFor(()=>assert.equal((screen.getByRole("button",{name:label}) as HTMLButtonElement).disabled,false));
  await user.click(screen.getByRole("button",{name:label}));
  await waitFor(()=>assert.ok(hidden>0));assert.deepEqual(actions,[action]);
  await waitFor(()=>assert.equal((screen.getByRole("button",{name:label}) as HTMLButtonElement).disabled,true));
  current=sample();current.eye_care_status!.revision=2;
  await waitFor(()=>assert.equal((screen.getByRole("button",{name:label}) as HTMLButtonElement).disabled,false));
  await user.click(screen.getByRole("button",{name:label}));
  await waitFor(()=>assert.equal(actions.length,2));view.unmount();
 }
});
test("a failed hide never leaves the next click permanently busy",async()=>{
 let actions=0;let fail=true;const adapter={poll:async()=>sample()};
 const hide=async()=>{if(fail)throw new Error("denied")};
 render(<ReminderApp adapter={adapter} control={{eyeCareAction:async()=>{actions++;return {ok:true}}}} hide={hide} pollMilliseconds={20}/>);
 const user=userEvent.setup();
 await waitFor(()=>assert.equal((screen.getByRole("button",{name:"开始休息"}) as HTMLButtonElement).disabled,false));
 await user.click(screen.getByRole("button",{name:"开始休息"}));
 await waitFor(()=>assert.equal((screen.getByRole("button",{name:"开始休息"}) as HTMLButtonElement).disabled,false));
 fail=false;await user.click(screen.getByRole("button",{name:"开始休息"}));
 await waitFor(()=>assert.equal(actions,2));
});
test("polling keeps running while hidden and finds the next due revision",async()=>{
 let current=sample();current.eye_care_status!.phase="FOCUSING";let hidden=0;
 const adapter={poll:async()=>current};const hide=async()=>{hidden++};
 render(<ReminderApp adapter={adapter} hide={hide} pollMilliseconds={20}/>);
 await waitFor(()=>assert.ok(hidden>0));current=sample();
 await waitFor(()=>assert.equal((screen.getByRole("button",{name:"开始休息"}) as HTMLButtonElement).disabled,false));
});

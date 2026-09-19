import { strict as assert } from "node:assert";
import test from "node:test";
import { eyeCareResumeAction, eyeCareResumeLabel } from "./eye-care-resume";
import type { NativeEyeCareStatus, NativeSupervisorStatus } from "../transport/supervisor";

const status = (phase: NativeEyeCareStatus["phase"]): NativeEyeCareStatus => ({
  enabled: true, phase, local_date: "2026-09-13", focus_segment_seconds: 2400,
  focus_since_long_break_seconds: 2400, snooze_count: 0, completed_short_breaks: 0,
  completed_long_breaks: 0, retry_focus_after_seconds: 0, revision: 2,
  updated_at: "2026-09-13T10:00:00Z", notification_suppressed: false,
});

test("eye-care resume routes active and waiting breaks through canonical actions", () => {
  assert.equal(eyeCareResumeAction("BREAK", "EYE_CARE", status("SHORT_BREAK")), "FINISH_EARLY");
  assert.equal(eyeCareResumeAction("BREAK", "EYE_CARE", status("LONG_BREAK")), "FINISH_EARLY");
  assert.equal(eyeCareResumeAction("BREAK", "EYE_CARE", status("WAITING_RETURN")), "RESUME_STUDY");
  assert.equal(eyeCareResumeAction("BREAK", "MANUAL", status("WAITING_RETURN")), undefined);
  assert.equal(eyeCareResumeAction("STUDY", "EYE_CARE", status("SHORT_BREAK")), undefined);
  assert.equal(eyeCareResumeLabel("BREAK", "EYE_CARE", status("LONG_BREAK")), "提前结束并继续");
  assert.equal(eyeCareResumeLabel("BREAK", "EYE_CARE", status("WAITING_RETURN")), "继续学习");
});

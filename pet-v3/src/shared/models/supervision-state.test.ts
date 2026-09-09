import { strict as assert } from "node:assert";
import test from "node:test";
import { activityLabels, deriveSupervisionState, formatLastActivity } from "./supervision-state";
import { VALID_ACTIVITIES } from "../../model/semantic";
import type { NativeSupervisorStatus } from "../../transport/supervisor";

const base: NativeSupervisorStatus = {
  user_mode: "STUDY",
  interaction_state: "ACTIVE",
  task_relation: "FOCUSED",
  privacy_state: "NORMAL",
  confidence: 0.9,
  task: "Go",
  study_seconds: 10,
  break_seconds: 0,
  active_seconds: 10,
  activitywatch_ok: true,
  screen_sensor_ok: true,
};

test("supervision behavior gives mode state priority over focused", () => {
  assert.equal(deriveSupervisionState(true, { ...base, user_mode: "STANDBY" }).behaviorLabel, "等待开始");
  assert.equal(deriveSupervisionState(true, { ...base, user_mode: "BREAK" }).behaviorLabel, "休息中");
  assert.equal(deriveSupervisionState(true, { ...base, user_mode: "OFF" }).behaviorLabel, "今日已结束");
});

test("supervision behavior distinguishes activity and relation", () => {
  assert.equal(deriveSupervisionState(true, base).behaviorLabel, "正在专注");
  assert.equal(deriveSupervisionState(true, { ...base, interaction_state: "IDLE_STATIC" }).behaviorLabel, "暂时离开");
  assert.equal(deriveSupervisionState(true, { ...base, interaction_state: "IDLE_DYNAMIC" }).behaviorLabel, "无输入，屏幕仍活动");
  assert.equal(deriveSupervisionState(true, { ...base, interaction_state: "IDLE_STATIC", afk_seconds: 360 }).behaviorLabel, "已无输入 6 分钟 · 屏幕静止");
  assert.equal(deriveSupervisionState(true, { ...base, interaction_state: "IDLE_DYNAMIC", afk_seconds: 901 }).behaviorLabel, "已无输入 15 分钟 · 屏幕仍有变化");
  assert.equal(deriveSupervisionState(true, { ...base, task_relation: "DISTRACTED" }).behaviorLabel, "可能偏离任务");
});

test("supervision health fails soft without exposing raw service data", () => {
  assert.equal(deriveSupervisionState(false, undefined).behaviorLabel, "监督离线");
  assert.equal(deriveSupervisionState(true, undefined).behaviorLabel, "状态暂不可用");
  assert.equal(deriveSupervisionState(true, undefined).systemLabel, "Supervisor 已连接");
  assert.equal(deriveSupervisionState(true, base).systemLabel, "采集服务在线");
  assert.equal(deriveSupervisionState(true, { ...base, screen_sensor_ok: false }).systemLabel, "屏幕采集异常");
  assert.equal(deriveSupervisionState(true, { ...base, privacy_state: "SENSITIVE" }).behaviorLabel, "隐私保护中");
});

test("service health never overrides mode behavior", () => {
  const sensorFailure = { ...base, screen_sensor_ok: false };
  const activityWatchFailure = { ...base, activitywatch_ok: false };
  assert.deepEqual(deriveSupervisionState(true, { ...sensorFailure, user_mode: "STANDBY" }), {
    behaviorLabel: "等待开始", behaviorTone: "neutral", systemLabel: "屏幕采集异常", systemTone: "warning",
  });
  assert.equal(deriveSupervisionState(true, { ...sensorFailure, user_mode: "BREAK" }).behaviorLabel, "休息中");
  assert.equal(deriveSupervisionState(true, { ...sensorFailure, user_mode: "OFF" }).behaviorLabel, "今日已结束");
  assert.deepEqual(deriveSupervisionState(true, { ...sensorFailure, interaction_state: "IDLE_STATIC" }), {
    behaviorLabel: "暂时离开", behaviorTone: "reminder", systemLabel: "屏幕采集异常", systemTone: "warning",
  });
  assert.deepEqual(deriveSupervisionState(true, activityWatchFailure), {
    behaviorLabel: "状态暂不可用", behaviorTone: "warning", systemLabel: "活动数据异常", systemTone: "warning",
  });
  assert.equal(deriveSupervisionState(true, { ...activityWatchFailure, user_mode: "BREAK" }).behaviorLabel, "休息中");
  assert.equal(deriveSupervisionState(true, { ...activityWatchFailure, user_mode: "OFF" }).behaviorLabel, "今日已结束");
});

test("last activity uses bounded friendly relative labels", () => {
  const now = Date.parse("2026-09-06T10:00:00Z");
  assert.equal(formatLastActivity("2026-09-06T09:59:30Z", now), "刚刚");
  assert.equal(formatLastActivity("2026-09-06T09:42:00Z", now), "18 分钟前");
  assert.equal(formatLastActivity(undefined, now), "暂无记录");
});

test("every semantic activity has a Chinese display label", () => {
  for (const activity of VALID_ACTIVITIES) {
    assert.equal(typeof activityLabels[activity], "string");
    assert.ok(activityLabels[activity].length > 0);
  }
});

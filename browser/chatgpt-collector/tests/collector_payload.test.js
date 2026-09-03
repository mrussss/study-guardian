import test from 'node:test';
import assert from 'node:assert/strict';
import { buildTurnPayload } from '../src/collector.js';
import { TurnTracker } from '../src/turn_tracker.js';

function fakeSession() {
  let value = {};
  return {
    async get() { return { studyguardian_turn_contexts: value }; },
    async set(next) { value = next.studyguardian_turn_contexts; }
  };
}

function candidate() {
  return {
    platform: 'chatgpt',
    external_conversation_id: 'abc',
    title: 'Go Lab',
    url: 'https://chatgpt.com/c/abc',
    turn_key: 'turn-1',
    user: {
      external_message_id: 'user-1',
      role: 'user',
      content: 'Explain method sets',
      content_hash: 'user-hash',
      observed_at: '2026-09-03T10:00:00+08:00',
      is_active: true,
      is_final: true
    },
    assistants: []
  };
}

test('new STUDY turn payload contains awaited frozen context', async () => {
  const tracker = new TurnTracker(fakeSession(), () => Date.parse('2026-09-03T02:00:00Z'));
  const payload = await buildTurnPayload(
    candidate(),
    tracker,
    async () => ({ context: { user_mode: 'STUDY', task: 'Go Lab' }, trustworthy: true })
  );
  assert.equal(payload.mode_at_start, 'STUDY');
  assert.equal(payload.task_at_start, 'Go Lab');
  assert.equal(payload.eligible_for_review, true);
  assert.equal(payload.observed_at, '2026-09-03T10:00:00+08:00');
});

test('UNKNOWN untrusted context is fail closed in the actual payload path', async () => {
  const tracker = new TurnTracker(fakeSession(), () => Date.parse('2026-09-03T02:00:00Z'));
  const payload = await buildTurnPayload(
    { ...candidate(), turn_key: 'turn-unknown' },
    tracker,
    async () => ({ context: { user_mode: 'UNKNOWN', task: '' }, trustworthy: false })
  );
  assert.equal(payload.mode_at_start, 'UNKNOWN');
  assert.equal(payload.task_at_start, '');
  assert.equal(payload.eligible_for_review, false);
});

test('payload JSON satisfies the collector contract fields', async () => {
  const tracker = new TurnTracker(fakeSession(), () => Date.parse('2026-09-03T02:00:00Z'));
  const payload = await buildTurnPayload(candidate(), tracker, async () => ({
    context: { user_mode: 'STUDY', task: 'Go' },
    trustworthy: true
  }));
  const json = JSON.parse(JSON.stringify(payload));
  assert.deepEqual({
    platform: json.platform,
    external_conversation_id: json.external_conversation_id,
    turn_key: json.turn_key,
    observed_at: json.observed_at,
    mode_at_start: json.mode_at_start,
    task_at_start: json.task_at_start,
    eligible_for_review: json.eligible_for_review,
    messages: json.messages
  }, {
    platform: 'chatgpt',
    external_conversation_id: 'abc',
    turn_key: 'turn-1',
    observed_at: '2026-09-03T10:00:00+08:00',
    mode_at_start: 'STUDY',
    task_at_start: 'Go',
    eligible_for_review: true,
    messages: [candidate().user]
  });
});

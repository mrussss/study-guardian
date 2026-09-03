import test from 'node:test';
import assert from 'node:assert/strict';
import { TurnTracker } from '../src/turn_tracker.js';

function fakeSession() {
  let value = {};
  return {
    async get() { return { studyguardian_turn_contexts: value }; },
    async set(next) { value = next.studyguardian_turn_contexts; },
    dump() { return value; }
  };
}

const turn = { turn_key: 'turn-1', user: { content: 'question' } };

test('frozen context survives a service worker tracker restart', async () => {
  const session = fakeSession();
  const first = new TurnTracker(session, () => Date.parse('2026-09-03T10:00:00Z'));
  const original = await first.contextFor(turn, { user_mode: 'STUDY', task: 'Go' });
  const restarted = new TurnTracker(session, () => Date.parse('2026-09-03T10:01:00Z'));
  assert.equal(await restarted.hasContext('turn-1'), true);
  assert.deepEqual(await restarted.contextFor(turn, { user_mode: 'BREAK', task: '' }), original);
  assert.equal((await restarted.contextFor(turn, { user_mode: 'BREAK' })).eligible_for_review, true);
});

test('old session contexts are pruned on load', async () => {
  const session = fakeSession();
  await session.set({ studyguardian_turn_contexts: {
    old: { created_at: '2026-09-01T00:00:00Z', eligible_for_review: true },
    recent: { created_at: '2026-09-03T09:00:00Z', eligible_for_review: false }
  }});
  const tracker = new TurnTracker(session, () => Date.parse('2026-09-03T10:00:00Z'));
  assert.equal(await tracker.hasContext('old'), false);
  assert.equal(await tracker.hasContext('recent'), true);
});

test('untrusted UNKNOWN context fails closed', async () => {
  const tracker = new TurnTracker(fakeSession(), () => Date.parse('2026-09-03T10:00:00Z'));
  const context = await tracker.contextFor({ turn_key: 'unknown', user: { content: 'question' } }, { user_mode: 'UNKNOWN', task: '' }, { allowReview: false });
  assert.equal(context.mode_at_start, 'UNKNOWN');
  assert.equal(context.eligible_for_review, false);
});

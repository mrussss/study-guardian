import test from 'node:test';
import assert from 'node:assert/strict';
import { turnsFromMessages, stableHash } from '../src/parser.js';
import { MessageStateTracker, ASSISTANT_STABILITY_MS } from '../src/stream_state.js';

function messages(assistantContents) {
  return [
    { external_message_id: 'user-1', role: 'user', content: 'Question', content_hash: stableHash('Question'), observed_at: '2026-09-03T10:00:00+08:00' },
    ...assistantContents.map((content, index) => ({
      external_message_id: `assistant-${index + 1}`,
      role: 'assistant',
      content,
      content_hash: stableHash(content),
      observed_at: '2026-09-03T10:00:00+08:00'
    }))
  ];
}

test('assistant starts active and streaming, then becomes final after stable scan', () => {
  const tracker = new MessageStateTracker(() => 0);
  const streaming = turnsFromMessages(tracker.update(messages(['Hel']), 0))[0];
  assert.equal(streaming.user.is_active, true);
  assert.equal(streaming.user.is_final, true);
  assert.equal(streaming.assistants[0].is_active, true);
  assert.equal(streaming.assistants[0].is_final, false);

  const final = turnsFromMessages(tracker.update(messages(['Hel']), ASSISTANT_STABILITY_MS))[0];
  assert.equal(final.assistants[0].is_active, true);
  assert.equal(final.assistants[0].is_final, true);
  assert.notEqual(final.assistants[0].finalized_at, null);
});

test('only the last assistant in a turn remains active', () => {
  const tracker = new MessageStateTracker(() => 0);
  const turn = turnsFromMessages(tracker.update(messages(['first', 'second']), 0))[0];
  assert.equal(turn.assistants[0].is_active, false);
  assert.equal(turn.assistants[1].is_active, true);
});

test('stable identity survives streaming content changes', () => {
  const tracker = new MessageStateTracker(() => 0);
  const first = tracker.update([{ external_message_id: 'conversation-turn-42:assistant', role: 'assistant', content: 'H', content_hash: stableHash('H'), observed_at: 'now' }], 0)[0];
  const second = tracker.update([{ external_message_id: 'conversation-turn-42:assistant', role: 'assistant', content: 'Hello', content_hash: stableHash('Hello'), observed_at: 'now' }], 1000)[0];
  assert.equal(first.external_message_id, second.external_message_id);
  assert.equal(second.is_final, false);
});

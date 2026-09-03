import test from 'node:test';
import assert from 'node:assert/strict';
import { DeliveredState, turnSnapshotHash } from '../src/candidate_dedupe.js';

const turn = (content, isFinal = false) => ({
  turn_key: 'turn-1',
  user: { content_hash: 'user' },
  assistants: [{ external_message_id: 'assistant-1', content_hash: content, is_active: true, is_final: isFinal }]
});

test('unchanged turn snapshot is suppressed', () => {
  const delivered = new DeliveredState();
  const first = turn('Hel');
  delivered.remember(first);
  assert.equal(delivered.hasSameSnapshot(first), true);
  assert.equal(turnSnapshotHash(first), delivered.hashFor(first));
});

test('streaming and final state changes are emitted once each', () => {
  const delivered = new DeliveredState();
  const streaming = turn('Hel');
  const grown = turn('Hello');
  const final = turn('Hello', true);
  delivered.remember(streaming);
  assert.equal(delivered.hasSameSnapshot(grown), false);
  delivered.remember(grown);
  assert.equal(delivered.hasSameSnapshot(final), false);
  delivered.remember(final);
  assert.equal(delivered.hasSameSnapshot(final), true);
  assert.equal(delivered.hasSameSnapshot(final), true);
});

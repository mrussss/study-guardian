import test from 'node:test';
import assert from 'node:assert/strict';
import { deliverInOrder } from '../src/collector_delivery.js';

test('queue flush completes before current final snapshot is posted', async () => {
  const calls = [];
  let stored = 'Hel';
  const result = await deliverInOrder({ turn_key: 'current', answer: 'Hello world', is_final: true }, {
    flushQueue: async () => {
      calls.push('queue:Hel');
      stored = 'Hel';
      return { ok: true };
    },
    postPayload: async payload => {
      calls.push(`current:${payload.answer}`);
      stored = payload.answer;
    },
    enqueue: async () => { throw new Error('enqueue should not be called'); }
  });
  assert.deepEqual(calls, ['queue:Hel', 'current:Hello world']);
  assert.equal(stored, 'Hello world');
  assert.equal(result.ok, true);
});

test('failed queue flush queues current without an immediate second POST', async () => {
  const calls = [];
  const queued = ['old payload'];
  const result = await deliverInOrder({ turn_key: 'current' }, {
    flushQueue: async () => {
      calls.push('flush');
      return { ok: false };
    },
    postPayload: async () => { calls.push('post:current'); },
    enqueue: async payload => { calls.push('enqueue:current'); queued.push(payload.turn_key); }
  });
  assert.deepEqual(calls, ['flush', 'enqueue:current']);
  assert.deepEqual(queued, ['old payload', 'current']);
  assert.equal(result.ok, false);
  assert.equal(result.stage, 'flush');
});

import test from 'node:test';
import assert from 'node:assert/strict';
import { DeliverySerializer } from '../src/delivery_serial.js';
import { deliverInOrder } from '../src/collector_delivery.js';
import { enqueue, readQueue } from '../src/queue.js';

const delay = milliseconds => new Promise(resolve => setTimeout(resolve, milliseconds));

function fakeStorage(initial = []) {
  let value = initial;
  return {
    async get() { return { studyguardian_collector_queue: value }; },
    async set(next) { value = next.studyguardian_collector_queue; }
  };
}

const snapshot = (answer, isFinal = false) => ({
  turn_key: answer,
  mode_at_start: 'STUDY',
  task_at_start: 'Collector',
  eligible_for_review: true,
  observed_at: '2026-09-03T10:00:00+08:00',
  messages: [{ external_message_id: 'assistant-1', role: 'assistant', content: answer, is_final: isFinal }]
});

test('slow old snapshot cannot be overtaken by fast new snapshot', async () => {
  const serializer = new DeliverySerializer();
  const calls = [];
  const first = serializer.run(async () => {
    calls.push('H:start');
    await delay(50);
    calls.push('H:end');
  });
  const second = serializer.run(async () => { calls.push('He'); });
  await Promise.all([first, second]);
  assert.deepEqual(calls, ['H:start', 'H:end', 'He']);
});

test('a failed operation does not poison the serializer chain', async () => {
  const serializer = new DeliverySerializer();
  const calls = [];
  const first = serializer.run(async () => { calls.push('H'); throw new Error('offline'); });
  const second = serializer.run(async () => { calls.push('He'); return 'recovered'; });
  await assert.rejects(first, /offline/);
  assert.equal(await second, 'recovered');
  assert.deepEqual(calls, ['H', 'He']);
});

test('concurrent offline deliveries enqueue H before He', async () => {
  const serializer = new DeliverySerializer();
  const storage = fakeStorage();
  const submit = payload => serializer.run(() => deliverInOrder(payload, {
    flushQueue: async () => ({ ok: true }),
    postPayload: async () => { throw new Error('offline'); },
    enqueue: item => enqueue(item, storage)
  }));
  await Promise.all([submit(snapshot('H')), submit(snapshot('He'))]);
  assert.deepEqual((await readQueue(storage)).map(item => item.messages[0].content), ['H', 'He']);
});

test('serialized deliverInOrder preserves three snapshots and final server state', async () => {
  const serializer = new DeliverySerializer();
  const calls = [];
  const serverState = {};
  const submit = (payload, milliseconds) => serializer.run(() => deliverInOrder(payload, {
    flushQueue: async () => ({ ok: true }),
    postPayload: async item => {
      await delay(milliseconds);
      const message = item.messages[0];
      calls.push(message.content);
      serverState.content = message.content;
      serverState.is_final = message.is_final;
    },
    enqueue: async () => { throw new Error('enqueue should not be called'); }
  }));
  await Promise.all([
    submit(snapshot('H'), 50),
    submit(snapshot('He'), 10),
    submit(snapshot('Hello', true), 0)
  ]);
  assert.deepEqual(calls, ['H', 'He', 'Hello']);
  assert.deepEqual(serverState, { content: 'Hello', is_final: true });
});

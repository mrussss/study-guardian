import test from 'node:test';
import assert from 'node:assert/strict';
import { ackQueueHead, enqueue, peekQueue, readQueue, writeQueue } from '../src/queue.js';
import { flushQueuedPayloads } from '../src/collector_delivery.js';

function fakeStorage(initial = []) {
  let value = initial;
  return {
    async get() { return { studyguardian_collector_queue: value }; },
    async set(next) { value = next.studyguardian_collector_queue; }
  };
}

function payload(answer, isFinal = false) {
  return {
    platform: 'chatgpt',
    external_conversation_id: 'abc',
    turn_key: 'turn-1',
    observed_at: '2026-09-03T10:00:00+08:00',
    mode_at_start: 'STUDY',
    task_at_start: 'Collector',
    eligible_for_review: true,
    messages: [{
      external_message_id: 'assistant-1',
      role: 'assistant',
      content: answer,
      content_hash: answer,
      is_active: true,
      is_final: isFinal
    }]
  };
}

function realFlush(storage, postPayload) {
  return flushQueuedPayloads({
    peekQueue: () => peekQueue(storage),
    ackQueueHead: item => ackQueueHead(item, storage),
    postPayload
  });
}

test('failed POST leaves the failed payload at the original queue head', async () => {
  const initial = [payload('H'), payload('He')];
  const storage = fakeStorage(initial);
  const calls = [];
  const result = await realFlush(storage, async item => {
    calls.push(item.messages[0].content);
    throw new Error('offline');
  });
  assert.equal(result.ok, false);
  assert.deepEqual(calls, ['H']);
  assert.deepEqual(await readQueue(storage), initial);
});

test('current payload appends after old queue when flush fails', async () => {
  const storage = fakeStorage([payload('H'), payload('He')]);
  await realFlush(storage, async () => { throw new Error('offline'); });
  await enqueue(payload('Hello', true), storage);
  assert.deepEqual((await readQueue(storage)).map(item => item.messages[0].content), ['H', 'He', 'Hello']);
});

test('successful POST is acknowledged before the next queue item is sent', async () => {
  const storage = fakeStorage([payload('H'), payload('He'), payload('Hello', true)]);
  const calls = [];
  const serverState = new Map();
  const result = await realFlush(storage, async item => {
    const message = item.messages[0];
    calls.push(message.content);
    serverState.set(message.external_message_id, { content: message.content, is_final: message.is_final });
    assert.equal(item.mode_at_start, 'STUDY');
    assert.equal(item.task_at_start, 'Collector');
    assert.equal(item.eligible_for_review, true);
    assert.equal(item.observed_at, '2026-09-03T10:00:00+08:00');
  });
  assert.equal(result.ok, true);
  assert.deepEqual(calls, ['H', 'He', 'Hello']);
  assert.deepEqual(await readQueue(storage), []);
  assert.deepEqual(serverState.get('assistant-1'), { content: 'Hello', is_final: true });
});

test('ack only removes the expected immutable head payload', async () => {
  const storage = fakeStorage([payload('H'), payload('He')]);
  assert.equal(await ackQueueHead(payload('He'), storage), false);
  assert.deepEqual((await readQueue(storage)).map(item => item.messages[0].content), ['H', 'He']);
  assert.equal(await ackQueueHead(payload('H'), storage), true);
  assert.deepEqual((await readQueue(storage)).map(item => item.messages[0].content), ['He']);
  assert.equal(await peekQueue(storage).then(item => item.messages[0].content), 'He');
});

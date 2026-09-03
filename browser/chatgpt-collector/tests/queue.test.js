import test from 'node:test';
import assert from 'node:assert/strict';
import { enqueue, readQueue } from '../src/queue.js';

function fakeStorage(initial = []) {
  let value = initial;
  return {
    async get() { return { studyguardian_collector_queue: value }; },
    async set(next) { value = next.studyguardian_collector_queue; }
  };
}

test('queue drops finalized assistant payload before user prompts', async () => {
  const initial = [{ turn_key: 'assistant-old', messages: [{ role: 'assistant', is_final: true, content: 'done' }] }];
  for (let i = 0; i < 999; i++) initial.push({ turn_key: `user-${i}`, messages: [{ role: 'user', content: 'keep me' }] });
  const storage = fakeStorage(initial);
  const result = await enqueue({ turn_key: 'new-user', messages: [{ role: 'user', content: 'new prompt' }] }, storage);
  const items = await readQueue(storage);
  assert.equal(result.dropped, 1);
  assert.equal(items.some(item => item.turn_key === 'assistant-old'), false);
  assert.equal(items.some(item => item.turn_key === 'user-0'), true);
});

test('queue enforces item count limit', async () => {
  const storage = fakeStorage();
  for (let i = 0; i < 1001; i++) await enqueue({ turn_key: `turn-${i}`, messages: [{ role: 'user', content: 'q' }] }, storage);
  const items = await readQueue(storage);
  assert.equal(items.length, 1000);
  assert.equal(items[0].turn_key, 'turn-1');
});

import test from 'node:test';
import assert from 'node:assert/strict';
import { contextAgeMs, isHardFresh, isSoftFresh, resolveContextForNewTurn, SOFT_TTL_MS, HARD_TTL_MS } from '../src/mode_cache.js';

function fakeStorage(value) {
  return {
    async get() { return { studyguardian_collector_context: value }; },
    async set(next) { value = next.studyguardian_collector_context; }
  };
}

const now = Date.parse('2026-09-03T10:00:00.000Z');
const contextAt = ms => ({ user_mode: 'STUDY', task: 'Go', status_observed_at: new Date(now - ms).toISOString() });

test('fresh context is used without refreshing', async () => {
  let refreshes = 0;
  const result = await resolveContextForNewTurn(async () => { refreshes++; return { user_mode: 'BREAK' }; }, fakeStorage(contextAt(2_000)), now);
  assert.equal(refreshes, 0);
  assert.equal(result.context.user_mode, 'STUDY');
  assert.equal(result.trustworthy, true);
  assert.equal(isSoftFresh(result.context, now), true);
});

test('soft-expired context refreshes successfully', async () => {
  let refreshes = 0;
  const result = await resolveContextForNewTurn(async () => { refreshes++; return { user_mode: 'BREAK', task: '' }; }, fakeStorage(contextAt(8_000)), now);
  assert.equal(refreshes, 1);
  assert.equal(result.context.user_mode, 'BREAK');
});

test('temporary refresh failure may use hard-fresh cache', async () => {
  const result = await resolveContextForNewTurn(async () => { throw new Error('offline'); }, fakeStorage(contextAt(8_000)), now);
  assert.equal(result.context.user_mode, 'STUDY');
  assert.equal(result.trustworthy, true);
  assert.equal(result.degraded, true);
});

test('hard-expired context fails closed', async () => {
  const result = await resolveContextForNewTurn(async () => { throw new Error('offline'); }, fakeStorage(contextAt(20_000)), now);
  assert.equal(result.context.user_mode, 'UNKNOWN');
  assert.equal(result.trustworthy, false);
  assert.equal(isHardFresh(result.context, now), false);
});

test('invalid and future timestamps are handled safely', () => {
  assert.equal(contextAgeMs({ status_observed_at: 'bad' }, now), Infinity);
  assert.equal(contextAgeMs({ status_observed_at: new Date(now + 1000).toISOString() }, now), 0);
  assert.equal(SOFT_TTL_MS, 5000);
  assert.equal(HARD_TTL_MS, 15000);
});

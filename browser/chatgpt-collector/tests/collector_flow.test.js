import test from 'node:test';
import assert from 'node:assert/strict';
import { filterNewTurns } from '../src/baseline.js';

const message = (id, role) => ({ external_message_id: id, role, content: id });

test('attach baseline filters old conversation and keeps a new turn', () => {
  const oldTurns = [
    { turn_key: 'old-1', user: message('u1', 'user'), assistants: [message('a1', 'assistant')] },
    { turn_key: 'old-2', user: message('u2', 'user'), assistants: [message('a2', 'assistant')] }
  ];
  const baseline = new Set(['u1', 'a1', 'u2', 'a2']);
  assert.equal(filterNewTurns(oldTurns, baseline).length, 0);
  const newTurn = { turn_key: 'new', user: message('u3', 'user'), assistants: [] };
  assert.deepEqual(filterNewTurns([...oldTurns, newTurn], baseline), [newTurn]);
});

test('assistant streaming remains the same new turn, not an old-history import', () => {
  const baseline = new Set(['u1', 'a1']);
  const newUser = { turn_key: 'new', user: message('u2', 'user'), assistants: [] };
  assert.equal(filterNewTurns([newUser], baseline).length, 1);
  const withAssistant = { ...newUser, assistants: [message('a2', 'assistant')] };
  assert.equal(filterNewTurns([withAssistant], baseline).length, 1);
});

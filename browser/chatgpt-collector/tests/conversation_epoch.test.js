import test from 'node:test';
import assert from 'node:assert/strict';
import { ConversationEpoch, conversationKey } from '../src/conversation_epoch.js';
import { filterNewTurns } from '../src/baseline.js';

const conversation = id => ({ platform: 'chatgpt', external_conversation_id: id });
const message = (id, role) => ({ external_message_id: id, role, content: id });

test('SPA conversation switch increments epoch and resets the key', () => {
  const epoch = new ConversationEpoch();
  assert.equal(conversationKey(conversation('A')), 'chatgpt:A');
  assert.equal(epoch.observe(conversation('A')), true);
  assert.equal(epoch.value, 1);
  assert.equal(epoch.observe(conversation('A')), false);
  assert.equal(epoch.observe(conversation('B')), true);
  assert.equal(epoch.value, 2);
  assert.equal(epoch.currentConversationKey, 'chatgpt:B');
});

test('new conversation baseline ignores all existing B history', () => {
  const bHistory = [
    { turn_key: 'b1', user: message('b-user-1', 'user'), assistants: [message('b-assistant-1', 'assistant')] },
    { turn_key: 'b2', user: message('b-user-2', 'user'), assistants: [message('b-assistant-2', 'assistant')] }
  ];
  const baseline = new Set(['b-user-1', 'b-assistant-1', 'b-user-2', 'b-assistant-2']);
  assert.equal(filterNewTurns(bHistory, baseline).length, 0);
  assert.equal(filterNewTurns([...bHistory, { turn_key: 'b3', user: message('b-user-3', 'user'), assistants: [] }], baseline).length, 1);
});

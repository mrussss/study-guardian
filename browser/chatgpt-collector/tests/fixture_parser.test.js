import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { parseConversation } from '../src/parser.js';
import { MessageStateTracker, ASSISTANT_STABILITY_MS } from '../src/stream_state.js';

const fixtureRoot = join(dirname(fileURLToPath(import.meta.url)), 'fixtures', 'chatgpt');

test('conversation-a fixture parses conversation identity and both message roles', async () => {
  const { parseHTML } = await import('linkedom');
  const html = readFileSync(join(fixtureRoot, 'conversation-a.html'), 'utf8');
  const { document } = parseHTML(html);
  const conversation = parseConversation(document, { pathname: '/c/conversation-a', href: 'https://chatgpt.com/c/conversation-a' });
  assert.equal(conversation.external_conversation_id, 'conversation-a');
  assert.deepEqual(conversation.messages.map(message => message.role), ['user', 'assistant', 'user', 'assistant']);
  assert.equal(conversation.messages[0].content, 'Old A question');
  assert.equal(conversation.messages[1].external_message_id, 'a-assistant-1');
});

test('streaming fixtures keep assistant identity while content changes', async () => {
  const { parseHTML } = await import('linkedom');
  const firstHTML = readFileSync(join(fixtureRoot, 'assistant-streaming-1.html'), 'utf8');
  const secondHTML = readFileSync(join(fixtureRoot, 'assistant-streaming-2.html'), 'utf8');
  const first = parseConversation(parseHTML(firstHTML).document, { pathname: '/c/fixture', href: 'https://chatgpt.com/c/fixture' }).messages[0];
  const second = parseConversation(parseHTML(secondHTML).document, { pathname: '/c/fixture', href: 'https://chatgpt.com/c/fixture' }).messages[0];
  const tracker = new MessageStateTracker(() => 0);
  const firstState = tracker.update([first], 0)[0];
  const secondState = tracker.update([second], 1000)[0];
  assert.equal(firstState.external_message_id, secondState.external_message_id);
  assert.notEqual(firstState.content_hash, secondState.content_hash);
  assert.equal(secondState.is_final, false);
});

test('assistant-final fixture becomes final after stable scan', async () => {
  const { parseHTML } = await import('linkedom');
  const html = readFileSync(join(fixtureRoot, 'assistant-final.html'), 'utf8');
  const message = parseConversation(parseHTML(html).document, { pathname: '/c/fixture', href: 'https://chatgpt.com/c/fixture' }).messages[0];
  const tracker = new MessageStateTracker(() => 0);
  tracker.update([message], 0);
  const final = tracker.update([message], ASSISTANT_STABILITY_MS)[0];
  assert.equal(final.is_active, true);
  assert.equal(final.is_final, true);
  assert.notEqual(final.finalized_at, null);
});

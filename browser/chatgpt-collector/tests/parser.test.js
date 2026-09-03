import test from 'node:test';
import assert from 'node:assert/strict';
import { parseConversation, turnsFromMessages, stableHash } from '../src/parser.js';

class FakeNode {
  constructor(role, content, children = []) { this.role = role; this.content = content; this.children = children; }
  getAttribute(name) { return name === 'data-message-author-role' ? this.role : null; }
  hasAttribute(name) { return name === 'data-message-author-role' && !!this.role; }
  querySelector(selector) {
    if (selector.includes('.markdown')) return this.children[0] || null;
    if (selector.includes('[data-message-content]')) return null;
    if (selector.includes('data-message-author-role="user"')) return this.role === 'user' ? this : null;
    if (selector.includes('data-message-author-role="assistant"')) return this.role === 'assistant' ? this : null;
    return null;
  }
  querySelectorAll() { return [this, ...this.children]; }
  get innerText() { return this.content; }
  get textContent() { return this.content; }
}

test('parser extracts DOM messages and groups a user turn with assistant output', () => {
  const user = new FakeNode('user', 'Explain method sets');
  const assistant = new FakeNode('assistant', '', [new FakeNode('', 'A method set is...')]);
  const documentLike = { title: 'Go Lab', querySelectorAll: () => [user, assistant] };
  const conversation = parseConversation(documentLike, { pathname: '/c/conversation-1', href: 'https://chatgpt.com/c/conversation-1' });
  assert.equal(conversation.external_conversation_id, 'conversation-1');
  assert.equal(conversation.messages.length, 2);
  assert.equal(turnsFromMessages(conversation.messages).length, 1);
  assert.equal(turnsFromMessages(conversation.messages)[0].assistants.length, 1);
});

test('stable hash is deterministic', () => {
  assert.equal(stableHash('study'), stableHash('study'));
});

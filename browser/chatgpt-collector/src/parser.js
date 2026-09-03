// DOM-only parser. It intentionally does not call ChatGPT history/private APIs.
export function normalizeText(value) {
  return String(value || '').replace(/\u00a0/g, ' ').replace(/[ \t]+/g, ' ').replace(/\n{3,}/g, '\n\n').trim();
}

export function stableHash(value) {
  let hash = 2166136261;
  for (const char of String(value || '')) {
    hash ^= char.codePointAt(0);
    hash = Math.imul(hash, 16777619);
  }
  return (hash >>> 0).toString(16).padStart(8, '0');
}

function textForNode(node) {
  const content = node.querySelector('[data-message-content], .markdown, .whitespace-pre-wrap') || node;
  return normalizeText(content.innerText || content.textContent || '');
}

export function messageIdentity(node) {
  const explicit = node.getAttribute('data-message-id') || node.getAttribute('data-testid');
  const role = node.getAttribute('data-message-author-role') || '';
  return explicit || `${role}:${stableHash(textForNode(node))}`;
}

export function parseMessageNode(node) {
  const role = node.getAttribute('data-message-author-role') ||
    (node.querySelector('[data-message-author-role="user"]') ? 'user' :
      node.querySelector('[data-message-author-role="assistant"]') ? 'assistant' : '');
  if (!role) return null;
  const content = textForNode(node);
  if (!content) return null;
  const id = messageIdentity(node);
  return { external_message_id: id, role, content, content_hash: stableHash(content), observed_at: new Date().toISOString() };
}

export function findMessageNodes(root = document) {
  return [...root.querySelectorAll('[data-message-author-role], [data-testid^="conversation-turn"]')]
    .filter(node => node.querySelector('[data-message-author-role]') === null || node.hasAttribute('data-message-author-role'));
}

export function conversationInfo(locationLike = window.location, documentLike = document) {
  const match = String(locationLike.pathname || '').match(/\/c\/([^/?#]+)/);
  return {
    platform: 'chatgpt',
    external_conversation_id: match ? match[1] : `url:${stableHash(locationLike.href || '')}`,
    title: normalizeText(documentLike.title || ''),
    url: String(locationLike.href || '')
  };
}

export function parseConversation(root = document, locationLike = globalThis.location) {
  const messages = findMessageNodes(root).map(parseMessageNode).filter(Boolean);
  const loc = root.location || locationLike || { pathname: '', href: '' };
  return { ...conversationInfo(loc, root), messages };
}

export function visibleIdentities(root = document) {
  return findMessageNodes(root).map(messageIdentity);
}

export function turnsFromMessages(messages) {
  const turns = [];
  let current = null;
  for (const message of messages) {
    if (message.role === 'user') {
      current = { user: message, assistants: [] };
      turns.push(current);
    } else if (message.role === 'assistant' && current) {
      current.assistants.push(message);
    }
  }
  return turns.map(turn => {
    const turnKey = stableHash(`${turn.user.external_message_id}|${turn.user.content_hash}`);
    return { turn_key: turnKey, user: turn.user, assistants: turn.assistants };
  });
}

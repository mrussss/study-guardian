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

function attribute(node, name) {
  return String(node?.getAttribute?.(name) || '');
}

export function detectRole(node) {
  return attribute(node, 'data-message-author-role') ||
    (node.querySelector?.('[data-message-author-role="user"]') ? 'user' :
      node.querySelector?.('[data-message-author-role="assistant"]') ? 'assistant' : '');
}

function conversationTurnNode(node) {
  if (attribute(node, 'data-testid').startsWith('conversation-turn')) return node;
  if (typeof node?.closest === 'function') return node.closest('[data-testid^="conversation-turn"]');
  let current = node?.parentElement;
  while (current) {
    if (attribute(current, 'data-testid').startsWith('conversation-turn')) return current;
    current = current.parentElement;
  }
  return null;
}

function identityDetails(node) {
  const role = detectRole(node);
  const explicit = attribute(node, 'data-message-id');
  if (explicit) return { value: explicit, source: 'data-message-id' };

  const turnNode = conversationTurnNode(node);
  const turnID = attribute(turnNode, 'data-testid');
  if (turnID) return { value: `${turnID}:${role}`, source: 'conversation-turn' };

  return { value: `${role}:fallback:${stableHash(textForNode(node))}`, source: 'content_hash_fallback' };
}

export function stableNodeIdentity(node) {
  return identityDetails(node).value;
}

export function messageIdentity(node) {
  return stableNodeIdentity(node);
}

export function parseMessageNode(node) {
  const role = detectRole(node);
  if (!role) return null;
  const content = textForNode(node);
  if (!content) return null;
  const identity = identityDetails(node);
  const observedAt = new Date().toISOString();
  const branchKey = attribute(node, 'data-branch-key') || attribute(node, 'data-message-branch-key');
  const isUser = role === 'user';
  return {
    external_message_id: identity.value,
    role,
    branch_key: branchKey,
    content,
    content_hash: stableHash(content),
    observed_at: observedAt,
    finalized_at: isUser ? observedAt : null,
    is_final: isUser,
    is_active: true,
    metadata_json: JSON.stringify({ identity_source: identity.source })
  };
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
    const assistants = turn.assistants.map((message, index) => ({
      ...message,
      is_active: index === turn.assistants.length - 1
    }));
    return { turn_key: turnKey, user: { ...turn.user, is_active: true, is_final: true }, assistants };
  });
}

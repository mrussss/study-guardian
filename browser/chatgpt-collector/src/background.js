import { getContext, resolveContextForNewTurn, setContext } from './mode_cache.js';
import { enqueue, dequeue, readQueue } from './queue.js';
import { TurnTracker } from './turn_tracker.js';

const BASE = 'http://127.0.0.1:17321';
const tracker = new TurnTracker();

async function token() {
  const result = await chrome.storage.local.get({ collector_token: '' });
  return String(result.collector_token || '').trim();
}

async function request(path, init = {}) {
  const auth = await token();
  if (!auth) throw new Error('collector token is not configured');
  const headers = { 'Content-Type': 'application/json', Authorization: `Bearer ${auth}`, ...(init.headers || {}) };
  return fetch(`${BASE}${path}`, { ...init, headers });
}

async function refreshContext() {
  const response = await request('/v1/collector/context');
  if (!response.ok) throw new Error(`context HTTP ${response.status}`);
  const context = await response.json();
  return setContext(context);
}

async function flushQueue() {
  while (true) {
    const payload = await dequeue();
    if (!payload) return;
    try {
      const response = await request('/v1/collector/turn', { method: 'POST', body: JSON.stringify(payload) });
      if (!response.ok) throw new Error(`turn HTTP ${response.status}`);
    } catch (error) {
      await enqueue(payload);
      return { error: String(error) };
    }
  }
}

async function sendTurn(candidate) {
  const isNewTurn = !(await tracker.hasContext(candidate.turn_key));
  const resolved = isNewTurn
    ? await resolveContextForNewTurn(refreshContext)
    : { context: await getContext(), trustworthy: true };
  const frozen = tracker.contextFor(candidate, resolved.context, { allowReview: resolved.trustworthy });
  const payload = {
    platform: candidate.platform,
    external_conversation_id: candidate.external_conversation_id,
    title: candidate.title,
    url: candidate.url,
    capture_policy: 'AUTO',
    external_turn_id: candidate.turn_key,
    turn_key: candidate.turn_key,
    observed_at: candidate.user.observed_at,
    ...frozen,
    active_branch_key: candidate.assistants.at(-1)?.branch_key || '',
    finalized: candidate.assistants.some(message => message.is_final),
    messages: [candidate.user, ...candidate.assistants]
  };
  try {
    const response = await request('/v1/collector/turn', { method: 'POST', body: JSON.stringify(payload) });
    if (!response.ok) throw new Error(`turn HTTP ${response.status}`);
  } catch (_) {
    await enqueue(payload);
  }
  await flushQueue();
}

chrome.runtime.onMessage.addListener((message, sender, sendResponse) => {
  (async () => {
    if (message.type === 'attach') {
      tracker.attach(message.identities || []);
      sendResponse({ ok: true });
      return;
    }
    if (message.type === 'turn_candidates') {
      for (const candidate of message.candidates || []) await sendTurn(candidate);
      sendResponse({ ok: true, queue_depth: (await readQueue()).length });
      return;
    }
    if (message.type === 'refresh_context') {
      sendResponse(await refreshContext());
      return;
    }
    if (message.type === 'queue_depth') {
      sendResponse({ queue_depth: (await readQueue()).length });
    }
  })().catch(error => sendResponse({ ok: false, error: String(error) }));
  return true;
});

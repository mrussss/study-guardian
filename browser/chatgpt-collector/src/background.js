import { getContext, resolveContextForNewTurn, setContext } from './mode_cache.js';
import { enqueue, dequeue, readQueue } from './queue.js';
import { TurnTracker } from './turn_tracker.js';
import { buildTurnPayload } from './collector.js';
import { deliverInOrder } from './collector_delivery.js';

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
    if (!payload) return { ok: true };
    try {
      const response = await request('/v1/collector/turn', { method: 'POST', body: JSON.stringify(payload) });
      if (!response.ok) throw new Error(`turn HTTP ${response.status}`);
    } catch (error) {
      await enqueue(payload);
      return { ok: false, error: String(error) };
    }
  }
}

async function sendTurn(candidate) {
  const payload = await buildTurnPayload(candidate, tracker, () => resolveContextForNewTurn(refreshContext), getContext);
  await deliverInOrder(payload, {
    flushQueue,
    postPayload: async current => {
      const response = await request('/v1/collector/turn', { method: 'POST', body: JSON.stringify(current) });
      if (!response.ok) throw new Error(`turn HTTP ${response.status}`);
    },
    enqueue
  });
}

chrome.runtime.onMessage.addListener((message, sender, sendResponse) => {
  (async () => {
    if (message.type === 'attach') {
      // Baseline belongs to the page-bound Content Script. Keep this message
      // as a harmless compatibility acknowledgement for an older page epoch.
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

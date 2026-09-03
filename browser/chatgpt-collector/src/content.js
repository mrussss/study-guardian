import { parseConversation, visibleIdentities, turnsFromMessages } from './parser.js';
import { filterNewTurns } from './baseline.js';
import { MessageStateTracker } from './stream_state.js';
import { ConversationEpoch } from './conversation_epoch.js';
import { DeliveredState } from './candidate_dedupe.js';

let lastSnapshot = '';
let observer;
let settleTimer;
let scanTimer;
let baselineIdentities = new Set();
let baselineSettled = false;
const conversationEpoch = new ConversationEpoch();
const messageStates = new MessageStateTracker();
const deliveredTurns = new DeliveredState();

function snapshotKey(conversation, messages) {
  return JSON.stringify({
    conversation: conversation.external_conversation_id,
    messages: messages.map(message => [message.external_message_id, message.content_hash, message.is_active, message.is_final])
  });
}

function scheduleScan(delay = 1500) {
  clearTimeout(scanTimer);
  scanTimer = setTimeout(scan, delay);
}

function resetConversationEpoch(conversation, increment = true) {
  if (increment) conversationEpoch.reset(conversation);
  baselineIdentities = new Set(visibleIdentities(document));
  deliveredTurns.clear();
  messageStates.clear();
  lastSnapshot = snapshotKey(conversation, conversation.messages);
  baselineSettled = false;
  clearTimeout(settleTimer);
  settleTimer = setTimeout(() => { baselineSettled = true; }, 1500);
}

async function emitCandidates(candidates) {
  const pending = candidates.filter(turn => !deliveredTurns.hasSameSnapshot(turn));
  if (!pending.length) return;
  try {
    await chrome.runtime.sendMessage({ type: 'turn_candidates', candidates: pending });
    for (const turn of pending) deliveredTurns.remember(turn);
  } catch (_) {
    // A Worker restart is retried by the next mutation or stability scan.
  }
}

function scan() {
  const conversation = parseConversation(document);
  if (conversationEpoch.observe(conversation)) {
    resetConversationEpoch(conversation, false);
    return;
  }

  const messages = messageStates.update(conversation.messages);
  const snapshot = snapshotKey(conversation, messages);
  if (snapshot === lastSnapshot) {
    if (messages.some(message => message.role === 'assistant' && !message.is_final)) scheduleScan(1500);
    return;
  }
  lastSnapshot = snapshot;
  if (!baselineSettled) {
    for (const identity of visibleIdentities(document)) baselineIdentities.add(identity);
    return;
  }
  const candidates = turnsFromMessages(messages)
    .filter(turn => filterNewTurns([turn], baselineIdentities).length > 0)
    .map(turn => ({ ...conversation, ...turn }));
  void emitCandidates(candidates);
  if (messages.some(message => message.role === 'assistant' && !message.is_final)) scheduleScan(1500);
}

function attach() {
  resetConversationEpoch(parseConversation(document));
  observer = new MutationObserver(() => {
    clearTimeout(observer.timer);
    observer.timer = setTimeout(scan, 1500);
  });
  observer.observe(document.body, { childList: true, subtree: true, characterData: true });
}

if (document.body) attach();
else window.addEventListener('DOMContentLoaded', attach, { once: true });

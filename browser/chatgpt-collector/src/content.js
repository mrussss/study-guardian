import { parseConversation, visibleIdentities, turnsFromMessages } from './parser.js';
import { filterNewTurns } from './baseline.js';

let lastSnapshot = '';
let observer;
let baselineIdentities = new Set();
let baselineSettled = false;

function snapshotKey(conversation) {
  return JSON.stringify(conversation.messages.map(message => [message.external_message_id, message.content_hash]));
}

function scan() {
  const conversation = parseConversation(document);
  const snapshot = snapshotKey(conversation);
  if (snapshot === lastSnapshot) return;
  lastSnapshot = snapshot;
  if (!baselineSettled) {
    for (const identity of visibleIdentities(document)) baselineIdentities.add(identity);
    return;
  }
  const candidates = turnsFromMessages(conversation.messages)
    .filter(turn => filterNewTurns([turn], baselineIdentities).length > 0)
    .map(turn => ({ ...conversation, ...turn }));
  chrome.runtime.sendMessage({ type: 'turn_candidates', candidates });
}

function attach() {
  baselineIdentities = new Set(visibleIdentities(document));
  lastSnapshot = snapshotKey(parseConversation(document));
  baselineSettled = false;
  setTimeout(() => { baselineSettled = true; }, 1500);
  observer = new MutationObserver(() => {
    clearTimeout(observer.timer);
    observer.timer = setTimeout(scan, 1500);
  });
  observer.observe(document.body, { childList: true, subtree: true, characterData: true });
}

if (document.body) attach();
else window.addEventListener('DOMContentLoaded', attach, { once: true });

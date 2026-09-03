import { parseConversation, visibleIdentities, turnsFromMessages } from './parser.js';

let lastSnapshot = '';
let observer;

function scan() {
  const conversation = parseConversation(document);
  const snapshot = JSON.stringify(conversation.messages.map(message => [message.external_message_id, message.content_hash]));
  if (snapshot === lastSnapshot) return;
  lastSnapshot = snapshot;
  const candidates = turnsFromMessages(conversation.messages).map(turn => ({ ...conversation, ...turn }));
  chrome.runtime.sendMessage({ type: 'turn_candidates', candidates });
}

function attach() {
  chrome.runtime.sendMessage({ type: 'attach', identities: visibleIdentities(document) });
  lastSnapshot = JSON.stringify(parseConversation(document).messages.map(message => [message.external_message_id, message.content_hash]));
  observer = new MutationObserver(() => {
    clearTimeout(observer.timer);
    observer.timer = setTimeout(scan, 1500);
  });
  observer.observe(document.body, { childList: true, subtree: true, characterData: true });
}

if (document.body) attach();
else window.addEventListener('DOMContentLoaded', attach, { once: true });

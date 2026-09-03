const MAX_ITEMS = 1000;
const MAX_BYTES = 10 * 1024 * 1024;
const KEY = 'studyguardian_collector_queue';
let mutationChain = Promise.resolve();

function bytes(value) {
  return new TextEncoder().encode(JSON.stringify(value)).byteLength;
}

export async function readQueue(storage = chrome.storage.local) {
  const result = await storage.get({ [KEY]: [] });
  return Array.isArray(result[KEY]) ? result[KEY] : [];
}

export async function writeQueue(items, storage = chrome.storage.local) {
  await storage.set({ [KEY]: items });
}

function samePayload(left, right) {
  return JSON.stringify(left) === JSON.stringify(right);
}

function withMutationLock(operation) {
  const next = mutationChain.then(operation, operation);
  mutationChain = next.catch(() => {});
  return next;
}

export async function enqueue(payload, storage = chrome.storage.local) {
  return withMutationLock(async () => {
    let items = await readQueue(storage);
    items.push(payload);
    let dropped = 0;
    const overLimit = () => items.length > MAX_ITEMS || items.reduce((total, item) => total + bytes(item), 0) > MAX_BYTES;
    while (overLimit()) {
      const assistantIndex = items.findIndex(item => item?.messages?.some(message => message.role === 'assistant' && message.is_final));
      items.splice(assistantIndex >= 0 ? assistantIndex : 0, 1);
      dropped++;
    }
    await writeQueue(items, storage);
    return { depth: items.length, dropped };
  });
}

export async function peekQueue(storage = chrome.storage.local) {
  const items = await readQueue(storage);
  return items[0] || null;
}

export async function ackQueueHead(expected, storage = chrome.storage.local) {
  return withMutationLock(async () => {
    const items = await readQueue(storage);
    if (!items.length || !samePayload(items[0], expected)) return false;
    await writeQueue(items.slice(1), storage);
    return true;
  });
}

export async function dequeue(storage = chrome.storage.local) {
  return withMutationLock(async () => {
    const items = await readQueue(storage);
    const [head, ...rest] = items;
    await writeQueue(rest, storage);
    return head;
  });
}

export const queueLimits = { MAX_ITEMS, MAX_BYTES };

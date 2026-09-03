const KEY = 'studyguardian_collector_context';

export async function getContext(storage = chrome.storage.local) {
  const value = await storage.get({ [KEY]: null });
  return value[KEY];
}

export async function setContext(context, storage = chrome.storage.local) {
  await storage.set({ [KEY]: { ...context, status_observed_at: new Date().toISOString() } });
}

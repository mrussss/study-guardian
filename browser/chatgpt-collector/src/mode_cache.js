const KEY = 'studyguardian_collector_context';

export const SOFT_TTL_MS = 5_000;
export const HARD_TTL_MS = 15_000;

export async function getContext(storage = chrome.storage.local) {
  const value = await storage.get({ [KEY]: null });
  return value[KEY];
}

export async function setContext(context, storage = chrome.storage.local) {
  const value = { ...context, status_observed_at: new Date().toISOString() };
  await storage.set({ [KEY]: value });
  return value;
}

export function contextAgeMs(context, now = Date.now()) {
  if (!context?.status_observed_at) return Infinity;
  const observed = Date.parse(context.status_observed_at);
  if (!Number.isFinite(observed)) return Infinity;
  return Math.max(0, now - observed);
}

export function isSoftFresh(context, now = Date.now()) {
  return contextAgeMs(context, now) <= SOFT_TTL_MS;
}

export function isHardFresh(context, now = Date.now()) {
  return contextAgeMs(context, now) <= HARD_TTL_MS;
}

export async function resolveContextForNewTurn(refresh, storage = chrome.storage.local, now = Date.now()) {
  const cached = await getContext(storage);
  if (isSoftFresh(cached, now)) {
    return { context: cached, trustworthy: true };
  }
  try {
    const fresh = await refresh();
    return { context: fresh, trustworthy: true };
  } catch (error) {
    if (isHardFresh(cached, now)) {
      return { context: cached, trustworthy: true, degraded: true };
    }
    return {
      context: { user_mode: 'UNKNOWN', task: '' },
      trustworthy: false,
      error: String(error)
    };
  }
}

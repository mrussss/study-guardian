import { stableHash } from './parser.js';

const KEY = 'studyguardian_turn_contexts';
const MAX_AGE_MS = 24 * 60 * 60 * 1000;

export class TurnTracker {
  constructor(storage = chrome.storage.session, now = () => Date.now()) {
    this.storage = storage;
    this.now = now;
    this.contexts = new Map();
    this.loaded = false;
  }

  async load() {
    if (this.loaded) return;
    const value = await this.storage.get({ [KEY]: {} });
    const entries = Object.entries(value[KEY] || {});
    const cutoff = this.now() - MAX_AGE_MS;
    for (const [key, context] of entries) {
      const created = Date.parse(context?.created_at || '');
      if (Number.isFinite(created) && created >= cutoff) this.contexts.set(key, context);
    }
    this.loaded = true;
    if (this.contexts.size !== entries.length) await this.persist();
  }

  async persist() {
    await this.storage.set({ [KEY]: Object.fromEntries(this.contexts) });
  }

  async hasContext(turnKey) {
    await this.load();
    return this.contexts.has(turnKey);
  }

  async contextFor(turn, modeContext, options = {}) {
    await this.load();
    const key = turn.turn_key || stableHash(turn.user.content);
    if (!this.contexts.has(key)) {
      const mode = modeContext?.user_mode || 'UNKNOWN';
      const context = {
        mode_at_start: mode,
        task_at_start: modeContext?.task || '',
        eligible_for_review: options.allowReview !== false && mode === 'STUDY',
        created_at: new Date(this.now()).toISOString()
      };
      this.contexts.set(key, context);
      await this.persist();
    }
    return this.contexts.get(key);
  }
}

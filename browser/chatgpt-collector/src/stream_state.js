import { stableHash } from './parser.js';

export const ASSISTANT_STABILITY_MS = 1500;

export class MessageStateTracker {
  constructor(now = () => Date.now()) {
    this.now = now;
    this.states = new Map();
  }

  clear() {
    this.states.clear();
  }

  update(messages, now = this.now()) {
    return messages.map(message => {
      if (message.role !== 'assistant') {
        return { ...message, is_active: true, is_final: true, finalized_at: message.observed_at };
      }

      const key = message.external_message_id;
      const contentHash = message.content_hash || stableHash(message.content);
      let state = this.states.get(key);
      if (!state) {
        state = { last_content_hash: contentHash, last_changed_at: now, stable_scans: 0, finalized_at: null };
      } else if (state.last_content_hash !== contentHash) {
        state = { last_content_hash: contentHash, last_changed_at: now, stable_scans: 0, finalized_at: null };
      } else {
        state.stable_scans += 1;
      }

      const isFinal = state.stable_scans >= 1 && now - state.last_changed_at >= ASSISTANT_STABILITY_MS;
      if (isFinal && !state.finalized_at) state.finalized_at = new Date(now).toISOString();
      this.states.set(key, state);
      return {
        ...message,
        content_hash: contentHash,
        is_active: true,
        is_final: isFinal,
        finalized_at: isFinal ? state.finalized_at : null
      };
    });
  }
}

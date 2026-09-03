import { stableHash } from './parser.js';

export class TurnTracker {
  constructor() {
    this.baseline = new Set();
    this.attachEpoch = Date.now();
    this.contexts = new Map();
  }

  attach(identities) {
    this.attachEpoch = Date.now();
    this.baseline = new Set(identities);
  }

  isNew(identity) {
    return !this.baseline.has(identity);
  }

  hasContext(turnKey) {
    return this.contexts.has(turnKey);
  }

  contextFor(turn, modeContext) {
    const key = turn.turn_key || stableHash(turn.user.content);
    if (!this.contexts.has(key)) {
      const mode = modeContext?.user_mode || 'OFF';
      this.contexts.set(key, {
        mode_at_start: mode,
        task_at_start: modeContext?.task || '',
        eligible_for_review: mode === 'STUDY'
      });
    }
    return this.contexts.get(key);
  }
}

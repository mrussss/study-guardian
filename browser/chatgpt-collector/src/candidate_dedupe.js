import { stableHash } from './parser.js';

export function turnSnapshotHash(turn) {
  return stableHash(JSON.stringify({
    user: turn.user?.content_hash || stableHash(turn.user?.content || ''),
    assistants: (turn.assistants || []).map(message => ({
      id: message.external_message_id,
      hash: message.content_hash || stableHash(message.content || ''),
      active: Boolean(message.is_active),
      final: Boolean(message.is_final)
    }))
  }));
}

export class DeliveredState {
  constructor() {
    this.values = new Map();
  }

  clear() {
    this.values.clear();
  }

  hashFor(turn) {
    return turnSnapshotHash(turn);
  }

  hasSameSnapshot(turn) {
    return this.values.get(turn.turn_key) === this.hashFor(turn);
  }

  remember(turn) {
    this.values.set(turn.turn_key, this.hashFor(turn));
  }

  forget(turn) {
    if (this.hasSameSnapshot(turn)) this.values.delete(turn.turn_key);
  }
}

export function conversationKey(conversation) {
  return [conversation?.platform || '', conversation?.external_conversation_id || ''].join(':');
}

export class ConversationEpoch {
  constructor() {
    this.currentConversationKey = '';
    this.value = 0;
  }

  reset(conversation) {
    this.currentConversationKey = conversationKey(conversation);
    this.value += 1;
    return this.value;
  }

  observe(conversation) {
    const next = conversationKey(conversation);
    if (next === this.currentConversationKey) return false;
    this.reset(conversation);
    return true;
  }
}

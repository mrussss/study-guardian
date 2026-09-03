export function turnHasNewIdentity(turn, baseline) {
  return [turn.user, ...turn.assistants].some(message => !baseline.has(message.external_message_id));
}

export function filterNewTurns(turns, baseline) {
  return turns.filter(turn => turnHasNewIdentity(turn, baseline));
}

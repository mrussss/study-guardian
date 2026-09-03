import { getContext } from './mode_cache.js';

export async function buildTurnPayload(candidate, tracker, resolveContext, getCachedContext = getContext) {
  const isNewTurn = !(await tracker.hasContext(candidate.turn_key));
  const resolved = isNewTurn
    ? await resolveContext()
    : { context: await getCachedContext(), trustworthy: true };
  const frozen = await tracker.contextFor(candidate, resolved.context, {
    allowReview: resolved.trustworthy
  });
  const activeAssistant = (candidate.assistants || []).find(message => message.is_active);
  return {
    platform: candidate.platform,
    external_conversation_id: candidate.external_conversation_id,
    title: candidate.title,
    url: candidate.url,
    capture_policy: 'AUTO',
    external_turn_id: candidate.turn_key,
    turn_key: candidate.turn_key,
    observed_at: candidate.user.observed_at,
    ...frozen,
    active_branch_key: activeAssistant?.branch_key || candidate.active_branch_key || '',
    finalized: Boolean(activeAssistant?.is_final),
    messages: [candidate.user, ...(candidate.assistants || [])]
  };
}

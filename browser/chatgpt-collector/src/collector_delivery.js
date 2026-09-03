export async function deliverInOrder(payload, { flushQueue, postPayload, enqueue }) {
  const flushed = await flushQueue();
  if (!flushed || flushed.ok !== true) {
    await enqueue(payload);
    return { ok: false, queued: true, stage: 'flush' };
  }

  try {
    await postPayload(payload);
    return { ok: true, queued: false };
  } catch (error) {
    await enqueue(payload);
    return { ok: false, queued: true, stage: 'current', error: String(error) };
  }
}

export async function flushQueuedPayloads({ peekQueue, ackQueueHead, postPayload }) {
  while (true) {
    const payload = await peekQueue();
    if (!payload) return { ok: true };
    try {
      await postPayload(payload);
    } catch (error) {
      return { ok: false, error: String(error) };
    }
    if (!await ackQueueHead(payload)) {
      return { ok: false, error: 'queue head changed before acknowledgement' };
    }
  }
}

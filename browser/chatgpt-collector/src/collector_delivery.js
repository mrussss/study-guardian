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

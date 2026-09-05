export interface TaskPickerActionResult {
  ok: boolean;
  error_kind?: string;
  task?: string;
}

export type MutationQueueResult<T> =
  | { status: "completed"; value: T }
  | { status: "failed"; error: unknown }
  | { status: "superseded" };

export interface MutationQueueOptions {
  coalesceKey?: string;
}

type QueueEntry<T> = {
  operation: () => Promise<T>;
  coalesceKey?: string;
  resolve: (result: MutationQueueResult<T>) => void;
};

/** Runs one mutation at a time and keeps only the latest replaceable pending selection. */
export class TaskMutationQueue {
  private readonly pending: Array<QueueEntry<unknown>> = [];
  private running = false;

  enqueue<T>(operation: () => Promise<T>, options: MutationQueueOptions = {}): Promise<MutationQueueResult<T>> {
    return new Promise(resolve => {
      const entry: QueueEntry<T> = { operation, coalesceKey: options.coalesceKey, resolve };
      const last = this.pending[this.pending.length - 1];
      if (entry.coalesceKey !== undefined && last?.coalesceKey === entry.coalesceKey) {
        this.pending[this.pending.length - 1] = entry as QueueEntry<unknown>;
        last.resolve({ status: "superseded" });
      } else {
        this.pending.push(entry as QueueEntry<unknown>);
      }
      void this.pump();
    });
  }

  private async pump(): Promise<void> {
    if (this.running) return;
    this.running = true;
    try {
      while (this.pending.length > 0) {
        const entry = this.pending.shift();
        if (!entry) continue;
        try {
          entry.resolve({ status: "completed", value: await entry.operation() });
        } catch (error) {
          entry.resolve({ status: "failed", error });
        }
      }
    } finally {
      this.running = false;
      if (this.pending.length > 0) void this.pump();
    }
  }
}

export interface TaskPickerSettlement {
  applied: boolean;
  result: TaskPickerActionResult;
}

/** Ignores stale responses without inventing a server-confirmed task value. */
export function settleTaskPickerResult(activeRevision: number, resultRevision: number, result: TaskPickerActionResult): TaskPickerSettlement {
  return { applied: activeRevision === resultRevision, result };
}

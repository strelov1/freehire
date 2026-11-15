import type { Attachment } from '#lib/assistant/client.js';

export interface QueuedMessage {
  id: string;
  text: string;
  attachments: Attachment[];
}

/** Messages typed while a turn is in flight, drained one by one when it ends. */
export class MessageQueue {
  items = $state<QueuedMessage[]>([]);
  #counter = 0;

  get length() {
    return this.items.length;
  }

  add(text: string, attachments: Attachment[]) {
    this.items = [...this.items, { id: `q${++this.#counter}`, text, attachments }];
  }

  /** Take the oldest message off the queue. */
  shift(): QueuedMessage | undefined {
    const [next, ...rest] = this.items;
    this.items = rest;
    return next;
  }

  remove(id: string) {
    this.items = this.items.filter((q) => q.id !== id);
  }

  clear() {
    this.items = [];
  }
}

// Reactive single-shot loader shared by the "fetch once, then render" views (API
// keys, submissions, pipeline, the moderation/report queues). Owns the
// loading/error/ready status plus the loaded value; the view supplies the fetch and
// renders off `status`/`value`. A sibling of Paginator (multi-page) for the
// single-fetch case. Local edits after a create/revoke/resolve reassign `value`.

export class AsyncData<T> {
  status = $state<'loading' | 'error' | 'ready'>('loading');
  // Reassigned wholesale (fetch result, or a local edit), never mutated in place, so
  // raw skips the deep-proxy overhead — same rule as Paginator's items.
  value = $state.raw<T>() as T;

  constructor(initial: T) {
    this.value = initial;
  }

  /** Fetch once, tracking status. A failure flips to 'error' and keeps the current
   *  value (usually the initial empty/default state). */
  async run(fetch: () => Promise<T>): Promise<void> {
    this.status = 'loading';
    try {
      this.value = await fetch();
      this.status = 'ready';
    } catch {
      this.status = 'error';
    }
  }
}

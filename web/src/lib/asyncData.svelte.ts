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

  // Bumped on every run() so a slower earlier call can't overwrite a newer one —
  // the same reqToken/gen guard as HeaderSearch/SwipeDeck, applied here since a call
  // site can retry (or re-run on mount) while a prior run() is still in flight.
  #generation = 0;

  constructor(initial: T) {
    this.value = initial;
  }

  /** Fetch once, tracking status. A failure flips to 'error' and keeps the current
   *  value (usually the initial empty/default state). A run() superseded by a later
   *  one before it resolves is discarded instead of overwriting the fresher result. */
  async run(fetch: () => Promise<T>): Promise<void> {
    const mine = ++this.#generation;
    this.status = 'loading';
    try {
      const value = await fetch();
      if (mine !== this.#generation) return; // superseded by a newer run()
      this.value = value;
      this.status = 'ready';
    } catch {
      if (mine !== this.#generation) return;
      this.status = 'error';
    }
  }
}

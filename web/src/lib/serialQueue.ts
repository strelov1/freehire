// A one-at-a-time queue for writes that must each be built from the previous one's
// result. `PUT /me/profile` replaces the whole profile row, so two claims issued a
// moment apart would both assemble their payload from the same pre-claim skill list and
// the second would silently drop the first. Queueing makes each write read the store
// after its predecessor has applied.

/** Create a queue. Each call runs its job only once every job queued before it has
 *  settled, and resolves (or rejects) with that job's own outcome. A failing job rejects
 *  its own caller and does not wedge the ones behind it — a refused profile write must
 *  not disable the next claim. */
export function serialQueue(): <T>(job: () => Promise<T>) => Promise<T> {
  let tail: Promise<unknown> = Promise.resolve();
  return <T>(job: () => Promise<T>): Promise<T> => {
    // The chain swallows the rejection so the *queue* keeps moving; the promise handed
    // back is the un-swallowed one, so the caller still sees the failure.
    const run = tail.then(job, job);
    tail = run.catch(() => {});
    return run;
  };
}

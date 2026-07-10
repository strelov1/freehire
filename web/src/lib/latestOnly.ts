// Guards an out-of-order async race: each call supersedes the previous, so only the
// most recent fetch's result reaches `apply`. Used for live facet-count refreshes,
// where a slow earlier request must not overwrite a newer one with stale data. Errors
// are swallowed — a failed refresh is best-effort and leaves the last good value.
//
// Returns a zero-arg runner so call sites read like a plain `refresh()`; the fetch and
// the apply are captured once at construction.
export function latestOnly<T>(
  fetch: () => Promise<T>,
  apply: (value: T) => void,
): () => Promise<void> {
  let generation = 0;
  return () => {
    const gen = ++generation;
    return fetch()
      .then((value) => {
        if (gen === generation) apply(value);
      })
      .catch(() => {});
  };
}

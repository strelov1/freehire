/**
 * Calls `fn` once a burst of calls has gone quiet for `waitMs`.
 *
 * The page's form fires an event per keystroke; the panel wants one re-read when
 * the user stops, not sixty while they type. Split out of the content script so
 * the timing is testable without a page.
 */
export function debounce(fn: () => void, waitMs: number): () => void {
  let timer: ReturnType<typeof setTimeout> | undefined;
  return () => {
    if (timer !== undefined) clearTimeout(timer);
    timer = setTimeout(() => {
      timer = undefined;
      fn();
    }, waitMs);
  };
}

// Reactive paginator shared by views over any endpoint that can report whether
// more items remain. The fetch fn returns a `Slice` ({ items, hasMore }), so the
// "has more" rule — total count, short page, cursor — lives in the caller, not
// here. Owns the load / append / in-flight / error state; views only render.

import { ApiError, type Slice } from './api';

type FetchSlice<T> = (limit: number, offset: number) => Promise<Slice<T>>;

// A fetch rejected without an HTTP response — an AbortError from an in-flight
// navigation cancelling the request, or a transient network drop (iOS WebKit
// surfaces this as `TypeError: Load failed`, Chrome as `TypeError: Failed to
// fetch`). Unlike an `ApiError` (a real 4xx/5xx the server answered with), this
// isn't a genuine load failure: it's worth a silent retry so the feed self-heals
// instead of flashing "Failed to load jobs" when the user simply tapped a nav link.
function isTransient(err: unknown): boolean {
  return !(err instanceof ApiError);
}

/** Run `fetch`, silently retrying a transient (navigation-cancelled / network-drop)
 *  rejection up to `maxRetries` times with a short backoff; an `ApiError` — a real
 *  server response — or an exhausted budget rejects. Kept as a free function (no
 *  `$state`) so it's unit-testable without a Svelte runtime. */
export async function loadWithRetry<T>(fetch: () => Promise<T>, maxRetries = 2): Promise<T> {
  for (let attempt = 0; ; attempt++) {
    try {
      return await fetch();
    } catch (err) {
      if (!isTransient(err) || attempt >= maxRetries) throw err;
      await new Promise((resolve) => setTimeout(resolve, 200 * (attempt + 1)));
    }
  }
}

/** Drop items whose key is already in `seen`, adding the rest. Offset-based paging
 *  re-derives "page 2" as items.length, so a re-ranked or re-ingested result set
 *  between requests can shift an already-shown item into the next page's window —
 *  appended again under the same key, which Svelte's keyed `#each` rejects
 *  (svelte.dev/e/each_key_duplicate). `seen` is mutated in place (carried across
 *  calls by the caller) so a repeat is caught however many pages back it first
 *  appeared. Kept as a free function so it's unit-testable without a Svelte runtime. */
export function dedupeByKey<T>(items: T[], keyOf: (item: T) => unknown, seen: Set<unknown>): T[] {
  const fresh: T[] = [];
  for (const item of items) {
    const key = keyOf(item);
    if (seen.has(key)) continue;
    seen.add(key);
    fresh.push(item);
  }
  return fresh;
}

export class Paginator<T> {
  // Slices are whole API responses, only ever reassigned (never mutated in
  // place), so `raw` skips the per-item proxy overhead of deep `$state`.
  items = $state.raw<T[]>([]);
  status = $state<'loading' | 'error' | 'ready'>('loading');
  loadingMore = $state(false);
  // A failed `loadMore` surfaces here instead of flipping `status`, so the
  // already-loaded items stay on screen while the error shows by the button.
  loadMoreError = $state(false);
  hasMore = $state(false);
  // Total items matching the current query (the search engine's estimate); shown as
  // the result count and refreshed each page since the estimate can drift.
  total = $state(0);

  #fetch: FetchSlice<T>;
  #limit: number;
  // Only set when the caller has a stable identity for T; see `dedupeByKey`.
  #keyOf?: (item: T) => unknown;
  #seen = new Set<unknown>();

  constructor(fetch: FetchSlice<T>, opts: { limit?: number; keyOf?: (item: T) => unknown } = {}) {
    this.#fetch = fetch;
    this.#limit = opts.limit ?? 20;
    this.#keyOf = opts.keyOf;
  }

  #dedupe(slice: T[]): T[] {
    return this.#keyOf ? dedupeByKey(slice, this.#keyOf, this.#seen) : slice;
  }

  /** Seed the first page from data already fetched (e.g. server-rendered) so the
   *  view renders it immediately and only fetches on `loadMore`. Use instead of
   *  `start()` when the route's `load` has already produced page one. */
  seed(slice: Slice<T>) {
    this.#seen = new Set();
    this.items = this.#dedupe(slice.items);
    this.total = slice.total ?? 0;
    this.hasMore = slice.hasMore;
    this.status = 'ready';
  }

  /** Load the first page. Call once from the view's onMount (or an effect). A
   *  request cancelled by a navigation (or a transient network drop) isn't a real
   *  failure — retry a couple of times so the feed just reloads rather than showing
   *  an error; only a genuine server error (or exhausted retries) flips to 'error'. */
  async start() {
    try {
      const slice = await loadWithRetry(() => this.#fetch(this.#limit, 0));
      this.#seen = new Set();
      this.items = this.#dedupe(slice.items);
      this.total = slice.total ?? 0;
      this.hasMore = slice.hasMore;
      this.status = 'ready';
    } catch {
      this.status = 'error';
    }
  }

  /** Append the next page; no-op while one is in flight or none remain. */
  async loadMore() {
    if (this.loadingMore || !this.hasMore) return;
    this.loadingMore = true;
    this.loadMoreError = false;
    try {
      const slice = await this.#fetch(this.#limit, this.items.length);
      this.items = [...this.items, ...this.#dedupe(slice.items)];
      this.total = slice.total ?? 0;
      this.hasMore = slice.hasMore;
    } catch {
      this.loadMoreError = true;
    } finally {
      this.loadingMore = false;
    }
  }
}

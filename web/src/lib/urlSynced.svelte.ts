// A reactive search/filter model mirrored into the URL query string. Owns the
// state <-> URL transport and the reload debounce; it knows nothing about the API
// or what data a screen loads. Consumers bind an input to `value` (live) and
// reload off `applied` (the debounced snapshot) — see the jobs/companies/analytics
// views.
//
// Why client-driven (replaceState) and not goto: the URL is written
// *synchronously* on every change, so it is always in lockstep with the input.
// Because of that, `syncFromUrl` (browser back/forward) can never observe a URL
// lagging behind what the user just typed, so it can never revert in-flight
// characters. Only the data reload is debounced — never the URL write. This is the
// structural fix for the dropped-character race the goto+debounce design had.

import { untrack } from 'svelte';
import { browser } from '$app/environment';
import { page } from '$app/state';
import { replaceState } from '$app/navigation';

/** Translates the model to/from URL query params — the only screen-specific part. */
export interface UrlCodec<T> {
  parse: (params: URLSearchParams) => T;
  serialize: (value: T) => URLSearchParams;
}

/** Re-seed a synced state from the URL on browser back/forward. Call once during a
 *  component's init. Tracks only the URL: `syncFromUrl` reads the state's own
 *  `value`, so untrack stops this from looping on our synchronous URL writes (and
 *  it no-ops when already in sync, which is exactly that case). The state's
 *  resulting `applied` change is what a consumer's reload effect then watches. */
export function syncOnNavigation(state: { syncFromUrl: () => void }): void {
  $effect(() => {
    void page.url.search; // track
    untrack(() => state.syncFromUrl());
  });
}

export class UrlSyncedState<T> {
  /** Live model — bind inputs to this; every keystroke updates it synchronously. */
  value = $state<T>(undefined as T);
  /** Debounced snapshot of `value` — the sole signal a consumer watches to reload. */
  applied = $state<T>(undefined as T);

  #codec: UrlCodec<T>;
  #debounceMs: number;
  // The one timer in the system: coalesces `value` -> `applied` so typing doesn't
  // reload per keystroke. `undefined` means no reload is queued (back/forward relies
  // on this being truthful — see syncFromUrl).
  #timer: ReturnType<typeof setTimeout> | undefined;

  /** Seed from the current URL params (passed by the view from `page.url`), so the
   *  same state renders on the server and hydrates on the client. */
  constructor(initial: URLSearchParams, codec: UrlCodec<T>, debounceMs = 300) {
    this.#codec = codec;
    this.#debounceMs = debounceMs;
    // The field initializers above are typed placeholders; both are seeded here
    // from the URL params. On the client the browser's address bar
    // (location.search) is the authoritative filter state: after a back/forward
    // onto a shallow-routing (replaceState) entry, SvelteKit's page.url — which the
    // view reads to pass `initial` — can lag to the pre-filter URL while the address
    // bar still shows the filter. Seeding from location makes a restored view mirror
    // the real URL. On the server location is unavailable; the passed page.url params
    // are correct there and match the client for every non-shallow navigation.
    const params = browser ? new URLSearchParams(location.search) : initial;
    const seeded = codec.parse(params);
    this.value = seeded;
    this.applied = seeded;
  }

  /** Discrete change (facet pill, checkbox, clear): write the URL and apply at once.
   *  A click is never typed fast, so debouncing it would only add latency. */
  setNow(next: T) {
    clearTimeout(this.#timer);
    this.#timer = undefined;
    this.#write(next);
    this.applied = next;
  }

  /** Continuous input (free text, slider): write the URL synchronously, debounce the
   *  reload. `value` stays live so the input never lags; `applied` follows after the
   *  quiet window. */
  setSoon(next: T) {
    this.#write(next);
    clearTimeout(this.#timer);
    this.#timer = setTimeout(() => {
      this.#timer = undefined;
      this.applied = this.value;
    }, this.#debounceMs);
  }

  /** Re-read from the current URL (browser back/forward). No-op when already in sync,
   *  which also breaks the write-back loop after our own replaceState. Because URL
   *  writes are synchronous, a mismatch here is a genuine external navigation — never
   *  a stale-our-own-commit, so reseeding can't eat in-flight typing. */
  syncFromUrl() {
    // Read the browser's address bar, not page.url: after a shallow-routing (replaceState)
    // back/forward, page.url lags to the pre-filter URL while location.search is correct.
    // Reading page.url here would revert the constructor's location-seeded value to empty.
    const current = browser ? new URLSearchParams(location.search) : page.url.searchParams;
    if (current.toString() === this.#codec.serialize(this.value).toString()) return;
    clearTimeout(this.#timer);
    this.#timer = undefined;
    const seeded = this.#codec.parse(current);
    this.value = seeded;
    this.applied = seeded;
  }

  /** Cancel a pending reload — call from the owning view's cleanup so a late apply
   *  can't fire after unmount. */
  dispose() {
    clearTimeout(this.#timer);
    this.#timer = undefined;
  }

  #write(next: T) {
    this.value = next;
    const qs = this.#codec.serialize(next).toString();
    // Shallow routing: updates the URL in place without a navigation or load, so the
    // write is cheap and synchronous. Browser-only (mutations are user events).
    // eslint-disable-next-line svelte/no-navigation-without-resolve -- in-place query write to the current pathname; there is no route to resolve
    replaceState(page.url.pathname + (qs ? `?${qs}` : ''), {});
  }
}

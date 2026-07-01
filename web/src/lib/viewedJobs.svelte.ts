// Tracks which jobs the signed-in user has already viewed, so the browse list
// and search results can dim already-seen cards. The set of viewed public_slugs
// is read once from GET /api/v1/me/jobs/viewed (the browse view triggers the
// load); recording a view on a job detail page marks its slug locally too, so a
// card dims on back-navigation without waiting for a reload.
//
// SSR-safe and auth-agnostic: `ensureLoaded` is a no-op off the browser, and the
// set simply stays empty for signed-out users (callers gate the load on auth, so
// no user lookup lives here). A failed load leaves the set empty — nothing dims,
// which is the correct degraded state.

import { SvelteSet } from 'svelte/reactivity';
import { browser } from '$app/environment';
import { listViewedSlugs } from '$lib/api';

class ViewedJobs {
  // SvelteSet (not a plain Set): a plain Set in $state is not deeply reactive, so
  // an in-place `.add` in `mark` would not re-run readers. SvelteSet makes both
  // the `.add` mutation and the `ensureLoaded` reassignment trigger dependent
  // $derived/$effect (e.g. JobRow's `isViewed`).
  #slugs = $state(new SvelteSet<string>());
  #loaded = false;
  // The in-flight load, shared so concurrent callers issue one request.
  #loading: Promise<void> | null = null;
  // Bumped by reset(); a load that resolves after a reset (a same-tab user handoff)
  // is discarded instead of repopulating the fresh set.
  #generation = 0;

  has(slug: string): boolean {
    return this.#slugs.has(slug);
  }

  /** Mark a slug viewed locally (e.g. right after recording a view), so its card
   *  dims immediately without re-fetching the whole set. */
  mark(slug: string) {
    this.#slugs.add(slug);
  }

  /** Load the viewed set once. Repeat calls reuse the first load (or its
   *  in-flight promise). No-op on the server. */
  async ensureLoaded(): Promise<void> {
    if (!browser || this.#loaded) return;
    if (this.#loading) return this.#loading;
    const gen = this.#generation;
    this.#loading = listViewedSlugs()
      .then((slugs) => {
        if (gen !== this.#generation) return; // reset() ran mid-load — discard stale slugs.
        this.#slugs = new SvelteSet(slugs);
        this.#loaded = true;
      })
      .catch(() => {
        // best-effort: an unreachable/failed load just means nothing dims.
      })
      .finally(() => {
        if (gen === this.#generation) this.#loading = null;
      });
    return this.#loading;
  }

  /** Drop the viewed set on sign-out so a different user signing in on the same tab
   *  can't see the previous user's viewed jobs dimmed — the store is a per-user module
   *  singleton that survives the soft invalidateAll() on logout. */
  reset() {
    this.#generation++;
    this.#slugs = new SvelteSet();
    this.#loaded = false;
    this.#loading = null;
  }
}

const viewedJobs = new ViewedJobs();

export function hasViewed(slug: string): boolean {
  return viewedJobs.has(slug);
}

export function markViewed(slug: string) {
  viewedJobs.mark(slug);
}

export function ensureViewedLoaded(): Promise<void> {
  return viewedJobs.ensureLoaded();
}

export function resetViewedJobs() {
  viewedJobs.reset();
}

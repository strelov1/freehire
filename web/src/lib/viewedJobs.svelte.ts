// Tracks which jobs the signed-in user has already viewed, so the browse list
// and search results can dim already-seen cards. The set of viewed public_slugs
// is read once from GET /api/v1/me/jobs/viewed (the browse view triggers the
// load); recording a view on a job detail page marks its slug locally too, so a
// card dims on back-navigation without waiting for a reload.
//
// SSR-safe and auth-agnostic (see UserResource): the load is a browser-only no-op
// and the set stays empty for signed-out users. A failed load leaves the set empty —
// nothing dims, the correct degraded state.

import { SvelteSet } from 'svelte/reactivity';
import { api } from '$lib/api';
import { UserResource } from '$lib/userResource.svelte';

class ViewedJobs extends UserResource<string[]> {
  // SvelteSet (not a plain Set): a plain Set in $state is not deeply reactive, so
  // an in-place `.add` in `mark` would not re-run readers. SvelteSet makes both
  // the `.add` mutation and the load reassignment trigger dependent
  // $derived/$effect (e.g. JobRow's `isViewed`).
  #slugs = $state(new SvelteSet<string>());

  has(slug: string): boolean {
    return this.#slugs.has(slug);
  }

  /** Mark a slug viewed locally (e.g. right after recording a view), so its card
   *  dims immediately without re-fetching the whole set. */
  mark(slug: string) {
    this.#slugs.add(slug);
  }

  protected load(): Promise<string[]> {
    return api.listViewedSlugs();
  }

  protected apply(slugs: string[]) {
    this.#slugs = new SvelteSet(slugs);
  }

  protected clearState() {
    this.#slugs = new SvelteSet();
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

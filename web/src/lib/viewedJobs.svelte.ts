// Tracks which jobs the signed-in user has already viewed, so the browse list
// and search results can dim already-seen cards. The set of viewed public_slugs
// is read once from GET /api/v1/me/tracking/viewed (the browse view triggers the
// load); recording a view on a job detail page marks its slug locally too, so a
// card dims on back-navigation without waiting for a reload.
//
// SSR-safe and auth-agnostic (see UserResource): the load is a browser-only no-op
// and the set stays empty for signed-out users. A failed load leaves the set empty —
// nothing dims, the correct degraded state.

import { api } from '$lib/api';
import { SlugSet } from '$lib/userResource.svelte';

class ViewedJobs extends SlugSet {
  protected load(): Promise<string[]> {
    return api.listViewedSlugs();
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

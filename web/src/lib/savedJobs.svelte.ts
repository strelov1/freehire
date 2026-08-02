// Tracks which jobs the signed-in user has saved (bookmarked), so the browse list
// and search results can render the save toggle as already-filled. The set of
// saved public_slugs is read once from GET /api/v1/me/tracking/saved (the browse
// view triggers the load); toggling save on a card updates the set locally so the
// bookmark reflects the change immediately, without waiting for a reload.
//
// Sibling of viewedJobs.svelte.ts, with one difference: saving is a two-way toggle,
// so this store both adds (mark) and removes (unmark), whereas a view is only ever
// added.
//
// SSR-safe and auth-agnostic (see UserResource): the load is a browser-only no-op
// and the set stays empty for signed-out users. A failed load leaves the set empty —
// nothing shows filled, the correct degraded state.

import { api } from '$lib/api';
import { SlugSet } from '$lib/userResource.svelte';

class SavedJobs extends SlugSet {
  protected load(): Promise<string[]> {
    return api.listSavedSlugs();
  }
}

const savedJobs = new SavedJobs();

export function isSaved(slug: string): boolean {
  return savedJobs.has(slug);
}

export function markSaved(slug: string) {
  savedJobs.mark(slug);
}

export function markUnsaved(slug: string) {
  savedJobs.unmark(slug);
}

export function ensureSavedLoaded(): Promise<void> {
  return savedJobs.ensureLoaded();
}

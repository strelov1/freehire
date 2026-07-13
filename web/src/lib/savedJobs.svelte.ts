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

import { SvelteSet } from 'svelte/reactivity';
import { api } from '$lib/api';
import { UserResource } from '$lib/userResource.svelte';

class SavedJobs extends UserResource<string[]> {
  // SvelteSet (not a plain Set): a plain Set in $state is not deeply reactive, so
  // an in-place `.add`/`.delete` would not re-run readers. SvelteSet makes both the
  // mutation and the load reassignment trigger dependent $derived/$effect (e.g.
  // JobRow's `saved`).
  #slugs = $state(new SvelteSet<string>());

  has(slug: string): boolean {
    return this.#slugs.has(slug);
  }

  /** Mark a slug saved locally (e.g. right after a successful save), so its card's
   *  bookmark fills immediately without re-fetching the whole set. */
  mark(slug: string) {
    this.#slugs.add(slug);
  }

  /** Clear a slug's saved mark locally (e.g. right after a successful unsave). */
  unmark(slug: string) {
    this.#slugs.delete(slug);
  }

  protected load(): Promise<string[]> {
    return api.listSavedSlugs();
  }

  protected apply(slugs: string[]) {
    this.#slugs = new SvelteSet(slugs);
  }

  protected clearState() {
    this.#slugs = new SvelteSet();
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

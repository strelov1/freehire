// Tracks which jobs the signed-in user has hidden (dismissed), so the browse list
// and search results can exclude them from the feed. The set of dismissed
// public_slugs is read once from GET /api/v1/me/tracking/dismissed (the browse
// view triggers the load); hiding a job from a card updates the set locally so it
// drops out of the feed immediately, and undo puts it back — both without a reload.
//
// Sibling of savedJobs.svelte.ts: same two-way toggle shape, different mark. A
// dismissed slug means "keep this out of my feed".
//
// SSR-safe and auth-agnostic (see UserResource): the load is a browser-only no-op
// and the set stays empty for signed-out users. A failed load leaves the set empty —
// nothing is excluded, the correct degraded state.

import { SvelteSet } from 'svelte/reactivity';
import { api } from '$lib/api';
import { UserResource } from '$lib/userResource.svelte';

class DismissedJobs extends UserResource<string[]> {
  // SvelteSet (not a plain Set): a plain Set in $state is not deeply reactive, so
  // an in-place `.add`/`.delete` would not re-run readers. SvelteSet makes both the
  // mutation and the load reassignment trigger dependent $derived/$effect (e.g.
  // the feed's dismissed exclusion).
  #slugs = $state(new SvelteSet<string>());

  has(slug: string): boolean {
    return this.#slugs.has(slug);
  }

  /** Mark a slug hidden locally (e.g. right after a successful dismiss), so its
   *  card drops out of the feed immediately without re-fetching the whole set. */
  mark(slug: string) {
    this.#slugs.add(slug);
  }

  /** Clear a slug's hidden mark locally (e.g. right after a successful undo). */
  unmark(slug: string) {
    this.#slugs.delete(slug);
  }

  protected load(): Promise<string[]> {
    return api.listDismissedSlugs();
  }

  protected apply(slugs: string[]) {
    this.#slugs = new SvelteSet(slugs);
  }

  protected clearState() {
    this.#slugs = new SvelteSet();
  }
}

const dismissedJobs = new DismissedJobs();

export function isDismissed(slug: string): boolean {
  return dismissedJobs.has(slug);
}

export function markDismissed(slug: string) {
  dismissedJobs.mark(slug);
}

export function markUndismissed(slug: string) {
  dismissedJobs.unmark(slug);
}

export function ensureDismissedLoaded(): Promise<void> {
  return dismissedJobs.ensureLoaded();
}

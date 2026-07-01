// The signed-in user's search profiles — named specialization + skills sets. The
// list is read once from GET /api/v1/me/profiles; create/update/delete call the API
// and keep the local list in sync, newest-first, so the view updates without a reload.
//
// SSR-safe and auth-agnostic: `ensureLoaded` is a no-op off the browser, and the list
// simply stays empty for signed-out users (the view gates the load on auth and renders
// a sign-in prompt instead). Mutations surface API errors to the caller (a duplicate
// name or the per-user cap is a 409, a bad specialization or empty skills a 400) so the
// UI can show them. Mirrors savedSearches.svelte.ts.

import { browser } from '$app/environment';
import {
  listSearchProfiles,
  createSearchProfile,
  updateSearchProfile,
  deleteSearchProfile,
} from '$lib/api';
import type { SearchProfile } from '$lib/types';

class SearchProfiles {
  // Reassigned (never mutated in place) on every change, so $state.raw is enough and
  // readers ($derived in the view) re-run on each new array.
  #items = $state.raw<SearchProfile[]>([]);
  #loaded = false;
  // The in-flight load, shared so concurrent callers issue one request.
  #loading: Promise<void> | null = null;
  // Bumped by reset(); a load resolving after a reset (a same-tab user handoff) is
  // discarded instead of repopulating the list with the previous user's profiles.
  #generation = 0;

  get items(): SearchProfile[] {
    return this.#items;
  }

  /** Load the list once. Repeat calls reuse the first load (or its in-flight
   *  promise). No-op on the server. A failed load leaves the list empty. */
  async ensureLoaded(): Promise<void> {
    if (!browser || this.#loaded) return;
    if (this.#loading) return this.#loading;
    const gen = this.#generation;
    this.#loading = listSearchProfiles()
      .then((rows) => {
        if (gen !== this.#generation) return; // reset() ran mid-load — discard stale rows.
        this.#items = rows;
        this.#loaded = true;
      })
      .catch(() => {
        // best-effort: a failed load just means the list is empty.
      })
      .finally(() => {
        if (gen === this.#generation) this.#loading = null;
      });
    return this.#loading;
  }

  /** Create a profile and prepend it (newest-first). Throws on a duplicate name, the
   *  per-user cap, a bad specialization, or empty skills (the caller shows the error). */
  async create(name: string, specializations: string[], skills: string[]): Promise<SearchProfile> {
    const row = await createSearchProfile(name, specializations, skills);
    this.#items = [row, ...this.#items];
    return row;
  }

  /** Overwrite a profile's name, specializations, and/or skills; move it to the front
   *  (it is now the most recently updated, matching the server's ordering). */
  async update(
    id: number,
    patch: { name?: string; specializations?: string[]; skills?: string[] },
  ): Promise<SearchProfile> {
    const row = await updateSearchProfile(id, patch);
    this.#items = [row, ...this.#items.filter((p) => p.id !== id)];
    return row;
  }

  /** Delete a profile and drop it from the list. */
  async remove(id: number): Promise<void> {
    await deleteSearchProfile(id);
    this.#items = this.#items.filter((p) => p.id !== id);
  }

  /** Drop the cached list and the loaded flag. The view calls this when the session
   *  ends, so a different user signing in on the same tab loads their own profiles. */
  reset() {
    this.#generation++;
    this.#items = [];
    this.#loaded = false;
    this.#loading = null;
  }
}

export const searchProfiles = new SearchProfiles();

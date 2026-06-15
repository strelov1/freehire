// The signed-in user's saved searches — named snapshots of the job-search filter
// state. The list is read once from GET /api/v1/me/searches (the filters panel
// triggers the load for an authenticated user); create/update/delete call the API
// and keep the local list in sync, newest-first, so the picker updates without a
// reload.
//
// SSR-safe and auth-agnostic: `ensureLoaded` is a no-op off the browser, and the
// list simply stays empty for signed-out users (the component gates the load on
// auth and renders a sign-in prompt instead). Mutations surface API errors to the
// caller (a duplicate name or the per-user cap is a 409) so the UI can show them.

import { browser } from '$app/environment';
import { listSavedSearches, createSavedSearch, updateSavedSearch, deleteSavedSearch } from '$lib/api';
import type { SavedSearch } from '$lib/types';

class SavedSearches {
  // Reassigned (never mutated in place) on every change, so $state.raw is enough
  // and readers ($derived in the component) re-run on each new array.
  #items = $state.raw<SavedSearch[]>([]);
  #loaded = false;
  // The in-flight load, shared so concurrent callers issue one request.
  #loading: Promise<void> | null = null;

  get items(): SavedSearch[] {
    return this.#items;
  }

  /** Load the list once. Repeat calls reuse the first load (or its in-flight
   *  promise). No-op on the server. A failed load leaves the list empty. */
  async ensureLoaded(): Promise<void> {
    if (!browser || this.#loaded) return;
    if (this.#loading) return this.#loading;
    this.#loading = listSavedSearches()
      .then((rows) => {
        this.#items = rows;
        this.#loaded = true;
      })
      .catch(() => {
        // best-effort: a failed load just means the picker is empty.
      })
      .finally(() => {
        this.#loading = null;
      });
    return this.#loading;
  }

  /** Save the current filters under a name; prepend the new set (newest-first).
   *  Throws on a duplicate name or the per-user cap (the caller shows the error). */
  async create(name: string, query: string): Promise<SavedSearch> {
    const row = await createSavedSearch(name, query);
    this.#items = [row, ...this.#items];
    return row;
  }

  /** Overwrite a set's name and/or query; move it to the front (it is now the
   *  most recently updated, matching the server's ordering). */
  async update(id: number, patch: { name?: string; query?: string }): Promise<SavedSearch> {
    const row = await updateSavedSearch(id, patch);
    this.#items = [row, ...this.#items.filter((s) => s.id !== id)];
    return row;
  }

  /** Delete a set and drop it from the list. */
  async remove(id: number): Promise<void> {
    await deleteSavedSearch(id);
    this.#items = this.#items.filter((s) => s.id !== id);
  }

  /** Drop the cached list and the loaded flag. The component calls this when the
   *  session ends, so a different user signing in on the same tab loads their own
   *  searches instead of seeing the previous user's (the list is per-user). */
  reset() {
    this.#items = [];
    this.#loaded = false;
    this.#loading = null;
  }
}

export const savedSearches = new SavedSearches();

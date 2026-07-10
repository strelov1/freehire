// The signed-in user's saved searches — named snapshots of the job-search filter
// state. The list is read once from GET /api/v1/me/searches (the filters panel
// triggers the load for an authenticated user); create/update/delete call the API
// and keep the local list in sync, newest-first, so the picker updates without a
// reload.
//
// SSR-safe and auth-agnostic (see UserResource): the load is a browser-only no-op and
// the list stays empty for signed-out users. Mutations surface API errors to the
// caller (a duplicate name or the per-user cap is a 409) so the UI can show them.

import { api } from '$lib/api';
import { UserResource } from '$lib/userResource.svelte';
import type { SavedSearch } from '$lib/types';

class SavedSearches extends UserResource<SavedSearch[]> {
  // Reassigned (never mutated in place) on every change, so $state.raw is enough
  // and readers ($derived in the component) re-run on each new array.
  #items = $state.raw<SavedSearch[]>([]);

  get items(): SavedSearch[] {
    return this.#items;
  }

  protected load(): Promise<SavedSearch[]> {
    return api.listSavedSearches();
  }

  protected apply(rows: SavedSearch[]) {
    this.#items = rows;
  }

  protected clearState() {
    this.#items = [];
  }

  /** Save the current filters under a name; prepend the new set (newest-first).
   *  Throws on a duplicate name or the per-user cap (the caller shows the error). */
  async create(name: string, query: string): Promise<SavedSearch> {
    const row = await api.createSavedSearch(name, query);
    this.#items = [row, ...this.#items];
    return row;
  }

  /** Overwrite a set's name and/or query; move it to the front (it is now the
   *  most recently updated, matching the server's ordering). */
  async update(id: number, patch: { name?: string; query?: string }): Promise<SavedSearch> {
    const row = await api.updateSavedSearch(id, patch);
    this.#items = [row, ...this.#items.filter((s) => s.id !== id)];
    return row;
  }

  /** Delete a set and drop it from the list. */
  async remove(id: number): Promise<void> {
    await api.deleteSavedSearch(id);
    this.#items = this.#items.filter((s) => s.id !== id);
  }

  /** Publish a set as a public board and replace it in place (keeping its position, so
   *  toggling share doesn't reorder the list). Returns the updated set with its slug. */
  async share(id: number, authorLabel = ''): Promise<SavedSearch> {
    const row = await api.shareSavedSearch(id, authorLabel);
    this.#items = this.#items.map((s) => (s.id === id ? row : s));
    return row;
  }

  /** Make a shared set private again and clear its board fields in place. */
  async unshare(id: number): Promise<void> {
    await api.unshareSavedSearch(id);
    this.#items = this.#items.map((s) =>
      s.id === id ? { ...s, public_slug: '', author_label: '' } : s,
    );
  }
}

export const savedSearches = new SavedSearches();

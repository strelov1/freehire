// What the match feed matches on: the profile or one of the user's saved searches. The server
// stores the choice (PUT /me/feed/source) and echoes it in every feed page's meta, so the
// current source is set from meta, never guessed. Picking one bumps `version`; the feed is
// keyed on it, so it remounts and reloads from its first card.

import { api } from '#lib/api.js';
import type { FeedMeta, SavedSearch } from '#lib/types.js';

export type FeedSourceChoice = { source: 'profile' } | { source: 'saved_search'; saved_search_id: number };

class FeedSource {
  source = $state<string>('profile');
  savedSearchId = $state<number | undefined>();
  version = $state(0);
  searches = $state<SavedSearch[]>([]);
  status = $state<'idle' | 'loading' | 'ready' | 'failed'>('idle');
  // Set by a pick so a remounted pill (swipe mode) takes focus back.
  refocus = false;

  /** The label of the current source. A search that is gone from the list reads generically. */
  get label(): string {
    if (this.source !== 'saved_search') return 'By my profile';
    return this.searches.find((s) => s.id === this.savedSearchId)?.name ?? 'Saved search';
  }

  sync(meta: FeedMeta | null | undefined) {
    if (!meta) return;
    this.source = meta.source;
    this.savedSearchId = meta.saved_search_id;
  }

  /** The user's own searches: the profile-derived alert search is never a source. */
  async loadSearches() {
    this.status = 'loading';
    try {
      this.searches = (await api.listSavedSearches()).filter((s) => !s.derived_from_profile);
      this.status = 'ready';
    } catch {
      this.status = 'failed';
    }
  }

  /** Stores the choice, then remounts the feed. Throws when the server refuses. */
  async choose(choice: FeedSourceChoice) {
    await api.setFeedSource(choice);
    this.source = choice.source;
    this.savedSearchId = choice.source === 'saved_search' ? choice.saved_search_id : undefined;
    this.refocus = true;
    this.version++;
    setTimeout(() => (this.refocus = false)); // the remount runs in the microtask flush before this
  }
}

export const feedSource = new FeedSource();

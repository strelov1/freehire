import { untrack } from 'svelte';
import { api } from '#lib/api.js';
import type { FeedMeta, FeedSourceChoice, SavedSearch } from '#lib/types.js';
import { UserResource } from '#lib/userResource.svelte.js';

type FeedMode = 'list' | 'swipe';

/**
 * What the match feed matches on, shared by the pill, the menu and both feed modes. The
 * server stores the choice; this mirrors it from the feed's `meta` so a reload shows the
 * stored source. `version` bumps on every pick so the feed remounts from its first card.
 */
class FeedSource extends UserResource<SavedSearch[]> {
  /** The server's current source, from the last feed response; null before the first. */
  current = $state<{ kind: string; id?: number } | null>(null);
  /** Bumped by choose(): key the feed on it. */
  version = $state(0);
  /** The user's own saved searches, never the profile-derived one. */
  searches = $state.raw<SavedSearch[]>([]);
  /** A PUT is in flight: picks wait, so the UI order is the persisted order. */
  busy = $state(false);
  status = $state<'idle' | 'loading' | 'failed' | 'ready'>('idle');
  #cardFocus: { version: number; mode: FeedMode } | undefined;

  fromMeta(meta: FeedMeta) {
    this.current = { kind: meta.source, id: meta.saved_search_id };
  }

  protected load(): Promise<SavedSearch[]> {
    return api.listSavedSearches();
  }

  protected apply(searches: SavedSearch[]): void {
    this.searches = searches.filter((search) => !search.derived_from_profile);
  }

  protected clearState(): void {
    this.current = null;
    this.searches = [];
    this.busy = false;
    this.status = 'idle';
    this.#cardFocus = undefined;
    untrack(() => this.version++);
  }

  async loadSearches() {
    if (this.status === 'loading') return;
    const generation = this.generation;
    this.status = 'loading';
    try {
      if (await this.reload()) this.status = 'ready';
    } catch {
      if (generation === this.generation) this.status = 'failed';
    }
  }

  /** Stores the choice, then restarts the feed. False when the server refused it, or a
   *  previous pick is still being stored (one PUT at a time). */
  async choose(choice: FeedSourceChoice, focusCardIn?: FeedMode): Promise<boolean> {
    if (this.busy) return false;
    const generation = this.generation;
    this.busy = true;
    try {
      await api.setFeedSource(choice);
    } catch {
      return false;
    } finally {
      if (generation === this.generation) this.busy = false;
    }
    if (generation !== this.generation) return false;
    this.current =
      choice.source === 'profile' ? { kind: 'profile' } : { kind: 'saved_search', id: choice.saved_search_id };
    this.#cardFocus = focusCardIn ? { version: this.version + 1, mode: focusCardIn } : undefined;
    this.version++;
    return true;
  }

  takeCardFocus(version: number, mode: FeedMode): boolean {
    if (this.#cardFocus?.version !== version || this.#cardFocus.mode !== mode) return false;
    this.#cardFocus = undefined;
    return true;
  }

  /** The pill's source name. */
  get label(): string {
    if (!this.current) return 'Loading…'; // unknown until the first feed response
    if (this.current.kind !== 'saved_search') return 'By my profile';
    return this.searches.find((s) => s.id === this.current?.id)?.name ?? 'Saved search';
  }
}

export const feedSource = new FeedSource();

// A tiny non-reactive fetch cache for the chat's job cards, shared across every
// card on the page so a repeated slug (same or later message) is fetched once.
// Lives in a plain module (not a component) so it's a plain Map, not reactive
// state.

import { api } from '$lib/api';
import type { Job } from '$lib/types';

// A long-running session (many turns, each surfacing its own search results and
// job decks) can name far more distinct slugs than are worth holding onto at
// once. Only a failed fetch was ever evicted, so a resolved entry sat in the map
// for the rest of the session — capping the size and evicting the
// least-recently-used entry keeps this a cache rather than an unbounded log of
// every job the chat has ever shown.
export const MAX_ENTRIES = 200;

const cache = new Map<string, Promise<Job>>();

/** Fetch a job's structured jobview by slug, deduped by slug. A rejected fetch
 *  is evicted so a later render can retry instead of being stuck on the link
 *  fallback for the whole session. */
export function loadJob(slug: string): Promise<Job> {
  const existing = cache.get(slug);
  if (existing) {
    // Re-insert to mark it most-recently-used — Map iterates in insertion order,
    // which is what the eviction below relies on.
    cache.delete(slug);
    cache.set(slug, existing);
    return existing;
  }

  const p = api.getJob(slug);
  p.catch(() => {
    if (cache.get(slug) === p) cache.delete(slug);
  });
  if (cache.size >= MAX_ENTRIES) {
    const oldest = cache.keys().next().value;
    if (oldest !== undefined) cache.delete(oldest);
  }
  cache.set(slug, p);
  return p;
}

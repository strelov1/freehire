// A tiny non-reactive fetch cache for job-card unfurls, shared across every card
// on the page so a repeated slug (same or later message) is fetched once. Lives
// in a plain module (not a component) so it's a plain Map, not reactive state.

import { api } from '$lib/api';
import type { Job } from '$lib/types';

const cache = new Map<string, Promise<Job>>();

/** Fetch a job's structured jobview by slug, deduped by slug. A rejected fetch
 *  is evicted so a later render can retry instead of being stuck on the link
 *  fallback for the whole session. */
export function loadJob(slug: string): Promise<Job> {
  let p = cache.get(slug);
  if (!p) {
    p = api.getJob(slug);
    p.catch(() => {
      if (cache.get(slug) === p) cache.delete(slug);
    });
    cache.set(slug, p);
  }
  return p;
}

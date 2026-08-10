// A small non-reactive fetch cache for the chat's job cards, shared across every
// card in the panel so a repeated slug (same or later message) is fetched once.
// Ported from the web app's `jobCache.ts`; a plain module, not reactive state, so
// it stays a plain Map.
//
// Divergence from the web's copy: the web's `api` carries the session cookie
// implicitly, and extension code has no cookie to carry. The token is read here
// rather than threaded through every card, keeping `loadJob(slug)` the same shape
// the web's components call.

import { getToken } from '../auth';
import { getJob, type FreehireJob } from '../freehire';

const cache = new Map<string, Promise<FreehireJob>>();

/** Fetch a job by slug, deduped by slug. A rejected fetch is evicted so a later
 *  render can retry instead of being stuck on the fallback for the whole session. */
export function loadJob(slug: string): Promise<FreehireJob> {
  let p = cache.get(slug);
  if (!p) {
    p = fetchJob(slug);
    p.catch(() => {
      if (cache.get(slug) === p) cache.delete(slug);
    });
    cache.set(slug, p);
  }
  return p;
}

async function fetchJob(slug: string): Promise<FreehireJob> {
  const token = await getToken();
  if (!token) throw new Error('not signed in');
  return getJob(slug, token);
}

/** Drop everything cached. Exists for tests; the panel has no reason to call it,
 *  since a vacancy's facts do not change within one sitting. */
export function resetJobCache(): void {
  cache.clear();
}

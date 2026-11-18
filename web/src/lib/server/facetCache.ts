import type { createApi } from '#lib/api.js';
import type { FacetCounts } from '#lib/types.js';

type FacetCountsFn = ReturnType<typeof createApi>['facetCounts'];

/** How long a server render may reuse a facet distribution. The counts are the same for
 *  every visitor and move only when ingest lands a wave (a few times an hour). The job
 *  page asks for them on every render, under a handful of filters shared by the whole
 *  catalogue, and crawlers render millions of distinct job pages. Measured 2026-10-30:
 *  /jobs/facets was the API's busiest route at ~44 req/s, nearly all of it from SSR. */
const TTL_MS = 10 * 60_000;

/** Bounds the memory: the job page's filters number in the dozens, the long tail (roles,
 *  skills, company pages) in the thousands. Past this the oldest entry goes. */
const DEFAULT_CAPACITY = 5_000;

/** A per-process facet cache for server renders. Each render brings its own counter (its
 *  own `fetch`); the entries are shared by all of them. The promise is what is stored, so
 *  concurrent renders share one request; a rejection is dropped so the next render asks
 *  again. The disjunctive mode reads a visitor's live selection and is never cached — the
 *  Go handler declines to cache it for the same reason. */
export function createFacetCache(capacity = DEFAULT_CAPACITY, ttlMs = TTL_MS): (live: FacetCountsFn) => FacetCountsFn {
  const entries = new Map<string, { at: number; value: Promise<FacetCounts> }>();

  return (live) => (params, opts) => {
    // The mode can also arrive in the params themselves: /analytics hands its URL through.
    if (opts?.disjunctive || params.has('disjunctive')) return live(params, opts);

    const key = `${params.toString()}|${opts?.facets?.join(',') ?? ''}`;
    const now = Date.now();
    const hit = entries.get(key);
    if (hit && now - hit.at < ttlMs) return hit.value;

    entries.delete(key);
    if (entries.size >= capacity) {
      const oldest = entries.keys().next();
      if (!oldest.done) entries.delete(oldest.value);
    }
    const value = live(params, opts);
    entries.set(key, { at: now, value });
    value.catch(() => {
      if (entries.get(key)?.value === value) entries.delete(key);
    });
    return value;
  };
}

/** The one cache every server render in this process shares. */
export const withFacetCache = createFacetCache();

/** A day-long cache, kept apart so its entries and the ten-minute ones never evict each
 *  other, for a distribution that is both expensive and slow-moving: the skills facet,
 *  ~1,100 values, 100-700 ms of engine time per call. A page that shows a few of its
 *  top values loses nothing to a day's staleness, and a crawler sweeping the long tail
 *  of such pages is what starved the engine into 500s on 2026-11-07 and 08. */
export const withDailyFacetCache = createFacetCache(DEFAULT_CAPACITY, 24 * 60 * 60_000);

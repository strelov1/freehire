// The saved-search query string: its canonical form, the form a saved search stores, and
// whether it narrows anything.

import { activeFilterCount, filtersFromParams, filtersToParams, type JobFilters } from './facetModel';

/** Normalize a search query string to its canonical form (parse → re-serialize),
 *  so two filter sets that differ only in param order or stale/unknown params
 *  compare equal. Used to detect which saved search matches the current filters. */
export function canonicalQuery(query: string): string {
  return savedSearchQuery(filtersFromParams(new URLSearchParams(query)));
}

/** The saved-search / alert target: the filters as a canonical query string, WITHOUT
 *  the ordering. A named wrapper over filtersToParams so the saved-search call sites
 *  (SavedSearches, FilterSummary) express intent ("the comparable
 *  query for this filter set") rather than reaching for the raw serializer.
 *
 *  The ordering is dropped because a saved search is about WHICH jobs are in the set,
 *  and the ordering does not change that. The digest matcher agrees already: it reads a
 *  stored query for `q` and the filter only and orders by its own clock
 *  (internal/engage/notify/match.go), so two sets differing only by sort mail the same
 *  jobs. Keeping the sort in the key made them compare unequal anyway, so choosing an
 *  ordering marked the saved search it came from as dirty and saving again created a
 *  duplicate that delivered identical digests.
 *
 *  It serializes with the ordering UNCHOSEN rather than deleting the key afterwards:
 *  `null` is the type's own word for "no choice", and filtersToParams already writes
 *  nothing for it, so this cannot drift if the wire parameter is ever renamed.
 *
 *  "Hiring again" is dropped for a different reason: it is resolved per request from the
 *  reader's experience bank, and the digest matcher replays a stored query with no reader
 *  to resolve it for — an alert carrying it would mail the whole catalogue. The watchlist
 *  is the opposite case and stays: the matcher resolves it against the list of the
 *  search's owner. */
export function savedSearchQuery(f: JobFilters): string {
  // Neither feed preference can be replayed by a digest.
  return filtersToParams({ ...f, sort: null, hideSeen: false, formerEmployers: false }).toString();
}

/** Whether a saved query can carry alerts: mirrors the server's search.Narrows, which
 *  refuses a subscription to a query that selects the whole catalogue. Only what the
 *  digest matcher's search reads counts, so a minimum skill match alone does not: the
 *  matcher applies it after a search that would have returned everything. */
export function queryNarrows(query: string): boolean {
  const f = filtersFromParams(new URLSearchParams(query));
  if (f.q.trim() !== '') return true;
  return activeFilterCount({ ...f, minSkillMatch: null, hideSeen: false, formerEmployers: false }) > 0;
}

// ---- per-value sign transitions (pure: FacetState -> FacetState) ----

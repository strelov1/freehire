// The job list's sort order: the default for a query, the order a filter state
// actually sorts by, and the options the sort control offers.

import type { JobFilters } from './facetModel';

/** The feed's ordering vocabulary. Deliberately short: this is not a general sort
 *  control (the API also accepts created_at and the salary bounds), it is the two
 *  orderings the endpoint defaults between, plus the profile-match feed and the
 *  most-opened feed. */
export type JobSort = 'relevance' | 'newest' | 'views' | 'match';

/** The ordering the endpoint applies when a request carries no `sort` at all:
 *  relevance under query text, posting date without it (see `searchSort` in
 *  internal/api/handler/search.go). The client mirrors it rather than restating it,
 *  so "the default" and "what the server does with no param" cannot drift — which is
 *  also why serializing the default means writing nothing. */
export function defaultSortFor(q: string): JobSort {
  return q ? 'relevance' : 'newest';
}

/** The ordering a filter set actually resolves to.
 *
 *  An unchosen ordering resolves to the contextual default, and an explicit
 *  `relevance` collapses to the browse default once the query is cleared — it has
 *  nothing left to rank against. `relevance` is the ONLY ordering that collapses:
 *  `views` ranks by a stored figure, so an emptied query leaves it perfectly
 *  servable and discarding the caller's choice there would be a bug, not a fallback.
 *  This is a pure function, not an effect that rewrites the stored value, because BOTH
 *  the sort control and filtersToParams need the answer and a second copy of the rule
 *  is a second answer. */
export function effectiveSort(f: JobFilters): JobSort {
  const sort = f.sort ?? defaultSortFor(f.q);
  return sort === 'relevance' && !f.q ? 'newest' : sort;
}

const SORT_LABEL: Record<JobSort, string> = {
  relevance: 'Relevance',
  newest: 'Newest',
  views: 'Most viewed',
  match: 'Best match',
};

export interface SortOption {
  value: JobSort;
  label: string;
}

/** The orderings a caller can actually choose between, in display order.
 *
 *  `relevance` is the only conditional one: it ranks against query text, so without any
 *  there is nothing for it to rank. `newest`, `views` and `match` always apply.
 *
 *  `match` was conditional too — it needed a profile with skills. That hid the one
 *  ordering that answers "which of these suit me" from precisely the people who had not
 *  filled a profile in, and told them nothing. It is offered to everyone now, and the
 *  view explains what it needs when there is nothing to rank against (see
 *  matchSortNeedsSkills below).
 *
 *  It lives here, not in the view, for the reason effectiveSort does: it is pure, it
 *  decides what the user sees, and in the component nothing could test it. */
export function sortOptionsFor(q: string): SortOption[] {
  // Match is offered to everyone, including a viewer with no skills on file. Hiding it
  // from them answered "why can I not sort by fit here?" with nothing at all — and it
  // hid the reason to fill in a profile from exactly the people who have not. What they
  // get instead is an explanation of what the ordering needs; see matchSortNeedsSkills.
  const values: JobSort[] = [...(q ? (['relevance'] as const) : []), 'newest', 'views', 'match'];
  return values.map((value) => ({ value, label: SORT_LABEL[value] }));
}

/** The option the control shows as selected.
 *
 *  Normally the effective ordering. `relevance` is the one that can be asked for and not
 *  offered — it ranks against query text, and a shared link can arrive without any — so
 *  the control then names what the server will ACTUALLY serve. A select whose value
 *  matches no option renders blank, which would put an empty control over a live
 *  ordering. */
export function selectedSortFor(f: JobFilters): JobSort {
  const sort = effectiveSort(f);
  const offered = sortOptionsFor(f.q);
  return offered.some((o) => o.value === sort) ? sort : defaultSortFor(f.q);
}

/** Whether the feed should explain that the match ordering has nothing to rank against.
 *
 *  Only when match is the ordering actually in force AND the viewer has no skills on
 *  file. The server degrades such a request to newest rather than refusing it, so the
 *  list still fills — and a visitor told nothing reads that as the sort being broken
 *  rather than as a profile they have not filled in yet. */
export function matchSortNeedsSkills(f: JobFilters, hasSkills: boolean): boolean {
  return effectiveSort(f) === 'match' && !hasSkills;
}

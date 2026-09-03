// The header search's target when no list page has registered one.
//
// There used to be two header search boxes: one that filtered the list under it, and
// one — on every other page — that navigated to the feed. They shared the debounce,
// the stale-response token, the arrow keys, the hotkeys, the dismissal and the row
// rendering, in two copies, and differed in exactly one thing: what a pick DOES.
//
// So that one thing became a target. On a list page the target is the page's filter
// store; everywhere else it is a target built from `browseQuery` below, which turns
// the same pick into a link to the feed carrying the same filter.
//
// Pure by design: the navigation itself lives with the component that owns `goto`, so
// this half is testable without a SvelteKit runtime.

import type { ApplyPlan } from './apiSuggestions';
import { emptyFacet, emptyFilters, facetSetSign, filtersToParams } from './facetModel';
import type { Suggestion } from './suggestions';

/** The feed query string a pick opens, or '' when the pick named nothing.
 *
 *  Built through the filter model rather than by concatenating params, so the URL a
 *  navigation opens and the URL the in-place filter writes come from one serializer.
 *  Two would drift, and the drift would be a link that filters differently from the
 *  control that made it. */
export function browseQuery(plan: ApplyPlan): string {
  const f = emptyFilters();
  f.q = plan.q?.trim() ?? '';
  for (const [param, value] of plan.facets) {
    f.facets[param] = facetSetSign(f.facets[param] ?? emptyFacet(), value, 'include');
  }
  return filtersToParams(f).toString();
}

/** The plan a locally-built starter row applies.
 *
 *  Only categories reach here — the empty box offers nothing else — but a role would
 *  apply `role`, and saying so keeps the mapping honest rather than assuming the
 *  caller's one kind. */
export function planForSuggestion(s: Suggestion): ApplyPlan {
  return { facets: [[s.kind === 'role' ? 'role' : 'category', s.slug]] };
}

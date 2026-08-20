// Role suggestions for the header search box. The jobs feed's search is free text,
// but most of what people type into it names a role the `role` facet already tags
// deterministically — so we offer that facet at the moment of typing instead of
// leaving it to be found in the filter modal.
//
// Pure by design: no Svelte, no DOM, no network. The role catalogue and its aliases
// already ship to the browser in generated/contracts.ts, and the caller passes the
// facet distribution it has already fetched, so a suggestion costs no round-trip.

import { ROLE_ALIASES, ROLE_LABELS } from './generated/contracts';
import { optionMatches } from './facets';
import type { FacetCounts } from './types';

/** One role offered under the header search box. `count` is the role's open-vacancy
 *  figure, absent when the facet distribution has not been measured yet. */
export interface RoleSuggestion {
  slug: string;
  label: string;
  count?: number;
}

// The catalogue as facet options, built once: optionMatches — the role picker's own
// matcher — takes an option, and reusing it is what keeps "does this query name this
// role" a single behaviour rather than two that drift apart.
const roleOptions = Object.entries(ROLE_LABELS).map(([value, label]) => ({ value, label }));

/** How many roles the dropdown offers. Past a handful the list stops being a
 *  shortcut and starts being a second filter panel. */
const maxSuggestions = 5;

/** Shortest query worth matching. One character matches most of the catalogue and
 *  an empty one matches all of it (fuzzyMatch treats an empty query as matching
 *  everything), so the floor belongs here rather than in each caller. */
const minQueryLength = 2;

export function suggestRoles(
  query: string,
  counts: FacetCounts | null,
  active: readonly string[] = [],
): RoleSuggestion[] {
  if (query.trim().length < minQueryLength) return [];

  // Two different absences, deliberately not folded together: no distribution at all
  // means "not measured yet" and every match is offered without a figure, while a
  // role missing FROM a distribution means "measured, and it is zero" — offering that
  // would send the user to an empty page.
  const dist = counts?.facets.role;
  const taken = new Set(active);

  // Each slug is a distinct key of ROLE_LABELS, so a role several of whose aliases
  // match is still visited once — no dedup pass needed. Eligibility is filtered
  // before matching on purpose: optionMatches runs an edit-distance pass per role,
  // and there is no reason to spend it on one we would drop anyway.
  return roleOptions
    .filter((o) => !taken.has(o.value) && (!dist || o.value in dist))
    .filter((o) => optionMatches(o, query, ROLE_ALIASES))
    .map((o) => {
      const suggestion: RoleSuggestion = { slug: o.value, label: o.label };
      // The eligibility filter guarantees the lookup whenever dist exists; when it
      // does not, the suggestion carries no count at all rather than a zero.
      if (dist) suggestion.count = dist[o.value];
      return suggestion;
    })
    .toSorted((a, b) => (b.count ?? 0) - (a.count ?? 0) || a.label.localeCompare(b.label))
    .slice(0, maxSuggestions);
}

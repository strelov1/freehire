// The skills typeahead's candidate universe — canonical tokens with live job counts,
// shared by the profile form, the Skills tab and the onboarding tour (all need the same
// source the job-search filter panel uses, not a bundled/static list) — and the one search
// every skills picker runs over it.

import { loadFacetDistribution, rankOptionMatches, type FacetOption } from '#lib/facets.js';
import { SKILL_ALIASES } from '#lib/generated/skillAliases.js';

/** Fetch the live skills distribution and shape it into sorted typeahead options. */
export function loadSkillDistribution(): Promise<FacetOption[]> {
  return loadFacetDistribution('skills');
}

/** The skills offered for `query`: matched on the label and the dictionary's own aliases
 *  ("golang" finds Go), typo-tolerantly, best match first — see rankOptionMatches. With no
 *  query, the busiest `idle` skills. `exclude` drops skills already taken elsewhere. */
export function searchSkillOptions(
  dist: readonly FacetOption[],
  query: string,
  exclude: readonly string[] = [],
  idle = 8,
): FacetOption[] {
  const pool = exclude.length ? dist.filter((o) => !exclude.includes(o.value)) : dist;
  if (!query.trim()) return pool.slice(0, idle);
  return rankOptionMatches(pool, query, SKILL_ALIASES).slice(0, 50);
}

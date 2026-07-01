// Resume-driven skill-gap analysis for search profiles.
//
// The "expected" skills for a profile are the most in-demand skills across the open
// jobs in its specialization(s), read from the /jobs/facets endpoint (the live market).
// The gap is which of those the profile lacks. All logic here is pure so it can be
// reasoned about and checked in isolation (the web app has no unit-test runner); the
// component only fetches facets and renders.

/** How many top market skills define the "expected" set. Mirrors Hirable's top-20 stack. */
export const GAP_TOP_N = 20;

export interface SkillGap {
  /** Top-N market skills for the specialization(s), most in-demand first. */
  expected: string[];
  /** Expected skills the profile already has. */
  have: string[];
  /** Expected skills the profile lacks, in demand order. */
  missing: string[];
  /** have.length — the numerator of the coverage ratio. */
  coverage: number;
  /** min(N, available market skills) — the denominator of the coverage ratio. */
  total: number;
}

/** Skill slugs of a facet `{skill: count}` map, ordered by demand: count descending,
 *  then name ascending as a stable tiebreak so the top-N is deterministic across loads. */
export function sortSkillsByCount(counts: Record<string, number>): string[] {
  return Object.entries(counts)
    .toSorted((a, b) => b[1] - a[1] || a[0].localeCompare(b[0]))
    .map(([skill]) => skill);
}

/** Compare a profile's skills against the top-N market skills. `marketSkills` must be
 *  pre-sorted by demand (see sortSkillsByCount). */
export function computeGap(
  marketSkills: string[],
  profileSkills: string[],
  n: number = GAP_TOP_N,
): SkillGap {
  const expected = marketSkills.slice(0, n);
  const owned = new Set(profileSkills);
  const have = expected.filter((s) => owned.has(s));
  const missing = expected.filter((s) => !owned.has(s));
  return { expected, have, missing, coverage: have.length, total: expected.length };
}

/** Build the facet query for a profile's specialization(s): one `category` param per
 *  specialization. Repeated params are OR-ed by the search backend, so the result is
 *  the combined market across all of the profile's roles. */
export function categoryParams(specializations: string[]): URLSearchParams {
  const params = new URLSearchParams();
  for (const spec of specializations) params.append('category', spec);
  return params;
}

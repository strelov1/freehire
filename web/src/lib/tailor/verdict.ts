import type { Analysis } from '$lib/generated/contracts';

type Requirement = Analysis['requirement_match'][number];

/**
 * Split a fit analysis's requirement coverage into the two honest-wall buckets the tailoring
 * UI surfaces: `missingHave` (the CV omits evidence the candidate has — reframe it) and
 * `missingGap` (a genuine gap — ask before adding). Covered / synonym-only are dropped.
 * A null analysis (none cached yet) yields empty lists.
 */
export function splitRequirements(analysis: Analysis | null): {
  missingHave: Requirement[];
  missingGap: Requirement[];
} {
  const reqs = analysis?.requirement_match ?? [];
  return {
    missingHave: reqs.filter((r) => r.status === 'missing-have'),
    missingGap: reqs.filter((r) => r.status === 'missing-gap'),
  };
}

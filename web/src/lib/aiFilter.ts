// The AI filter: turn a written description of a job search into filter values.
//
// The server does the interpreting (POST /api/v1/search/interpret, internal/searchintent)
// and returns values it has already canonicalised against the real dictionaries. This
// module owns the two ends of that: asking, and turning the answer into filters.

import { FACETS } from './facets';
import {
  type JobFilters,
  emptyFacet,
  emptyFilters,
  facetAdd,
  facetSetSign,
  signOf,
} from './facetModel';
import type { Interpretation } from './types';

/** Build a fresh filter set from an interpretation — the reset-and-apply behind the
 *  dialog's Apply. Every value is already canonical; the work here is placing it.
 *
 *  A facet this build has no control for is dropped rather than passed through. The
 *  server's vocabulary and this one drift apart across a deploy, and a value with no
 *  chip would narrow the results with nothing on screen to lift it.
 *
 *  A value that arrives both wanted and avoided stays wanted, the same rule
 *  filtersFromProfile and the URL parser apply — so the two sides can never
 *  self-cancel into a filter that looks like it was never applied. */
export function filtersFromInterpretation(result: Interpretation): JobFilters {
  const f = emptyFilters();
  const known = new Set(FACETS.map((def) => def.param));

  for (const [param, values] of Object.entries(result.facets ?? {})) {
    if (!known.has(param)) continue;
    f.facets[param] = (values ?? []).reduce(facetAdd, f.facets[param] ?? emptyFacet());
  }
  for (const [param, values] of Object.entries(result.exclude ?? {})) {
    if (!known.has(param)) continue;
    f.facets[param] = (values ?? []).reduce((st, raw) => {
      const v = raw.trim();
      return v && signOf(st, v) === 'off' ? facetSetSign(st, v, 'exclude') : st;
    }, f.facets[param] ?? emptyFacet());
  }

  f.q = result.query ?? '';
  f.visa = result.visa_sponsorship ?? false;
  // `?? null`, not `|| null`: zero is a real bound on the experience ceiling — it is the
  // entry-level filter — and truthiness would erase it.
  f.salaryMin = result.salary_min ?? null;
  f.postedWithinDays = result.posted_within_days ?? null;
  f.experienceYearsMax = result.experience_years_max ?? null;
  return f;
}

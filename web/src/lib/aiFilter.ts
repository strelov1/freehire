// The AI filter: turn a written description of a job search into filter values.
//
// The server does the interpreting (POST /api/v1/search/interpret, internal/searchintent)
// and returns values it has already canonicalised against the real dictionaries. This
// module owns the two ends of that: asking, and turning the answer into filters.

import { dynamicLabel, FACETS } from './facets';
import { experienceLabel, freshnessLabel } from './filterControls';
import {
  type JobFilters,
  emptyFacet,
  emptyFilters,
  facetAdd,
  facetSetSign,
  filtersToParams,
  signOf,
} from './facetModel';
import type { Interpretation, ResumeProfile } from './types';

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

/** The plan as job-list query params: what the results overview counts and what Apply
 *  writes, so the two can never describe different searches. */
export function planParams(result: Interpretation): URLSearchParams {
  return filtersToParams(filtersFromInterpretation(result));
}

/** The line a CV adds to the first description: the deterministic profile the resume
 *  extractor derived, in words the interpreter reads like any other constraint. Fifteen
 *  skills is enough to steer the search without drowning the visitor's own words. */
export function cvContext(profile: ResumeProfile): string {
  const parts: string[] = [];
  if (profile.seniority) parts.push(`seniority ${profile.seniority}`);
  if (profile.categories.length) parts.push(`field ${profile.categories.join(', ')}`);
  if (profile.skills.length) parts.push(`skills ${profile.skills.slice(0, 15).join(', ')}`);
  return parts.length ? `\n\nFrom my CV: ${parts.join('; ')}.` : '';
}

/** The country distribution to draw for a plan, largest first.
 *
 *  A posting can list several countries ("remote LATAM: BR, MX, CO"), so the facet also
 *  counts the OTHER countries of every job that matched. Drawn as-is, a plan for Brazil
 *  shows Mexico beside it and reads as "these jobs are not in Brazil". When the plan
 *  names countries only those are drawn; the ones it excludes never are. */
export function limitToRequestedCountries(
  counts: Record<string, number>,
  result: Interpretation,
): [string, number][] {
  const wanted = new Set(result.facets?.countries ?? []);
  const avoided = new Set(result.exclude?.countries ?? []);
  return Object.entries(counts)
    .filter(([code]) => (wanted.size === 0 || wanted.has(code)) && !avoided.has(code))
    .sort((a, b) => b[1] - a[1]);
}

/** One previewed value. `exclude` draws it struck through, as the sidebar does; `key` is
 *  its identity in a keyed `{#each}` — `param:value`, never the label, which two values
 *  of one facet can share. */
type PreviewChip = { key: string; text: string; exclude: boolean };
export type PreviewGroup = { label: string; chips: PreviewChip[] };

/** What a plan will apply, grouped and labelled exactly as the filter controls label it,
 *  so the preview and the applied filter read the same. Included wins over excluded —
 *  the rule filtersFromInterpretation applies — which also keeps one value from being
 *  keyed twice. */
export function planPreview(result: Interpretation): PreviewGroup[] {
  const label = (param: string, value: string) =>
    FACETS.find((d) => d.param === param)?.options?.find((o) => o.value === value)?.label ??
    dynamicLabel(param, value);
  const groups: PreviewGroup[] = [];
  const push = (heading: string, chips: PreviewChip[]) => {
    if (chips.length) groups.push({ label: heading, chips });
  };
  const one = (heading: string, text: string) => push(heading, [{ key: heading, text, exclude: false }]);

  for (const def of FACETS) {
    const included = result.facets?.[def.param] ?? [];
    push(def.label, [
      ...included.map((v) => ({ key: `${def.param}:${v}`, text: label(def.param, v), exclude: false })),
      ...(result.exclude?.[def.param] ?? [])
        .filter((v) => !included.includes(v))
        .map((v) => ({ key: `${def.param}:${v}`, text: label(def.param, v), exclude: true })),
    ]);
  }
  if (result.query) one('Search', result.query);
  if (result.salary_min != null) one('Salary', `${result.salary_min.toLocaleString('en-US')}+`);
  if (result.posted_within_days != null) one('Posted', freshnessLabel(result.posted_within_days));
  if (result.experience_years_max != null) one('Experience', experienceLabel(result.experience_years_max));
  if (result.visa_sponsorship) one('Visa', 'Sponsorship');
  return groups;
}

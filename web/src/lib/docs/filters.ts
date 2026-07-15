// The job-search filter vocabulary, for the docs. The bulk of the facet rows are
// DERIVED from the live FACETS registry (web/src/lib/facets.ts) — itself driven
// by the generated contracts mirrored from the Go StringFacets map — so a facet
// the SPA renders documents itself. A few StringFacets keys the SPA does not
// surface (company_size, education_level, salary_period) are added by hand so the
// docs cover the FULL API vocabulary, not just the SPA subset. The numeric/boolean
// filters and recipes are hand-written; they live in query_filter.go with no
// generated counterpart.
//
// Note on modifiers: query_filter.go applies repeat-OR, `_mode=and`, and
// `_exclude` UNIFORMLY to every string facet, so the docs state that once in
// FILTER_MODIFIERS rather than per row (the per-facet AND/OR toggle in facets.ts
// is a SPA UI choice, not an API limit).

import { FACETS } from '../facets';

/** One documented filter parameter. */
export interface FilterRow {
  param: string;
  label: string;
  /** Human summary of the accepted values. */
  values: string;
}

// A facet with no static option list is an open vocabulary (skills, countries,
// company_slug): its values come from the live distribution, so we point readers
// at /jobs/facets instead of enumerating them.
const OPEN_VOCAB = 'Open vocabulary — call /jobs/facets for live values';

function valuesOf(options?: { value: string }[]): string {
  if (!options || options.length === 0) return OPEN_VOCAB;
  return options.map((o) => o.value).join(', ');
}

// StringFacets keys the SPA registry (facets.ts) does not render, kept in sync by
// hand. Values mirror the Go enrichment vocabularies (internal/enrich).
const API_ONLY_FACETS: FilterRow[] = [
  { param: 'company_size', label: 'Company size', values: '1-10, 11-50, 51-200, 201-500, 501-1000, 1000+' },
  { param: 'education_level', label: 'Education level', values: 'none, bachelor, master, phd' },
  { param: 'salary_period', label: 'Salary period', values: 'year, month, day, hour' },
];

/** Every string facet: the SPA-derived rows plus the API-only ones. */
export const FILTER_FACETS: FilterRow[] = [
  ...FACETS.map((f) => ({ param: f.param, label: f.label, values: valuesOf(f.options) })),
  ...API_ONLY_FACETS,
];

/** Non-facet filters: numeric ranges and the boolean visa flag. These live in
 *  query_filter.go outside StringFacets, so they are documented by hand. */
export const FILTER_EXTRAS: FilterRow[] = [
  { param: 'visa_sponsorship', label: 'Visa sponsorship', values: 'true, false' },
  {
    param: 'salary_min',
    label: 'Minimum salary',
    values: 'integer — jobs whose minimum salary is at least this (pair with salary_currency)',
  },
  {
    param: 'salary_max',
    label: 'Maximum salary',
    values: 'integer — jobs whose maximum salary is at most this (pair with salary_currency)',
  },
  {
    param: 'experience_years_min',
    label: 'Minimum experience',
    values: 'integer — jobs requiring at least this many years',
  },
  {
    param: 'posted_within_days',
    label: 'Posted within',
    values: 'integer — jobs whose effective posting date falls in the last N days',
  },
];

/** How the cross-facet modifiers behave — they apply to every string facet. */
export const FILTER_MODIFIERS = [
  'Repeat any facet param to OR its values: `skills=go&skills=rust` matches either.',
  'Add `<param>_mode=and` to require all selected values: `skills=go&skills=rust&skills_mode=and` matches both.',
  'Add `<param>_exclude=<value>` to exclude matches: `company_type_exclude=outstaff` drops outstaff jobs.',
  'Different facets are ANDed together; numeric and boolean filters are ANDed too.',
  'Use `regions=none` to match jobs with no resolved geography (an empty region set); it ORs with real region values and supports `_exclude` like any region.',
];

/** Worked filter recipes shown as ready-to-run examples. */
export interface Recipe {
  title: string;
  query: string;
}

export const RECIPES: Recipe[] = [
  { title: 'Senior Go, remote, in the CIS region', query: 'q=go&seniority=senior&work_mode=remote&regions=cis' },
  { title: 'Backend roles, freshest first, in Germany', query: 'category=backend&countries=DE&sort=posted_at&order=desc' },
  { title: 'Must use both Go and Rust', query: 'skills=go&skills=rust&skills_mode=and' },
  { title: 'Exclude outstaff companies', query: 'company_type_exclude=outstaff' },
  { title: 'At least $100k, with visa sponsorship', query: 'salary_currency=USD&salary_min=100000&visa_sponsorship=true' },
];

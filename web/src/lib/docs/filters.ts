// The job-search filter vocabulary, for the docs. The string-facet rows are
// DERIVED from the live FACETS registry (web/src/lib/facets.ts), which is itself
// driven by the generated contracts mirrored from the Go StringFacets map — so a
// new facet documents itself with no edit here. Only the non-facet filters
// (numeric/boolean), the cross-facet modifiers, and the worked recipes are
// hand-written, because they live in query_filter.go with no generated counterpart.

import { FACETS } from '../facets';

/** One documented filter parameter. */
export interface FilterRow {
  param: string;
  label: string;
  /** Human summary of the accepted values. */
  values: string;
  /** Supports `<param>_exclude=<value>` to exclude matches. */
  excludable: boolean;
  /** Supports `<param>_mode=and` to require all selected values. */
  andOr: boolean;
}

// A facet with no static option list is an open vocabulary (skills, countries,
// company_slug): its values come from the live distribution, so we point readers
// at /jobs/facets instead of enumerating them.
const OPEN_VOCAB = 'Open vocabulary — call /jobs/facets for live values';

function valuesOf(options?: { value: string }[]): string {
  if (!options || options.length === 0) return OPEN_VOCAB;
  return options.map((o) => o.value).join(', ');
}

/** String facets, derived from the FACETS registry. */
export const FILTER_FACETS: FilterRow[] = FACETS.map((f) => ({
  param: f.param,
  label: f.label,
  values: valuesOf(f.options),
  excludable: f.excludable,
  andOr: Boolean(f.hasAndOr),
}));

/** Non-facet filters: numeric ranges and the boolean visa flag. These live in
 *  query_filter.go outside StringFacets, so they are documented by hand. */
export const FILTER_EXTRAS: FilterRow[] = [
  {
    param: 'visa_sponsorship',
    label: 'Visa sponsorship',
    values: 'true, false',
    excludable: false,
    andOr: false,
  },
  {
    param: 'salary_min',
    label: 'Minimum salary',
    values: 'integer — jobs whose minimum salary is at least this (pair with salary_currency)',
    excludable: false,
    andOr: false,
  },
  {
    param: 'salary_max',
    label: 'Maximum salary',
    values: 'integer — jobs whose maximum salary is at most this (pair with salary_currency)',
    excludable: false,
    andOr: false,
  },
  {
    param: 'experience_years_min',
    label: 'Minimum experience',
    values: 'integer — jobs requiring at least this many years',
    excludable: false,
    andOr: false,
  },
];

/** How the cross-facet modifiers behave, for a prose note above the tables. */
export const FILTER_MODIFIERS = [
  'Repeat a facet param to OR its values: `skills=go&skills=rust` matches either.',
  'Add `<param>_mode=and` to require all selected values: `skills=go&skills=rust&skills_mode=and` matches both.',
  'Add `<param>_exclude=<value>` to exclude matches: `company_type_exclude=outstaff` drops outstaff jobs.',
  'Different facets are ANDed together; numeric and boolean filters are ANDed too.',
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

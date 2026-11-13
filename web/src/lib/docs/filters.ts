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

import { COMPANY_FACETS, FACETS } from '../facets';

/** One documented filter parameter. */
export interface FilterRow {
  param: string;
  label: string;
  /** Human summary of the accepted values. */
  values: string;
  /** A scalar filter's type; a facet (repeatable strings) leaves it out. */
  type?: 'boolean' | 'integer';
  /** A closed vocabulary's values: anything else matches nothing and is reported in `meta.ignored_values`. */
  options?: string[];
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
// hand. Values mirror the Go vocabularies (internal/dict/vocab).
const apiOnly = (param: string, label: string, options: string[]): FilterRow => ({
  param,
  label,
  values: options.join(', '),
  options,
});
const API_ONLY_FACETS: FilterRow[] = [
  apiOnly('company_size', 'Company size', ['1-10', '11-50', '51-200', '201-500', '501-1000', '1000+']),
  apiOnly('education_level', 'Education level', ['none', 'bachelor', 'master', 'phd']),
  apiOnly('salary_period', 'Salary period', ['year', 'month', 'day', 'hour']),
  apiOnly('is_tech', 'Technical role', ['tech', 'non_tech']),
];

// Facets whose SPA option list is a sample, not the vocabulary: the API takes any value
// (closedFacets in internal/search/search/query_values.go leaves them out).
const OPEN_WITH_OPTIONS = new Set(['collections', 'salary_currency']);

// A facet whose value list needs saying more than enumerating. Keyed by param, this
// replaces the derived `values` text for that row only — the row itself still comes
// from the FACETS registry, so the facet cannot be documented twice or go missing.
const VALUES_OVERRIDE: Record<string, string> = {
  cities:
    'Open vocabulary — call /jobs/facets for live values. Recognized dictionary aliases such as München and Köln resolve to their canonical city names.',
  // The one-sided vocabulary. An integrator who reads `role_type_exclude` as a
  // positive individual-contributor filter is building on a claim the catalogue
  // cannot support, so the caveat has to ride with the row rather than be inferred.
  role_type:
    'people_manager — the title names a people-management role. One value only: carrying nothing means no management marker was found, NOT that the posting is individual-contributor work, so role_type_exclude means "no marker", not "IC"',
};

// A note appended to a facet's value list rather than replacing it. `management` is no
// longer a category, but an integration built before its retirement still sends it, and
// the filter answers it rather than matching nothing (search.retiredCategory).
const VALUES_NOTE: Record<string, string> = {
  clearance_levels:
    '. none = no detected clearance requirement (the same postings as requires_clearance=false); other = a non-US scheme (UK SC/DV, AU NV1, NATO) or a clearance stated with no level; interim is stated beside a level, not instead of one',
  category: '. The retired value management is still accepted and answered as role_type=people_manager',
  perks:
    ' — matched from the description; a posting without one does not mention it, which is not the same as not offering it',
  equity: ' — not_stated means none was read, not that the job offers no equity',
  engagement:
    " — employee or own business, in each market's own term (UK IR35, US W-2/1099/C2C, Polish B2B and umowy, Brazilian CLT/PJ, Russian ТК/ГПХ/самозанятость/ИП, German, Dutch, French, Spanish and Argentine freelance statuses), as the title, the description or a source's own contract-type field states it; a posting offering several carries each, one that does not say carries none. Separate from employment_type",
};

// Values the API still accepts but the SPA no longer offers (see VALUES_NOTE).
const RETIRED_VALUES: Record<string, string[]> = { category: ['management'] };

/** Every string facet: the SPA-derived rows plus the API-only ones. */
export const FILTER_FACETS: FilterRow[] = [
  ...FACETS.map((f) => ({
    param: f.param,
    label: f.label,
    values: (VALUES_OVERRIDE[f.param] ?? valuesOf(f.options)) + (VALUES_NOTE[f.param] ?? ''),
    options:
      f.options?.length && !OPEN_WITH_OPTIONS.has(f.param)
        ? [...f.options.map((o) => o.value), ...(RETIRED_VALUES[f.param] ?? [])]
        : undefined,
  })),
  ...API_ONLY_FACETS,
];

/** Non-facet filters: numeric ranges and the boolean visa flag. These live in
 *  query_filter.go outside StringFacets, so they are documented by hand. */
export const FILTER_EXTRAS: FilterRow[] = [
  { param: 'visa_sponsorship', type: 'boolean', label: 'Visa sponsorship', values: 'true, false' },
  { param: 'requires_clearance', type: 'boolean', label: 'Security clearance required', values: 'true, false' },
  {
    param: 'via_recruiter',
    type: 'boolean',
    label: 'Posting says a recruiter publishes it for a client',
    values:
      'true — the description says so; false — every posting not marked, including recruiter posts that never say so',
  },
  {
    param: 'former_employers',
    type: 'boolean',
    label: 'Hiring again: companies you have worked at and left',
    values:
      'true — signed in only, read from your experience bank; a name that does not resolve to exactly one company is left out. Anonymous, it is reported in meta.ignored_params. `/jobs/facets` never applies this filter, so its counts ignore it even for signed-in callers',
  },
  {
    param: 'watchlist',
    type: 'boolean',
    label: 'Your watchlist: companies you follow',
    values:
      'true — signed in only; an empty watchlist matches nothing. Anonymous, it is reported in meta.ignored_params. Kept in a saved search, so `watchlist=true` is the alert for new postings at the companies you follow. `/jobs/facets` never applies this filter, so its counts ignore it',
  },
  {
    param: 'salary_min',
    type: 'integer',
    label: 'Minimum salary',
    values: 'integer — jobs whose minimum salary is at least this (pair with salary_currency)',
  },
  {
    param: 'salary_max',
    type: 'integer',
    label: 'Maximum salary',
    values: 'integer — jobs whose maximum salary is at most this (pair with salary_currency)',
  },
  {
    param: 'experience_years_min',
    type: 'integer',
    label: 'Minimum experience',
    values: 'integer — jobs requiring at least this many years',
  },
  {
    param: 'experience_years_max',
    type: 'integer',
    label: 'Maximum experience',
    values:
      'integer — jobs requiring at most this many years, the same figure experience_years_min bounds from below. Use 0 for jobs stating no prior experience is required. Either bound excludes jobs that state no requirement at all, unless experience_include_unspecified is set',
  },
  {
    param: 'experience_include_unspecified',
    type: 'boolean',
    label: 'Include unstated experience',
    values:
      'true — with either experience bound, also keep the jobs that state no experience requirement (most of the catalogue). Has no effect without a bound',
  },
  {
    param: 'experience_only_seniority_unspecified',
    type: 'boolean',
    label: 'Experience only without a level',
    values:
      'true — apply either experience bound only to the jobs that carry no seniority level; a job with a level passes the bound whatever it states. With seniority and seniority_include_unspecified this reads "one of these levels, or no level and within the bound". Has no effect without a bound',
  },
  {
    param: 'seniority_include_unspecified',
    type: 'boolean',
    label: 'Include unstated seniority',
    values:
      'true — with seniority, also keep the jobs that carry no seniority level (most of the catalogue). Has no effect without an included seniority',
  },
  {
    param: 'posted_within_days',
    type: 'integer',
    label: 'Posted within',
    values:
      'integer — jobs whose SOURCE states a posting date in the last N days. Some boards restate that date on every crawl, so a job open for months can satisfy a narrow bound here; open_within_days is the one that cannot be rewritten from outside',
  },
  {
    param: 'open_within_days',
    type: 'integer',
    label: 'Open within',
    values:
      'integer — jobs first recorded by freehire in the last N days, i.e. how long the posting has been in the catalogue regardless of the date its source states. Independent of posted_within_days; both may be set and they narrow together',
  },
  {
    param: 'apply_questions_max',
    type: 'integer',
    label: 'Application questions',
    values:
      'integer ≥ 0 — jobs whose application form requires at most N questions beyond name, contact details and CV (3 = the "short form" filter). Jobs with no form freehire could read are excluded: unknown is not short',
  },
  {
    param: 'auto_apply_available',
    type: 'boolean',
    label: 'Auto-apply can attempt it',
    values:
      'true — only postings whose ATS freehire auto-apply can currently attempt to fill and submit for; a provider-eligibility signal, never a guarantee the submission succeeds',
  },
  {
    param: 'work_mode_remote_anywhere',
    type: 'boolean',
    label: 'Remote from anywhere, other modes at home',
    values:
      'true — with `remote` and a physical work_mode both selected and a geography facet set, reads "remote anywhere, OR hybrid/onsite within the geography" instead of ANDing them. Otherwise, or with work_mode_mode=and, it changes nothing',
  },
  {
    param: 'work_mode_remote_regions',
    label: 'Remote arm: regions',
    options: FILTER_FACETS.find((f) => f.param === 'regions')?.options,
    values:
      'Region values, as regions — bound the remote arm of work_mode_remote_anywhere. Read only while it is in force',
  },
  {
    param: 'work_mode_remote_countries',
    label: 'Remote arm: countries',
    values:
      'ISO 3166-1 alpha-2 codes, as countries — bound the remote arm of work_mode_remote_anywhere, ORed with work_mode_remote_regions. Read only while it is in force',
  },
];

/** How the cross-facet modifiers behave — they apply to every string facet. */
export const FILTER_MODIFIERS = [
  'Pass multiple values as a comma-separated list to OR them: `skills=go,rust` matches either. Repeating the param (`skills=go&skills=rust`) also works.',
  'Add `<param>_mode=and` to require all selected values: `skills=go,rust&skills_mode=and` matches both.',
  'Add `<param>_exclude=<value>` to exclude matches: `company_type_exclude=outstaff` drops outstaff jobs.',
  'Different facets are ANDed together; numeric and boolean filters are ANDed too. The geography facets are the one exception — see below.',
  'Geography is a single OR group: `regions`, `countries`, `metros` and `cities` widen each other instead of narrowing. `regions=eu&countries=IT` means "in Europe **or** in Italy", so it returns everything `regions=eu` alone would. To search one country, drop the region: `countries=IT`. The four name a single concept — *where* — so picking two places reads as "either", which is what makes `regions=eu&countries=BR` ("Europe or Brazil") useful. There is no AND to switch on: `_mode=and` does not apply to geography.',
  'Use `regions=none` to match jobs with no resolved geography (an empty region set); it ORs with real region values and supports `_exclude` like any region.',
  'A param no filter reads is ignored rather than refused — so old links and saved searches keep working — and comes back in `meta.ignored_params`, with `did_you_mean` when it is only the singular of a real facet. Check it: a dropped filter otherwise looks like a genuinely broad result. At most 10 are listed per response. The same report rides on `/jobs/facets`, `/market/coverage` and `/companies`, which answer `{"data": ...}` and grow a `meta` block only when there is something to warn about. `/companies` filters on its own vocabulary, so a jobs facet sent there is reported as ignored.',
];

/** The company listing's filters (`GET /companies`): the SPA's company facets plus the
 *  job-derived `domains`. Values OR within a filter and AND across filters; there is no
 *  `_exclude` or `_mode`. A long vocabulary is sampled, not listed. */
export const COMPANY_FILTERS: FilterRow[] = [
  ...COMPANY_FACETS,
  {
    param: 'domains',
    label: 'Job-derived domains, aggregated per company',
    options: FACETS.find((f) => f.param === 'domains')?.options,
  },
].map((f) => {
  const values = (f.options ?? []).map((o) => o.value);
  return {
    param: f.param,
    label: f.label,
    values: values.length > 30 ? `e.g. ${values.slice(0, 5).join(', ')}` : valuesOf(f.options),
  };
});

/** Worked filter recipes shown as ready-to-run examples. */
export interface Recipe {
  title: string;
  query: string;
}

export const RECIPES: Recipe[] = [
  { title: 'Senior Go, remote, in the CIS region', query: 'q=go&seniority=senior&work_mode=remote&regions=cis' },
  {
    title: 'Backend roles, freshest first, in Germany',
    query: 'category=backend&countries=DE&sort=posted_at&order=desc',
  },
  {
    title: 'One country only — no region param, or it widens back out',
    query: 'countries=IT&employment_type=contract',
  },
  { title: 'Must use both Go and Rust', query: 'skills=go,rust&skills_mode=and' },
  { title: 'Exclude outstaff companies', query: 'company_type_exclude=outstaff' },
  {
    title: 'At least $100k, with visa sponsorship',
    query: 'salary_currency=USD&salary_min=100000&visa_sponsorship=true',
  },
  { title: 'Excluding jobs that need a security clearance', query: 'clearance_levels=none' },
  { title: 'Jobs stating Secret or Top Secret clearance', query: 'clearance_levels=secret,top_secret' },
  { title: 'Only jobs that need a security clearance', query: 'requires_clearance=true' },
];

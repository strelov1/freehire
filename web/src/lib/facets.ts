// The job-search facets, as a data-driven registry. Each entry declares its
// query param (matching the backend search API), section label, the control to
// render, its options (for pills/select), and whether it supports an "exclude"
// mode. The panel iterates this list; the store keys facet state by `param`.
//
// SOURCE_VALUES and the other generated value arrays (WORK_MODE_VALUES, etc.)
// in ./generated/contracts are the single source of truth for closed-vocabulary
// option values. A new backend value appears automatically here (humanized);
// only the label-override map below needs updating when the label differs from
// the title-cased fallback. REGION and CURRENCY are curated subsets not driven
// by generated arrays — leave those alone. Job language is a dynamic facet: its
// options come from the live distribution (any detected language), labelled via
// languageLabel.

import {
  WORK_MODE_VALUES, SENIORITY_VALUES, CATEGORY_VALUES,
  EMPLOYMENT_TYPE_VALUES, RELOCATION_VALUES, ENGLISH_LEVEL_VALUES,
  COMPANY_TYPE_VALUES, DOMAIN_VALUES, ROLE_LABELS, ROLE_ALIASES,
} from './generated/contracts';
import { fuzzyMatch } from './fuzzy';
import {
  REGION_LABELS, SENIORITY_LABELS, EMPLOYMENT_LABELS, WORK_MODE_LABELS,
  CATEGORY_LABELS, DOMAIN_LABELS, COMPANY_TYPE_LABELS,
} from './labels';
import { COLLECTIONS } from './collections';
import { ROLE_RELATED } from './roleRelated';
import { api } from './api';

export interface FacetOption {
  value: string;
  label: string;
  /** Number of matching jobs, for dynamic (distribution-driven) options. */
  count?: number;
}

export type FacetControl = 'pills' | 'select' | 'tokens' | 'remote';

/** One facet's live selection, as FacetSection reads it: the included and excluded
 *  values (a value is in at most one set), plus the include-set match mode. */
export interface FacetSelection {
  include: string[];
  exclude: string[];
  matchAll: boolean;
}

/** The narrow store contract FacetSection drives — the subset of FilterStore's
 *  surface a facet control touches. Both the job FilterStore and the company
 *  filter store satisfy it, so the same section/control components render either.
 *  (cycle/toggleSign move a value between include and exclude; the company registry
 *  has no excludable facets, so pick/add/remove only ever land values in include.) */
export interface FacetStore {
  facet(param: string): FacetSelection;
  /** Pills: cycle a value off → include → exclude → off. */
  cycle(param: string, v: string): void;
  /** Select dropdown: add a value to include, or clear it if already selected. */
  pick(param: string, v: string): void;
  /** Per-chip toggle: flip a value between include and exclude. */
  toggleSign(param: string, v: string): void;
  add(param: string, raw: string): void;
  remove(param: string, v: string): void;
  clearFacet(param: string): void;
  setMatchAll(param: string, on: boolean): void;
}

export interface FacetDef {
  param: string;
  label: string;
  control: FacetControl;
  options?: FacetOption[];
  excludable: boolean;
  /** Show a per-facet AND/OR toggle (match all vs match any) over selected values. */
  hasAndOr?: boolean;
  placeholder?: string;
  /**
   * Cap the number of options rendered when the facet-local search is empty, to
   * keep a very large distribution (role, with hundreds of values) readable: the
   * top `cap` busiest options show, the rest are reachable by typing. Unset =
   * render all (the default for smaller selects).
   */
  cap?: number;
  /**
   * Options come from the live facet distribution (with counts), not a static
   * `options` list — for open/large vocabularies (skills, countries). The panel
   * builds them from the facet-counts endpoint at render time.
   */
  dynamic?: boolean;
  /**
   * Server-backed option source for a `'remote'` control: a debounced query
   * function returning matching options with counts. Used for an entity facet too
   * large to ship as a distribution (company), whose options are fetched from a
   * dedicated endpoint instead of the Meili facet counts.
   */
  remote?: (query: string) => Promise<FacetOption[]>;
  /**
   * Curated adjacency map (base slug → related base slugs) driving a "Related"
   * suggestion row under the picker — for surfacing siblings the text search
   * can't name (typing "mobile" won't match "iOS Developer"). Only the role facet
   * sets it; see relatedOptions / ROLE_RELATED.
   */
  related?: Record<string, string[]>;
  /**
   * Curated slug → shorthand-aliases map letting the picker's search match roles
   * by abbreviation/synonym, not just the display label — typing "swe"/"sre"/
   * "devrel" finds the role. Only the role facet sets it; see optionMatches /
   * ROLE_ALIASES (generated from the same dictionaries that tag titles).
   */
  searchAliases?: Record<string, readonly string[]>;
}

// Resolve an ISO 3166-1 alpha-2 code to an English country name via platform Intl
// data (no hand-maintained table); fall back to the upper-cased code.
const regionNames = (() => {
  try {
    return new Intl.DisplayNames(['en'], { type: 'region' });
  } catch {
    return null;
  }
})();

export function countryLabel(code: string): string {
  const up = code.toUpperCase();
  try {
    return regionNames?.of(up) ?? up;
  } catch {
    return up;
  }
}

// Resolve an ISO 639-1 code (en, pt, ru, …) to an English language name via
// platform Intl data (no hand-maintained table); fall back to the upper-cased
// code. Mirrors countryLabel — the posting-language facet is dynamic, so any
// code the language detector emits gets a readable label.
const languageNames = (() => {
  try {
    return new Intl.DisplayNames(['en'], { type: 'language' });
  } catch {
    return null;
  }
})();

export function languageLabel(code: string): string {
  if (!code) return code;
  try {
    return languageNames?.of(code) ?? code.toUpperCase();
  } catch {
    return code.toUpperCase();
  }
}

// Company facet values are slugs (group-ib, epam); the live distribution carries
// no display name, so humanize the slug for a readable label. Imperfect for
// acronyms (group-ib → "Group Ib") but consistent with the other facets — a real
// slug→name lookup would need a per-company fetch, which this facet forgoes.
export function companyLabel(slug: string): string {
  return humanize(slug.replace(/-/g, '_'));
}

// Option source for the Company facet's 'remote' control: the count-ordered
// companies endpoint (real names + open-job counts), not the Meili facet
// distribution (which Meili caps at 300 values and returns alphabetically — so
// popular employers never surface). An empty query returns the most active
// companies (the endpoint's first page).
async function companySearch(query: string): Promise<FacetOption[]> {
  const { items } = await api.listCompanies(query, 20, 0);
  return items.map((c) => ({ value: c.slug, label: c.name, count: c.job_count }));
}

// Role facet values are canonical slugs (senior_backend, founding_engineer); the
// live distribution carries no display name, so map them through the generated
// ROLE_LABELS catalog (the roletag dictionary is the source of truth), falling
// back to a humanized slug for a value the catalog somehow lacks.
export function roleLabel(slug: string): string {
  return (ROLE_LABELS as Record<string, string>)[slug] ?? humanize(slug);
}

/** Display label for a dynamic facet value: country code → name, company slug →
 *  humanized name, else the value. */
export function dynamicLabel(param: string, value: string): string {
  if (param === 'countries') return countryLabel(value);
  if (param === 'posting_language') return languageLabel(value);
  if (param === 'company_slug') return companyLabel(value);
  if (param === 'source') return sourceLabel(value);
  if (param === 'role') return roleLabel(value);
  return value;
}

/** Drop options with a duplicate `value`, keeping the first. The facet controls
 *  render a keyed `{#each}` on `value`, and Svelte aborts the whole render with
 *  `each_key_duplicate` on a collision — so a single stray duplicate (e.g. a
 *  generated-vocabulary regression) would take down every page with a filter
 *  panel. This makes that failure mode a harmless repeat instead of a crash. */
export function uniqueByValue(opts: FacetOption[]): FacetOption[] {
  const seen = new Set<string>();
  return opts.filter((o) => (seen.has(o.value) ? false : seen.add(o.value)));
}

/** Build select options for a dynamic facet from its live distribution (value →
 *  count) plus any already-selected values (so a selection absent from the current
 *  distribution stays listed and removable), labelled via dynamicLabel and sorted
 *  busiest-first. Shared by the filter panel's dynamic selects (FacetSection) and
 *  the onboarding stack picker. */
export function dynamicOptions(param: string, dist: Record<string, number>, selected: string[]): FacetOption[] {
  const keys = new Set<string>([...Object.keys(dist), ...selected]);
  return [...keys]
    .map((value) => ({ value, label: dynamicLabel(param, value), count: dist[value] ?? 0 }))
    .toSorted((a, b) => (b.count ?? 0) - (a.count ?? 0) || a.label.localeCompare(b.label));
}

// Role slugs carry an optional seniority grade prefix (senior_backend); the
// related-role map is keyed by the ungraded base (backend), so one entry serves
// every grade. Longest prefix first is unnecessary — the grades share no
// prefix among themselves — but drawing them from SENIORITY_VALUES keeps this in
// lockstep with the roletag vocabulary.
const gradePrefixes = SENIORITY_VALUES.map((s) => s + '_');

/** Strip a leading seniority grade from a role slug: senior_mobile → mobile. An
 *  ungraded slug (mobile, ios_developer, or a bare seniority like c_level) is
 *  returned unchanged. */
export function baseRole(slug: string): string {
  for (const p of gradePrefixes) {
    if (slug.startsWith(p)) return slug.slice(p.length);
  }
  return slug;
}

/** Suggestions for the role picker's "Related" row: for each currently-surfaced
 *  role (`matched`), look up its base's curated relatives, keep only those that
 *  have jobs in the current distribution (present in `options`) and aren't already
 *  shown or selected, dedupe, and cap at `limit`. Pure so it's unit-testable; the
 *  point is to surface siblings the text search can't ("mobile" never matches
 *  "iOS Developer"). */
export function relatedOptions(
  options: FacetOption[],
  matched: string[],
  selected: string[],
  related: Record<string, string[]>,
  limit = 8,
): FacetOption[] {
  const byValue = new Map(options.map((o) => [o.value, o]));
  const skip = new Set([...matched, ...selected]);
  const seen = new Set<string>();
  const out: FacetOption[] = [];
  for (const v of matched) {
    for (const slug of related[baseRole(v)] ?? []) {
      if (skip.has(slug) || seen.has(slug)) continue;
      const o = byValue.get(slug);
      if (!o) continue;
      seen.add(slug);
      out.push(o);
      if (out.length >= limit) return out;
    }
  }
  return out;
}

/** Does a facet option match the picker's search query? Always typo-tolerantly
 *  matches the display label; when `searchAliases` is supplied (role facet), also
 *  matches the option's curated shorthand aliases — keyed by BASE slug, so "swe"
 *  surfaces Software Engineer and its seniority variants, "sre"/"devrel" find
 *  their roles. An empty query matches everything (via the label match). */
export function optionMatches(
  option: FacetOption,
  query: string,
  searchAliases?: Record<string, readonly string[]>,
): boolean {
  if (fuzzyMatch(option.label, query)) return true;
  const aliases = searchAliases?.[baseRole(option.value)];
  return !!aliases && aliases.some((a) => fuzzyMatch(a, query));
}

/** A title-cased fallback label for a value with no explicit label. */
function humanize(value: string): string {
  return value
    .split('_')
    .map((w) => w.charAt(0).toUpperCase() + w.slice(1))
    .join(' ');
}

/** Build facet options from generated values, overriding labels where given. */
function options(values: readonly string[], labels: Record<string, string> = {}): FacetOption[] {
  return values.map((value) => ({ value, label: labels[value] ?? humanize(value) }));
}

// Label overrides for the source facet, where the display name differs from the
// title-cased fallback (e.g. "smartrecruiters" → "SmartRecruiters"). A new ATS
// adapter needs no entry unless its casing is special. Used by sourceLabel for the
// dynamic (distribution-driven) source select, so a source with a real job count
// renders with its proper name.
const SOURCE_LABELS: Record<string, string> = {
  telegram: 'Telegram', greenhouse: 'Greenhouse', smartrecruiters: 'SmartRecruiters',
  bamboohr: 'BambooHR', successfactors: 'SuccessFactors',
  workatastartup: 'Work at a Startup', remoteok: 'RemoteOK', arc: 'Arc',
  jobstash: 'JobStash', globalpayments: 'Global Payments',
};

/** Display label for a source slug (e.g. smartrecruiters → "SmartRecruiters"),
 *  used by the dynamic source select; falls back to the humanized slug. */
export function sourceLabel(value: string): string {
  return SOURCE_LABELS[value] ?? humanize(value);
}

// The backend's `regions` reach vocabulary (enrich.RegionValues): one consistent
// macro level (continents/macro-regions, plus `global` and the distinct `uk`).
// Country-level filtering lives in the Countries facet, so the US sits under
// `north_america` and Russia under `cis`. Built from the shared REGION_LABELS
// whose insertion order is the curated display order.
const REGION: FacetOption[] = options(Object.keys(REGION_LABELS), REGION_LABELS);

// Reserved `regions` value selecting jobs with no resolved geography (an empty
// regions array) — the "region not specified" bucket — rather than a real region
// code. The backend maps it to Meilisearch's IS EMPTY (see search.RegionUnspecified),
// so it ORs with real regions and supports exclude like any region pill. Only the
// job facet offers it; the company facet omits it (companies filter by SQL array
// overlap, which has no empty-set sentinel).
export const REGION_UNSPECIFIED = 'none';

// Job region options: the macro-regions plus the "Not specified" sentinel chip,
// appended last so it reads after the real regions.
const JOB_REGION: FacetOption[] = [...REGION, { value: REGION_UNSPECIFIED, label: 'Not specified' }];

const WORK_MODE: FacetOption[] = options(WORK_MODE_VALUES, WORK_MODE_LABELS);
const SENIORITY: FacetOption[] = options(SENIORITY_VALUES, SENIORITY_LABELS);
const COMPANY_TYPE: FacetOption[] = options(COMPANY_TYPE_VALUES, COMPANY_TYPE_LABELS);
const EMPLOYMENT: FacetOption[] = options(EMPLOYMENT_TYPE_VALUES, EMPLOYMENT_LABELS);
const RELOCATION: FacetOption[] = options(RELOCATION_VALUES, {
  not_supported: 'None', supported: 'Supported', required: 'Required',
});
const ENGLISH: FacetOption[] = options(ENGLISH_LEVEL_VALUES, {
  a1: 'A1', a2: 'A2', b1: 'B1', b2: 'B2', c1: 'C1', c2: 'C2', native: 'Native', none: 'None',
});
const CATEGORY: FacetOption[] = options(CATEGORY_VALUES, CATEGORY_LABELS);

// The category facet options, exported for reuse outside the filter panel (the search
// profile's specialization picker) so the same labels/order are shared, not duplicated.
export const CATEGORY_OPTIONS: FacetOption[] = CATEGORY;

// Work-mode and region options, exported for the profile's location preferences editor so
// it shares the filter panel's vocabulary/order instead of duplicating it.
export const WORK_MODE_OPTIONS: FacetOption[] = WORK_MODE;
export const REGION_OPTIONS: FacetOption[] = REGION;

/** Display label for a category code (e.g. ml_ai → "ML / AI"), shared by the profile
 *  view; falls back to the humanized code. */
export function categoryLabel(value: string): string {
  return CATEGORY_LABELS[value] ?? humanize(value);
}
const DOMAINS: FacetOption[] = options(DOMAIN_VALUES, DOMAIN_LABELS);

// The job-reality classes (internal/jobreality) — a small closed set, spelled out
// like CURRENCY (not a generated values array). Offered excludable so the common use
// is "exclude Likely evergreen" to hide probable ghost postings; not hidden by default.
const REALITY: FacetOption[] = [
  { value: 'fresh', label: 'Fresh' },
  { value: 'stale', label: 'Stale' },
  { value: 'likely-evergreen', label: 'Likely evergreen' },
];

const CURRENCY: FacetOption[] = [
  { value: 'USD', label: 'USD' },
  { value: 'EUR', label: 'EUR' },
  { value: 'GBP', label: 'GBP' },
  { value: 'RUB', label: 'RUB' },
];

// Curated collections (yc, bigtech, …) as pill options, sourced from the same
// registry the /collections hub renders so the label/slug pairs never drift.
const COLLECTION: FacetOption[] = COLLECTIONS.map((c) => ({ value: c.slug, label: c.title }));

// Company-size buckets — the enrich.CompanySizeValues vocabulary. Not exported as
// a generated values array (it's a scalar enrichment field, not a search facet on
// jobs), so the closed set is spelled out here like CURRENCY; the values are
// already display-ready.
const COMPANY_SIZE: FacetOption[] = ['1-10', '11-50', '51-200', '201-500', '501-1000', '1000+'].map(
  (value) => ({ value, label: value }),
);

// The ISO 3166-1 alpha-2 code set — the country vocabulary the location dictionary
// draws from. The company country facet is a static searchable select over this
// (labelled via countryLabel), not a live distribution: the companies list is plain
// SQL with no facet-count endpoint yet, so options carry no counts.
const ISO_COUNTRY_CODES = [
  'ad', 'ae', 'af', 'ag', 'ai', 'al', 'am', 'ao', 'aq', 'ar', 'as', 'at', 'au', 'aw', 'ax', 'az',
  'ba', 'bb', 'bd', 'be', 'bf', 'bg', 'bh', 'bi', 'bj', 'bl', 'bm', 'bn', 'bo', 'bq', 'br', 'bs',
  'bt', 'bv', 'bw', 'by', 'bz', 'ca', 'cc', 'cd', 'cf', 'cg', 'ch', 'ci', 'ck', 'cl', 'cm', 'cn',
  'co', 'cr', 'cu', 'cv', 'cw', 'cx', 'cy', 'cz', 'de', 'dj', 'dk', 'dm', 'do', 'dz', 'ec', 'ee',
  'eg', 'eh', 'er', 'es', 'et', 'fi', 'fj', 'fk', 'fm', 'fo', 'fr', 'ga', 'gb', 'gd', 'ge', 'gf',
  'gg', 'gh', 'gi', 'gl', 'gm', 'gn', 'gp', 'gq', 'gr', 'gs', 'gt', 'gu', 'gw', 'gy', 'hk', 'hm',
  'hn', 'hr', 'ht', 'hu', 'id', 'ie', 'il', 'im', 'in', 'io', 'iq', 'ir', 'is', 'it', 'je', 'jm',
  'jo', 'jp', 'ke', 'kg', 'kh', 'ki', 'km', 'kn', 'kp', 'kr', 'kw', 'ky', 'kz', 'la', 'lb', 'lc',
  'li', 'lk', 'lr', 'ls', 'lt', 'lu', 'lv', 'ly', 'ma', 'mc', 'md', 'me', 'mf', 'mg', 'mh', 'mk',
  'ml', 'mm', 'mn', 'mo', 'mp', 'mq', 'mr', 'ms', 'mt', 'mu', 'mv', 'mw', 'mx', 'my', 'mz', 'na',
  'nc', 'ne', 'nf', 'ng', 'ni', 'nl', 'no', 'np', 'nr', 'nu', 'nz', 'om', 'pa', 'pe', 'pf', 'pg',
  'ph', 'pk', 'pl', 'pm', 'pn', 'pr', 'ps', 'pt', 'pw', 'py', 'qa', 're', 'ro', 'rs', 'ru', 'rw',
  'sa', 'sb', 'sc', 'sd', 'se', 'sg', 'sh', 'si', 'sj', 'sk', 'sl', 'sm', 'sn', 'so', 'sr', 'ss',
  'st', 'sv', 'sx', 'sy', 'sz', 'tc', 'td', 'tf', 'tg', 'th', 'tj', 'tk', 'tl', 'tm', 'tn', 'to',
  'tr', 'tt', 'tv', 'tw', 'tz', 'ua', 'ug', 'um', 'us', 'uy', 'uz', 'va', 'vc', 've', 'vg', 'vi',
  'vn', 'vu', 'wf', 'ws', 'ye', 'yt', 'za', 'zm', 'zw',
];
const COUNTRY: FacetOption[] = ISO_COUNTRY_CODES.map((value) => ({
  value,
  label: countryLabel(value),
})).toSorted((a, b) => a.label.localeCompare(b.label));

// The full ISO country select, exported for the profile's location editor (base +
// remote/relocation countries) so it reuses the same list/labels as the company facet.
export const COUNTRY_OPTIONS: FacetOption[] = COUNTRY;

// The company catalog's filter facets: mostly a subset of the job facets whose
// values are mostly derived from a company's open jobs and denormalized onto the
// companies row (collections + the RefreshCompanyFacets arrays), including
// `remote_regions` (regions scoped to remote jobs). `yc_batch`/`yc_status` are the
// curated YC-directory facets loaded by cmd/import-yc. No exclude/AND-OR modes — the
// companies list endpoint filters by plain array overlap. Reuses the same option
// vocabularies as the job facets so labels never drift.
const YC_STATUS: FacetOption[] = options(['Active', 'Acquired', 'Public', 'Inactive']);
const YC_STAGE: FacetOption[] = options(['Early', 'Growth']);
const YC_FLAGS: FacetOption[] = options(['top_company', 'hiring'], {
  top_company: 'YC Top Company',
  hiring: 'Hiring',
});
// YC batch labels are the verbatim source strings ("Winter 2012"). Generate the full
// season×year grid so the searchable select covers any batch; phantom combinations
// that no company has simply match nothing.
const YC_BATCH: FacetOption[] = Array.from({ length: 2027 - 2005 + 1 }, (_, i) => 2027 - i).flatMap((y) =>
  ['Winter', 'Spring', 'Summer', 'Fall'].map((s) => ({ value: `${s} ${y}`, label: `${s} ${y}` })),
);
export const COMPANY_FACETS: FacetDef[] = [
  { param: 'collections', label: 'Collection', control: 'pills', options: COLLECTION, excludable: false },
  { param: 'regions', label: 'Region', control: 'pills', options: REGION, excludable: false },
  { param: 'remote_regions', label: 'Remote hiring', control: 'pills', options: REGION, excludable: false },
  { param: 'countries', label: 'Country', control: 'select', options: COUNTRY, excludable: false, placeholder: 'Search countries' },
  { param: 'domains', label: 'Industry', control: 'select', options: DOMAINS, excludable: false, placeholder: 'Search industries' },
  { param: 'company_type', label: 'Company type', control: 'pills', options: COMPANY_TYPE, excludable: false },
  { param: 'company_size', label: 'Company size', control: 'pills', options: COMPANY_SIZE, excludable: false },
  { param: 'yc_status', label: 'YC status', control: 'pills', options: YC_STATUS, excludable: false },
  { param: 'yc_stage', label: 'YC stage', control: 'pills', options: YC_STAGE, excludable: false },
  { param: 'yc_flags', label: 'YC highlights', control: 'pills', options: YC_FLAGS, excludable: false },
  { param: 'yc_batch', label: 'YC batch', control: 'select', options: YC_BATCH, excludable: false, placeholder: 'Search YC batches' },
];

export const FACETS: FacetDef[] = [
  { param: 'collections', label: 'Collection', control: 'pills', options: COLLECTION, excludable: false },
  { param: 'regions', label: 'Region', control: 'pills', options: JOB_REGION, excludable: true },
  { param: 'work_mode', label: 'Work format', control: 'pills', options: WORK_MODE, excludable: true },
  { param: 'role', label: 'Role', control: 'select', dynamic: true, excludable: true, hasAndOr: true, placeholder: 'Search roles', cap: 8, related: ROLE_RELATED, searchAliases: ROLE_ALIASES },
  { param: 'category', label: 'Specialization', control: 'select', options: CATEGORY, excludable: true, placeholder: 'Search specializations' },
  { param: 'seniority', label: 'Seniority', control: 'pills', options: SENIORITY, excludable: true },
  { param: 'skills', label: 'Skills', control: 'select', dynamic: true, excludable: true, hasAndOr: true, placeholder: 'Search skills' },
  { param: 'domains', label: 'Industry', control: 'select', options: DOMAINS, excludable: true, placeholder: 'Search industries' },
  { param: 'company_type', label: 'Company type', control: 'pills', options: COMPANY_TYPE, excludable: true },
  { param: 'countries', label: 'Countries', control: 'select', dynamic: true, excludable: true, placeholder: 'Search countries' },
  { param: 'cities', label: 'City', control: 'select', dynamic: true, excludable: true, placeholder: 'Search cities' },
  { param: 'relocation', label: 'Relocation', control: 'pills', options: RELOCATION, excludable: true },
  { param: 'employment_type', label: 'Employment', control: 'pills', options: EMPLOYMENT, excludable: true },
  { param: 'english_level', label: 'English', control: 'pills', options: ENGLISH, excludable: true },
  { param: 'posting_language', label: 'Job language', control: 'select', dynamic: true, excludable: true, placeholder: 'Search languages' },
  { param: 'reality', label: 'Posting reality', control: 'pills', options: REALITY, excludable: true },
  { param: 'salary_currency', label: 'Currency', control: 'pills', options: CURRENCY, excludable: true },
  { param: 'company_slug', label: 'Company', control: 'remote', excludable: true, placeholder: 'Search companies', remote: companySearch },
  { param: 'source', label: 'Source', control: 'select', dynamic: true, excludable: true, placeholder: 'Search sources' },
];

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
  SOURCE_VALUES, WORK_MODE_VALUES, SENIORITY_VALUES, CATEGORY_VALUES,
  EMPLOYMENT_TYPE_VALUES, RELOCATION_VALUES, ENGLISH_LEVEL_VALUES,
  COMPANY_TYPE_VALUES, DOMAIN_VALUES,
} from './generated/contracts';
import {
  REGION_LABELS, SENIORITY_LABELS, EMPLOYMENT_LABELS, WORK_MODE_LABELS,
  CATEGORY_LABELS, DOMAIN_LABELS, COMPANY_TYPE_LABELS,
} from './labels';
import { COLLECTIONS } from './collections';
import { api } from './api';

export interface FacetOption {
  value: string;
  label: string;
  /** Number of matching jobs, for dynamic (distribution-driven) options. */
  count?: number;
}

export type FacetControl = 'pills' | 'select' | 'tokens' | 'remote';

/** One facet's live selection, as FacetSection reads it. */
export interface FacetSelection {
  values: string[];
  exclude: boolean;
  matchAll: boolean;
}

/** The narrow store contract FacetSection drives — the subset of FilterStore's
 *  surface a facet control touches. Both the job FilterStore and the company
 *  filter store satisfy it, so the same section/control components render either.
 *  (setExclude/setMatchAll are only invoked for excludable/hasAndOr facets, which
 *  the company registry doesn't use.) */
export interface FacetStore {
  facet(param: string): FacetSelection;
  toggle(param: string, v: string): void;
  add(param: string, raw: string): void;
  remove(param: string, v: string): void;
  clearFacet(param: string): void;
  setExclude(param: string, exclude: boolean): void;
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

/** Display label for a dynamic facet value: country code → name, company slug →
 *  humanized name, else the value. */
export function dynamicLabel(param: string, value: string): string {
  if (param === 'countries') return countryLabel(value);
  if (param === 'posting_language') return languageLabel(value);
  if (param === 'company_slug') return companyLabel(value);
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

// Values come from the generated SOURCE_VALUES (sources.All adapter registry +
// "telegram" from the tg-extract worker). A new ATS adapter appears here
// automatically; only add a label override below when the label differs from
// the title-cased fallback (e.g. "smartrecruiters" → "SmartRecruiters").
const SOURCE: FacetOption[] = options(SOURCE_VALUES, {
  telegram: 'Telegram', greenhouse: 'Greenhouse', smartrecruiters: 'SmartRecruiters',
  bamboohr: 'BambooHR', successfactors: 'SuccessFactors',
  workatastartup: 'Work at a Startup', remoteok: 'RemoteOK', arc: 'Arc',
  jobstash: 'JobStash', globalpayments: 'Global Payments',
});

// The backend's `regions` reach vocabulary (enrich.RegionValues): one consistent
// macro level (continents/macro-regions, plus `global` and the distinct `uk`).
// Country-level filtering lives in the Countries facet, so the US sits under
// `north_america` and Russia under `cis`. Built from the shared REGION_LABELS
// whose insertion order is the curated display order.
const REGION: FacetOption[] = options(Object.keys(REGION_LABELS), REGION_LABELS);

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

/** Display label for a category code (e.g. ml_ai → "ML / AI"), shared by the profile
 *  view; falls back to the humanized code. */
export function categoryLabel(value: string): string {
  return CATEGORY_LABELS[value] ?? humanize(value);
}
const DOMAINS: FacetOption[] = options(DOMAIN_VALUES, DOMAIN_LABELS);

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

// The company catalog's filter facets: a subset of the job facets whose values are
// derivable from a company's open jobs and denormalized onto the companies row
// (collections + the RefreshCompanyFacets arrays). No exclude/AND-OR modes — the
// companies list endpoint filters by plain array overlap. Reuses the same option
// vocabularies as the job facets so labels never drift.
export const COMPANY_FACETS: FacetDef[] = [
  { param: 'collections', label: 'Collection', control: 'pills', options: COLLECTION, excludable: false },
  { param: 'regions', label: 'Region', control: 'pills', options: REGION, excludable: false },
  { param: 'countries', label: 'Country', control: 'select', options: COUNTRY, excludable: false, placeholder: 'Search countries' },
  { param: 'domains', label: 'Industry', control: 'select', options: DOMAINS, excludable: false, placeholder: 'Search industries' },
  { param: 'company_type', label: 'Company type', control: 'pills', options: COMPANY_TYPE, excludable: false },
  { param: 'company_size', label: 'Company size', control: 'pills', options: COMPANY_SIZE, excludable: false },
];

export const FACETS: FacetDef[] = [
  { param: 'collections', label: 'Collection', control: 'pills', options: COLLECTION, excludable: false },
  { param: 'regions', label: 'Region', control: 'pills', options: REGION, excludable: true },
  { param: 'work_mode', label: 'Work format', control: 'pills', options: WORK_MODE, excludable: true },
  { param: 'category', label: 'Specialization', control: 'select', options: CATEGORY, excludable: true, placeholder: 'Search specializations' },
  { param: 'seniority', label: 'Seniority', control: 'pills', options: SENIORITY, excludable: true },
  { param: 'skills', label: 'Skills', control: 'select', dynamic: true, excludable: true, hasAndOr: true, placeholder: 'Search skills' },
  { param: 'domains', label: 'Industry', control: 'select', options: DOMAINS, excludable: true, placeholder: 'Search industries' },
  { param: 'company_type', label: 'Company type', control: 'pills', options: COMPANY_TYPE, excludable: true },
  { param: 'countries', label: 'Countries', control: 'select', dynamic: true, excludable: true, placeholder: 'Search countries' },
  { param: 'relocation', label: 'Relocation', control: 'pills', options: RELOCATION, excludable: true },
  { param: 'employment_type', label: 'Employment', control: 'pills', options: EMPLOYMENT, excludable: true },
  { param: 'english_level', label: 'English', control: 'pills', options: ENGLISH, excludable: true },
  { param: 'posting_language', label: 'Job language', control: 'select', dynamic: true, excludable: true, placeholder: 'Search languages' },
  { param: 'salary_currency', label: 'Currency', control: 'pills', options: CURRENCY, excludable: true },
  { param: 'company_slug', label: 'Company', control: 'remote', excludable: true, placeholder: 'Search companies', remote: companySearch },
  { param: 'source', label: 'Source', control: 'pills', options: SOURCE, excludable: true },
];

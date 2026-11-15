import { SPECIALIZATION_ALIASES } from './specializationAliases';
// The job-search facets, as a data-driven registry. Each entry declares its
// query param (matching the backend search API), section label, the control to
// render, its options (for pills/select), and whether it supports an "exclude"
// mode. The panel iterates this list; the store keys facet state by `param`.
//
// SOURCE_VALUES and the other generated value arrays (WORK_MODE_VALUES, etc.)
// in ./generated/contracts are the single source of truth for closed-vocabulary
// option values, and labels.ts is the single source of their display text. A new
// backend value appears automatically here, title-cased, unless labels.ts overrides
// it — except for categories, whose map is exhaustive and fails its test until the
// new value is named. REGION and CURRENCY are curated subsets not driven by
// generated arrays — leave those alone. Job language is a dynamic facet: its
// options come from the live distribution (any detected language), labelled via
// languageLabel.

import { metroLabel } from './metro';
import {
  WORK_MODE_VALUES,
  SENIORITY_VALUES,
  CATEGORY_VALUES,
  EMPLOYMENT_TYPE_VALUES,
  RELOCATION_VALUES,
  ENGLISH_LEVEL_VALUES,
  COMPANY_TYPE_VALUES,
  DOMAIN_VALUES,
  INDUSTRY_VALUES,
  AI_ARCHETYPE_VALUES,
  SKILL_LABELS,
  INDUSTRY_LABELS,
  ROLE_TYPE_VALUES,
  SOURCE_TYPE_VALUES,
  CLEARANCE_LEVEL_VALUES,
  PERK_VALUES,
  PERK_LABELS,
  EQUITY_VALUES,
  ENGAGEMENT_VALUES,
  ENGAGEMENT_LABELS,
  ENGAGEMENT_MARKETS,
} from './generated/contracts';
import {
  REGION_LABELS,
  SENIORITY_LABELS,
  EMPLOYMENT_LABELS,
  WORK_MODE_LABELS,
  CATEGORY_LABELS,
  DOMAIN_LABELS,
  COMPANY_TYPE_LABELS,
  ENGLISH_LEVEL_LABELS,
  RELOCATION_LABELS,
  AI_ARCHETYPE_LABELS,
  ROLE_TYPE_LABELS,
  SOURCE_TYPE_LABELS,
  CLEARANCE_LEVEL_LABELS,
  EQUITY_LABELS,
  titleCase,
} from './labels';
import { COLLECTIONS } from './collections';
import { TIMEZONE_REGIONS } from './talentFacetModel';
import { backerBadges } from './backers';
import { api } from './api';
import { sourceLabel } from './sourceLabels';

export interface FacetOption {
  value: string;
  label: string;
  /** Number of matching jobs, for dynamic (distribution-driven) options. */
  count?: number;
  /** A value the facet distribution cannot count (a reserved "not stated" value), so
   *  its pill shows no number rather than a 0 that would read as "no postings". */
  uncounted?: boolean;
  /** ISO 3166-1 alpha-2 code for country options, so the pill shows a flag icon. */
  flag?: string;
  /** Sub-heading this option renders under, for a facet whose options are not all
   *  the same sort of thing. Used by the collections facet to keep credentials
   *  (verifiable licences) visually apart from editorial collections while both
   *  stay on one `collections` query param — the facet machinery keys state and URL
   *  serialization by param, so two facet entries sharing one param would double
   *  every value in the URL and in the active-filter count. */
  group?: string;
  /** Image URL for a mark rendered inside the pill, left of the label. Used by the
   *  backer collections, whose brand mark is recognised faster than their name. */
  icon?: string;
}

type FacetControl = 'pills' | 'select' | 'tokens' | 'remote';

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
   * Curated slug → shorthand-aliases map letting the picker's search match a facet
   * value by abbreviation or synonym, not just the display label — typing "swe" or
   * "data engineer" finds the specialization. See optionMatches / FACET_ALIASES
   * (generated from the same dictionaries that tag titles).
   */
  searchAliases?: Record<string, readonly string[]>;
  /**
   * Show a brand logo (via SkillIcon) beside an option's label, for a facet whose
   * values are the `skills` dictionary — the only one techmarks.ts has real marks
   * for. A `select`/`remote` control on any other facet (company, role, industry)
   * leaves this unset: their slugs are a different vocabulary, and one could
   * coincidentally collide with a tech mark's key (a company named "Docker").
   */
  techIcons?: boolean;
  /**
   * Show the option's brand mark (via EntityLogo, resolved from its display label
   * through the logo proxy) beside its label — set only by the source facet, whose
   * values are ATS/aggregator names the proxy actually resolves.
   */
  entityLogos?: boolean;
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

/** A representative country flag code for an ISO 639-1 language (en → gb), or
 *  undefined when none is clear. A flag only decorates a language; it never decides
 *  which languages exist. */
export function languageFlag(code: string): string | undefined {
  let region: string | undefined;
  try {
    region = code === 'en' ? 'GB' : code === 'pt' ? 'PT' : new Intl.Locale(code).maximize().region;
  } catch {
    return undefined;
  }
  return region && /^[A-Z]{2}$/.test(region) ? region.toLowerCase() : undefined;
}

function languageLabel(code: string): string {
  if (!code) return code;
  try {
    return languageNames?.of(code) ?? code.toUpperCase();
  } catch {
    return code.toUpperCase();
  }
}

// Company facet values are slugs (group-ib, epam); the live distribution carries
// no display name, so title-case the slug for a readable label. Imperfect for
// acronyms (group-ib → "Group Ib") but consistent with the other facets — a real
// slug→name lookup would need a per-company fetch, which this facet forgoes.
function companyLabel(slug: string): string {
  return titleCase(slug.replace(/-/g, '_'));
}

// Option source for the Company facet's 'remote' control: the count-ordered
// companies endpoint (real names + open-job counts), not the Meili facet
// distribution (which Meili caps at 300 values and returns alphabetically — so
// popular employers never surface). An empty query returns the most active
// companies (the endpoint's first page).
export async function companySearch(query: string): Promise<FacetOption[]> {
  const { items } = await api.listCompanies(query, 20, 0);
  return items.map((c) => ({ value: c.slug, label: c.name, count: c.job_count }));
}

// Skill facet values are canonical slugs (postgresql, ci-cd, nodejs); the live
// distribution carries no display name, so map them through the generated SKILL_LABELS
// catalog (the skilltag dictionary is the source of truth). The fallback title-cases the
// slug the way companyLabel does, so a value that outlived a dictionary edit still reads
// as words rather than as data.
/** Display label for a normalised city slug: `new-york` reads as `New York`.
 *
 *  Title-casing the slug rather than looking the city up: `users.resume_cities` stores
 *  the normalisation, and the dictionary's own spellings (internal/dict/location's
 *  CityMatch.Name) are reachable only through a search API keyed by query text, not by
 *  slug. THE SEAM: a city whose real name is not its title-cased slug — 's-Hertogenbosch,
 *  Rio de Janeiro — is spelled wrong here, and fixing it means exposing a slug lookup
 *  from that dictionary rather than adding a second list of exceptions.
 *
 *  Worth having anyway: without it a card reads "berlin" beside the timezone's own
 *  "Berlin", which looks like a bug in the half that is correct. */
export function cityLabel(slug: string): string {
  return slug
    .split('-')
    .map((part) => (part ? part.charAt(0).toUpperCase() + part.slice(1) : part))
    .join(' ');
}

export function skillLabel(slug: string): string {
  return (SKILL_LABELS as Record<string, string>)[slug] ?? titleCase(slug.replace(/-/g, '_'));
}

/** Display label for a dynamic facet value: country code → name, company slug →
 *  title-cased name, else the value. */
export function dynamicLabel(param: string, value: string): string {
  if (param === 'countries') return countryLabel(value);
  if (param === 'metros') return metroLabel(value);
  if (param === 'posting_language') return languageLabel(value);
  if (param === 'company_slug') return companyLabel(value);
  if (param === 'source') return sourceLabel(value);
  // Skills have no static option list, so every surface naming a SELECTED skill — the
  // filter summary chips above all — arrives here. Without this it fell through to the
  // raw slug, and one skill was spelled two ways on one screen: "ci-cd" in the summary,
  // "CI/CD" in the panel beside it.
  if (param === 'skills') return skillLabel(value);
  return value;
}

// Role slugs carry an optional seniority grade prefix (senior_backend); the
// related-role map is keyed by the ungraded base (backend), so one entry serves
// every grade. Longest prefix first is unnecessary — the grades share no
// prefix among themselves — but drawing them from SENIORITY_VALUES keeps this in

/** Build facet options from generated values, overriding labels where given. */
function options(values: readonly string[], labels: Record<string, string> = {}): FacetOption[] {
  return values.map((value) => ({ value, label: labels[value] ?? titleCase(value) }));
}

// The backend's `regions` reach vocabulary (vocab.RegionValues): one consistent
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
const RELOCATION: FacetOption[] = options(RELOCATION_VALUES, RELOCATION_LABELS);
const ENGLISH: FacetOption[] = options(ENGLISH_LEVEL_VALUES, ENGLISH_LEVEL_LABELS);
const CATEGORY: FacetOption[] = options(CATEGORY_VALUES, CATEGORY_LABELS);

// IANA timezone REGIONS — the part before the slash. What a recruiter asking "can we
// overlap for a call" means is a continent, not a city's zone; there are dozens of zones
// per region. The list is the regions a candidate can actually be in: Antarctica and the
// single-city oddities are left out, because a chip nobody will ever match is a chip in
// the way of the ones they will.
const TIMEZONE_REGION: FacetOption[] = TIMEZONE_REGIONS.map((r) => ({ value: r, label: r }));

// The category facet options, exported for reuse outside the filter panel (the search
// profile's specialization picker) so the same labels/order are shared, not duplicated.
export const CATEGORY_OPTIONS: FacetOption[] = CATEGORY;

// Six fixed values (internal/aiarchetype) — a static select like CATEGORY/DOMAINS,
// not a dynamic (distribution-driven) control like role/skills.
const AI_ARCHETYPE: FacetOption[] = options(AI_ARCHETYPE_VALUES, AI_ARCHETYPE_LABELS);
const ROLE_TYPE: FacetOption[] = options(ROLE_TYPE_VALUES, ROLE_TYPE_LABELS);
const SOURCE_TYPE: FacetOption[] = options(SOURCE_TYPE_VALUES, SOURCE_TYPE_LABELS);

// Reserved `clearance_levels` value selecting postings with no explicit reference to a
// clearance (search.ClearanceLevelUnspecified). It is the old `requires_clearance=false`
// under a new name, so it leads the chips: it is the one most people want. Uncounted —
// the facet distribution counts the levels postings carry, and "none" is the absence
// of one, so any number shown for it would be invented.
export const CLEARANCE_UNSPECIFIED = 'none';
const CLEARANCE_LEVEL: FacetOption[] = [
  { value: CLEARANCE_UNSPECIFIED, label: 'No explicit reference to clearance', uncounted: true },
  ...options(CLEARANCE_LEVEL_VALUES, CLEARANCE_LEVEL_LABELS),
];

// Reserved `equity` value selecting the postings that state nothing about equity
// (search.EquityNotStated) — the attribute's absence, not a member of the vocabulary,
// so it is added here rather than generated. Last, unlike clearance's `none`: people
// filter on equity to FIND it. Uncounted for the same reason as CLEARANCE_UNSPECIFIED.
const EQUITY_NOT_STATED = 'not_stated';
const EQUITY: FacetOption[] = [
  ...options(EQUITY_VALUES, EQUITY_LABELS),
  { value: EQUITY_NOT_STATED, label: 'Not stated', uncounted: true },
];

// Work-mode and region options, exported for the profile's location preferences editor so
// it shares the filter panel's vocabulary/order instead of duplicating it.
export const WORK_MODE_OPTIONS: FacetOption[] = WORK_MODE;
export const REGION_OPTIONS: FacetOption[] = REGION;

// Seniority and employment-type options, exported for the submit form so a submitter's
// choice aligns with the filter panel's vocabulary/order instead of duplicating it.
export const SENIORITY_OPTIONS: FacetOption[] = SENIORITY;
export const EMPLOYMENT_TYPE_OPTIONS: FacetOption[] = EMPLOYMENT;

const DOMAINS: FacetOption[] = options(DOMAIN_VALUES, DOMAIN_LABELS);

// The curated company-industry vocabulary (internal/industrytag) — not a level
// beneath domains but the company catalogue's single industry axis, which the
// backend answers from the curated column and the job-derived domains alike.
// Generated, like DOMAINS, so the options cannot drift from the Go dictionary.
const INDUSTRIES: FacetOption[] = options(INDUSTRY_VALUES, INDUSTRY_LABELS);
// The perks the description states (internal/dict/perks); values and labels are both
// generated from the dictionary, so a chip cannot offer a perk it never writes.
const PERKS: FacetOption[] = options(PERK_VALUES, PERK_LABELS);
// The legal terms a posting states it engages on (internal/dict/engagement), each in its
// market's own term. Generated like PERKS, so a chip cannot offer a value never written.
const ENGAGEMENT: FacetOption[] = options(ENGAGEMENT_VALUES, ENGAGEMENT_LABELS);
// The same options grouped by the market whose term each is ("United Kingdom": the two
// IR35 chips), in vocabulary order — the filter pane draws one chip block per market.
export const ENGAGEMENT_GROUPS: { market: string; options: FacetOption[] }[] = ENGAGEMENT.reduce(
  (groups, o) => {
    const market = ENGAGEMENT_MARKETS[o.value as keyof typeof ENGAGEMENT_MARKETS];
    const last = groups.at(-1);
    if (last?.market === market) last.options.push(o);
    else groups.push({ market, options: [o] });
    return groups;
  },
  [] as { market: string; options: FacetOption[] }[],
);

// The market blocks worth drawing under the current scope: one that matches postings or
// holds a selection. Without counts (still loading, or none returned) every block shows.
// A search narrowed to the UK therefore offers the IR35 chips and not twenty others,
// which is the country scoping the filter needs, read off the counts it already has.
export function visibleEngagementGroups(
  counts: Record<string, number> | undefined,
  selected: readonly string[],
): typeof ENGAGEMENT_GROUPS {
  if (!counts) return ENGAGEMENT_GROUPS;
  return ENGAGEMENT_GROUPS.filter((g) =>
    g.options.some((o) => (counts[o.value] ?? 0) > 0 || selected.includes(o.value)),
  );
}
// The words people type for a perk that its label does not contain, so the filter search
// finds "pension" under Retirement Plan and "pto" under Generous Paid Time Off.
const PERK_ALIASES: Record<string, readonly string[]> = {
  generous_pto: ['pto', 'vacation', 'holiday', 'annual leave', 'time off'],
  four_day_week: ['4 day week', 'four-day week'],
  '401k_match': ['401k', 'retirement match', 'employer match'],
  parental_leave: ['maternity', 'paternity', 'family leave'],
  retirement_plan: ['pension', '401k', 'superannuation', 'rrsp'],
  tuition_reimbursement: ['tuition', 'education assistance'],
  unlimited_pto: ['unlimited vacation', 'unlimited holiday'],
  health_insurance: ['medical', 'healthcare', 'private medical'],
  mental_health: ['therapy', 'counseling', 'wellbeing'],
  learning_budget: ['training budget', 'conference', 'l&d', 'professional development'],
  home_office_stipend: ['wfh stipend', 'remote stipend', 'home office'],
  sabbatical: ['career break'],
  sign_on_bonus: ['signing bonus', 'joining bonus'],
};

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

// The salary currency vocabulary, reused by the submit form so a submitter's choice
// aligns with the filter's currency facet.
export const CURRENCY_OPTIONS: FacetOption[] = CURRENCY;

// The company-tag registry as pill options, from the generated registry the
// /collections hub also renders, so the label/slug pairs cannot drift.
// Credentials last, under their own sub-heading: a credential is a verifiable
// licence drawn from a public register, not one of our curated picks, and running
// them together would read as though we vouched for both the same way. They stay on
// one `collections` param regardless: facet state and URL serialization are keyed by
// param, so a second facet entry sharing it would append every value twice.
//
// Everything that is not a credential is offered ungrouped, rather than listing the
// kinds that qualify. Naming them was how `yc` and `techstars` fell out of the
// filters entirely the day they became backers — filterable through the API,
// unreachable in the UI, and silent about it.
const COLLECTION: FacetOption[] = [
  ...COLLECTIONS.filter((c) => c.kind !== 'credential').map((c) => ({
    value: c.slug,
    label: c.title,
    icon: backerBadges([c.slug])[0]?.mark,
  })),
  ...COLLECTIONS.filter((c) => c.kind === 'credential').map((c) => ({
    value: c.slug,
    label: c.title,
    group: 'Employer credentials',
  })),
];

// The job filter modal splits COLLECTION across two panes instead of one grouped
// list: the curated picks stay on Industry & collection, the credentials move to
// their own block on the Relocation pane (they're evidence of visa/sponsorship
// eligibility, which is what that pane is about). Both stay ChipFacet `options`
// overrides on the same `collections` param rather than becoming separate FACETS
// entries — a second entry sharing the param would double its value in the query
// string. The company modal has no Relocation pane to split into, so it keeps using
// COLLECTION (and its "Employer credentials" sub-heading) unchanged.
export const JOB_COLLECTION: FacetOption[] = COLLECTION.filter((o) => o.group !== 'Employer credentials');

// Stripped of `group`: this renders as its own ChipFacet block with its own "Employer
// credentials" header, so the in-list sub-heading PillGroup would otherwise add above
// the first pill (COLLECTION's grouping cue for the combined company-modal list) would
// just repeat that header.
export const EMPLOYER_CREDENTIALS: FacetOption[] = COLLECTION.filter((o) => o.group === 'Employer credentials').map(
  ({ group: _group, ...o }) => o,
);

// Company-size buckets — the vocab.CompanySizeValues vocabulary. Not exported as
// a generated values array (it's a scalar enrichment field, not a search facet on
// jobs), so the closed set is spelled out here like CURRENCY; the values are
// already display-ready.
const COMPANY_SIZE: FacetOption[] = ['1-10', '11-50', '51-200', '201-500', '501-1000', '1000+'].map((value) => ({
  value,
  label: value,
}));

// The ISO 3166-1 alpha-2 code set — the country vocabulary the location dictionary
// draws from — plus 'xk', Kosovo's user-assigned code: ISO has not assigned it, but
// the location dictionary emits it and Intl names it, so leaving it out made Kosovo
// the one country a posting could carry and no picker could select (freehire#3847). The company country facet is a static searchable select over this
// (labelled via countryLabel), not a live distribution: the companies list is plain
// SQL with no facet-count endpoint yet, so options carry no counts.
export const ISO_COUNTRY_CODES = [
  'ad',
  'ae',
  'af',
  'ag',
  'ai',
  'al',
  'am',
  'ao',
  'aq',
  'ar',
  'as',
  'at',
  'au',
  'aw',
  'ax',
  'az',
  'ba',
  'bb',
  'bd',
  'be',
  'bf',
  'bg',
  'bh',
  'bi',
  'bj',
  'bl',
  'bm',
  'bn',
  'bo',
  'bq',
  'br',
  'bs',
  'bt',
  'bv',
  'bw',
  'by',
  'bz',
  'ca',
  'cc',
  'cd',
  'cf',
  'cg',
  'ch',
  'ci',
  'ck',
  'cl',
  'cm',
  'cn',
  'co',
  'cr',
  'cu',
  'cv',
  'cw',
  'cx',
  'cy',
  'cz',
  'de',
  'dj',
  'dk',
  'dm',
  'do',
  'dz',
  'ec',
  'ee',
  'eg',
  'eh',
  'er',
  'es',
  'et',
  'fi',
  'fj',
  'fk',
  'fm',
  'fo',
  'fr',
  'ga',
  'gb',
  'gd',
  'ge',
  'gf',
  'gg',
  'gh',
  'gi',
  'gl',
  'gm',
  'gn',
  'gp',
  'gq',
  'gr',
  'gs',
  'gt',
  'gu',
  'gw',
  'gy',
  'hk',
  'hm',
  'hn',
  'hr',
  'ht',
  'hu',
  'id',
  'ie',
  'il',
  'im',
  'in',
  'io',
  'iq',
  'ir',
  'is',
  'it',
  'je',
  'jm',
  'jo',
  'jp',
  'ke',
  'kg',
  'kh',
  'ki',
  'km',
  'kn',
  'kp',
  'kr',
  'kw',
  'ky',
  'kz',
  'la',
  'lb',
  'lc',
  'li',
  'lk',
  'lr',
  'ls',
  'lt',
  'lu',
  'lv',
  'ly',
  'ma',
  'mc',
  'md',
  'me',
  'mf',
  'mg',
  'mh',
  'mk',
  'ml',
  'mm',
  'mn',
  'mo',
  'mp',
  'mq',
  'mr',
  'ms',
  'mt',
  'mu',
  'mv',
  'mw',
  'mx',
  'my',
  'mz',
  'na',
  'nc',
  'ne',
  'nf',
  'ng',
  'ni',
  'nl',
  'no',
  'np',
  'nr',
  'nu',
  'nz',
  'om',
  'pa',
  'pe',
  'pf',
  'pg',
  'ph',
  'pk',
  'pl',
  'pm',
  'pn',
  'pr',
  'ps',
  'pt',
  'pw',
  'py',
  'qa',
  're',
  'ro',
  'rs',
  'ru',
  'rw',
  'sa',
  'sb',
  'sc',
  'sd',
  'se',
  'sg',
  'sh',
  'si',
  'sj',
  'sk',
  'sl',
  'sm',
  'sn',
  'so',
  'sr',
  'ss',
  'st',
  'sv',
  'sx',
  'sy',
  'sz',
  'tc',
  'td',
  'tf',
  'tg',
  'th',
  'tj',
  'tk',
  'tl',
  'tm',
  'tn',
  'to',
  'tr',
  'tt',
  'tv',
  'tw',
  'tz',
  'ua',
  'ug',
  'um',
  'us',
  'uy',
  'uz',
  'va',
  'vc',
  've',
  'vg',
  'vi',
  'vn',
  'vu',
  'wf',
  'ws',
  'xk',
  'ye',
  'yt',
  'za',
  'zm',
  'zw',
];

const COUNTRY: FacetOption[] = ISO_COUNTRY_CODES.map((value) => ({
  value,
  label: countryLabel(value),
  flag: value,
})).sort((a, b) => a.label.localeCompare(b.label));

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
// Deterministic company maturity (see the company-classification-facets change),
// derived server-side from company signals; unknown (NULL) companies match nothing.
const MATURITY: FacetOption[] = options(['startup', 'scaleup', 'enterprise', 'government'], {
  startup: 'Startup',
  scaleup: 'Scaleup',
  enterprise: 'Enterprise',
  government: 'Government',
});
const YC_FLAGS: FacetOption[] = options(['top_company', 'hiring'], {
  top_company: 'YC Top Company',
  hiring: 'Hiring',
});
// YC batch labels are the verbatim source strings ("Winter 2012"). Generate the full
// season×year grid so the searchable select covers any batch; phantom combinations
// that no company has simply match nothing. The grid runs from the first YC batch
// (2005) through next year so a fresh batch is covered as soon as it appears.
const YC_BATCH: FacetOption[] = (() => {
  const max = new Date().getFullYear() + 1;
  return Array.from({ length: max - 2005 + 1 }, (_, i) => max - i).flatMap((y) =>
    ['Winter', 'Spring', 'Summer', 'Fall'].map((s) => ({ value: `${s} ${y}`, label: `${s} ${y}` })),
  );
})();
/** The Talent Network catalogue's facets.
 *
 *  All include-only — the catalogue endpoint has no `_exclude` and no AND/OR mode — so
 *  every entry sets `excludable: false` and none carries `hasAndOr`.
 *
 *  Skills and city are `dynamic`: their options come from the live facet distribution
 *  (`/talent/facets`) rather than a static list, which is what makes them searchable and
 *  what puts a count beside each value. A closed vocabulary offered as pills and an open
 *  one offered as a searched select are not two styles of the same control — thousands of
 *  skills as pills is the wrong control, and a skill offered without a count cannot be
 *  told apart from one nobody carries.
 *
 *  `specializations` and `categories` share the category vocabulary and ask different
 *  questions of it: what a candidate SAYS they want, and what their most recent title
 *  resolves to. Both are selects rather than pills because 48 values is a wall. */
export const TALENT_FACETS: FacetDef[] = [
  {
    param: 'specializations',
    label: 'Open to',
    control: 'select',
    options: CATEGORY,
    excludable: false,
    placeholder: 'Search disciplines',
  },
  {
    param: 'categories',
    label: 'Has worked in',
    control: 'select',
    options: CATEGORY,
    excludable: false,
    placeholder: 'Search disciplines',
  },
  { param: 'seniorities', label: 'Grade', control: 'pills', options: SENIORITY, excludable: false },
  {
    param: 'skills',
    label: 'Skills',
    control: 'select',
    dynamic: true,
    excludable: false,
    placeholder: 'Search skills',
    techIcons: true,
  },
  { param: 'tz', label: 'Timezone', control: 'pills', options: TIMEZONE_REGION, excludable: false },
  { param: 'cities', label: 'City', control: 'select', dynamic: true, excludable: false, placeholder: 'Search cities' },
];

export const COMPANY_FACETS: FacetDef[] = [
  { param: 'collections', label: 'Collection', control: 'pills', options: COLLECTION, excludable: false },
  { param: 'regions', label: 'Region', control: 'pills', options: REGION, excludable: false },
  { param: 'remote_regions', label: 'Remote hiring', control: 'pills', options: REGION, excludable: false },
  {
    param: 'countries',
    label: 'Country',
    control: 'select',
    options: COUNTRY,
    excludable: false,
    placeholder: 'Search countries',
  },
  // The catalogue's only industry control. The backend matches it against the curated
  // industries column AND the job-derived domains one (internal/industrytag maps
  // between the vocabularies), so the Domain facet that used to sit here asked the
  // same question of one of the same columns and is gone. `domains` remains a working
  // query parameter for a caller who wants the raw job-derived value.
  {
    param: 'industries',
    label: 'Industry',
    control: 'select',
    options: INDUSTRIES,
    excludable: false,
    placeholder: 'Search industries',
  },
  { param: 'company_type', label: 'Company type', control: 'pills', options: COMPANY_TYPE, excludable: false },
  { param: 'company_size', label: 'Company size', control: 'pills', options: COMPANY_SIZE, excludable: false },
  { param: 'maturity', label: 'Company stage', control: 'pills', options: MATURITY, excludable: false },
  { param: 'yc_status', label: 'YC status', control: 'pills', options: YC_STATUS, excludable: false },
  { param: 'yc_stage', label: 'YC stage', control: 'pills', options: YC_STAGE, excludable: false },
  { param: 'yc_flags', label: 'YC highlights', control: 'pills', options: YC_FLAGS, excludable: false },
  {
    param: 'yc_batch',
    label: 'YC batch',
    control: 'select',
    options: YC_BATCH,
    excludable: false,
    placeholder: 'Search YC batches',
  },
];

export const FACETS: FacetDef[] = [
  { param: 'collections', label: 'Collection', control: 'pills', options: COLLECTION, excludable: false },
  { param: 'regions', label: 'Region', control: 'pills', options: JOB_REGION, excludable: true },
  { param: 'work_mode', label: 'Work format', control: 'pills', options: WORK_MODE, excludable: true },
  {
    param: 'category',
    label: 'Specialization',
    control: 'select',
    options: CATEGORY,
    excludable: true,
    placeholder: 'Search specializations',
    searchAliases: SPECIALIZATION_ALIASES,
  },
  {
    param: 'ai_archetype',
    label: 'AI Specialization',
    control: 'select',
    options: AI_ARCHETYPE,
    excludable: true,
    placeholder: 'Search AI specializations',
  },
  { param: 'seniority', label: 'Seniority', control: 'pills', options: SENIORITY, excludable: true },
  { param: 'role_type', label: 'Role type', control: 'pills', options: ROLE_TYPE, excludable: true },
  {
    param: 'skills',
    label: 'Skills',
    control: 'select',
    dynamic: true,
    excludable: true,
    hasAndOr: true,
    placeholder: 'Search skills',
    techIcons: true,
  },
  {
    param: 'domains',
    label: 'Industry',
    control: 'select',
    options: DOMAINS,
    excludable: true,
    placeholder: 'Search industries',
  },
  { param: 'company_type', label: 'Company type', control: 'pills', options: COMPANY_TYPE, excludable: true },
  {
    param: 'countries',
    label: 'Countries',
    control: 'select',
    dynamic: true,
    excludable: true,
    placeholder: 'Search countries',
  },
  { param: 'cities', label: 'City', control: 'select', dynamic: true, excludable: true, placeholder: 'Search cities' },
  {
    param: 'metros',
    label: 'Metro area',
    control: 'select',
    dynamic: true,
    excludable: true,
    placeholder: 'Search metro areas',
  },
  { param: 'relocation', label: 'Relocation', control: 'pills', options: RELOCATION, excludable: true },
  { param: 'employment_type', label: 'Employment', control: 'pills', options: EMPLOYMENT, excludable: true },
  // Excludable: both values are statements, so "hide Inside IR35" keeps the postings that
  // do not say — unlike perks, where an exclusion would hide jobs that never mentioned one.
  { param: 'engagement', label: 'Contract terms', control: 'pills', options: ENGAGEMENT, excludable: true },
  { param: 'english_level', label: 'English', control: 'pills', options: ENGLISH, excludable: true },
  {
    param: 'posting_language',
    label: 'Job language',
    control: 'select',
    dynamic: true,
    excludable: true,
    placeholder: 'Search languages',
  },
  { param: 'reality', label: 'Posting reality', control: 'pills', options: REALITY, excludable: true },
  { param: 'salary_currency', label: 'Currency', control: 'pills', options: CURRENCY, excludable: true },
  // Include-only: a posting without a perk is one whose description does not MENTION it,
  // so "exclude 401(k) matching" would hide jobs that may well offer it.
  {
    param: 'perks',
    label: 'Perks mentioned',
    control: 'pills',
    options: PERKS,
    excludable: false,
    hasAndOr: true,
    searchAliases: PERK_ALIASES,
  },
  { param: 'equity', label: 'Equity', control: 'pills', options: EQUITY, excludable: true },
  {
    param: 'company_slug',
    label: 'Company',
    control: 'remote',
    excludable: true,
    placeholder: 'Search companies',
    remote: companySearch,
  },
  { param: 'source_type', label: 'Source type', control: 'pills', options: SOURCE_TYPE, excludable: true },
  { param: 'clearance_levels', label: 'Clearance level', control: 'pills', options: CLEARANCE_LEVEL, excludable: true },
  {
    param: 'source',
    label: 'Source',
    control: 'select',
    dynamic: true,
    excludable: true,
    placeholder: 'Search sources',
    entityLogos: true,
  },
];

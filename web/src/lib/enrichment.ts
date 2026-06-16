// Presentation helpers for a job's AI enrichment. Pure functions that turn the
// controlled-vocabulary codes (validated server-side) into display labels.
// Unknown codes fall back to a humanized form so a future vocabulary addition
// never renders blank — the SPA never re-validates, it only formats.

import type { Enrichment, Job } from './types';

/** One value within a facet row: its display text and, when the facet maps to a
 *  job-search filter, the /jobs URL that applies it. */
export interface FacetValue {
  text: string;
  href?: string;
}

/** A labelled facet row. Most facets carry a single value; the array-valued ones
 *  (region, country, industry) carry one entry per code, each independently
 *  clickable. */
export interface Facet {
  label: string;
  values: FacetValue[];
}

const SENIORITY: Record<string, string> = {
  intern: 'Intern',
  junior: 'Junior',
  middle: 'Middle',
  senior: 'Senior',
  lead: 'Lead',
  staff: 'Staff',
  principal: 'Principal',
  c_level: 'C-level',
};

const EMPLOYMENT: Record<string, string> = {
  full_time: 'Full-time',
  part_time: 'Part-time',
  contract: 'Contract',
  internship: 'Internship',
};

const WORK_MODE: Record<string, string> = {
  remote: 'Remote',
  hybrid: 'Hybrid',
  onsite: 'On-site',
};

// Region codes (the top-level `regions` facet) → readable labels. Unmapped codes humanize.
const REGION: Record<string, string> = {
  global: 'Global',
  eu: 'Europe',
  emea: 'EMEA',
  eea: 'EEA',
  uk: 'UK',
  americas: 'Americas',
  north_america: 'North America',
  latam: 'LATAM',
  apac: 'APAC',
  mena: 'MENA',
  africa: 'Africa',
  us: 'USA',
  ru: 'Russia',
  cis: 'CIS',
  central_asia: 'Central Asia',
};

const RELOCATION: Record<string, string> = {
  not_supported: 'Not supported',
  supported: 'Supported',
  required: 'Required',
};

const ENGLISH_LEVEL: Record<string, string> = {
  a1: 'A1',
  a2: 'A2',
  b1: 'B1',
  b2: 'B2',
  c1: 'C1',
  c2: 'C2',
  native: 'Native',
};

// Only the acronym/compound domains need a label; the rest (Crypto, Healthcare,
// Media…) humanize cleanly, so they fall through to `humanize` like CATEGORY.
const DOMAINS: Record<string, string> = {
  fintech: 'FinTech',
  ecommerce: 'E-commerce',
  saas: 'SaaS',
  gamedev: 'GameDev',
  edtech: 'EdTech',
  adtech: 'AdTech',
  govtech: 'GovTech',
};

const CATEGORY: Record<string, string> = {
  ml_ai: 'ML / AI',
  qa: 'QA',
  devops: 'DevOps',
  data_engineering: 'Data engineering',
  data_science: 'Data science',
  project_management: 'Project management',
};

const COMPANY_TYPE: Record<string, string> = {
  product: 'Product',
  startup: 'Startup',
  outsource: 'Outsource',
  outstaff: 'Outstaff',
  agency: 'Agency',
  inhouse: 'In-house',
  government: 'Government',
};

const CURRENCY_SYMBOL: Record<string, string> = { USD: '$', EUR: '€', GBP: '£' };

const PERIOD_SUFFIX: Record<string, string> = {
  month: ' / mo',
  day: ' / day',
  hour: ' / hr',
  // `year` is the implicit default and reads cleaner without a suffix.
};

/** Title-case an unknown snake_case code (e.g. "data_engineering" → "Data engineering"). */
function humanize(value: string): string {
  const spaced = value.replace(/_/g, ' ');
  return spaced.charAt(0).toUpperCase() + spaced.slice(1);
}

/** Look a code up in its label map, humanizing anything outside the map. */
function label(map: Record<string, string>, value: string): string {
  return map[value] ?? humanize(value);
}

/** The /jobs URL that filters by a single facet value. Param names match the
 *  search API (see facets.ts / filters.svelte.ts). */
export function filterHref(param: string, value: string): string {
  return `/jobs?${param}=${encodeURIComponent(value)}`;
}

/** Group thousands with thin spaces, matching the salary line in the design. */
function groupThousands(n: number): string {
  return n.toLocaleString('en-US').replace(/,/g, ' ');
}

/**
 * Render the compensation line, or null when no salary is stated. Handles the
 * full range, a min-only floor ("from …"), and a max-only ceiling ("up to …").
 * The currency symbol and period suffix trail the amount, as in the design.
 */
export function formatSalary(e: Enrichment): string | null {
  const { salary_min, salary_max } = e;
  if (salary_min == null && salary_max == null) return null;

  const symbol = e.salary_currency
    ? (CURRENCY_SYMBOL[e.salary_currency] ?? e.salary_currency)
    : '';
  const period = e.salary_period ? (PERIOD_SUFFIX[e.salary_period] ?? '') : '';
  const tail = `${symbol}${period}`;

  let amount: string;
  if (salary_min != null && salary_max != null) {
    amount = `${groupThousands(salary_min)} – ${groupThousands(salary_max)}`;
  } else if (salary_min != null) {
    amount = `from ${groupThousands(salary_min)}`;
  } else {
    amount = `up to ${groupThousands(salary_max as number)}`;
  }

  return tail ? `${amount} ${tail}` : amount;
}

/**
 * The work-arrangement label for compact contexts (list cards): the resolved
 * top-level `work_mode` (LLM value, else the one parsed from the location), or
 * null when neither stated it.
 */
export function workArrangement(job: Pick<Job, 'work_mode'>): string | null {
  return job.work_mode ? label(WORK_MODE, job.work_mode) : null;
}

/** The seniority label (e.g. `Senior`, `C-level`), or null when unstated. */
export function seniorityLabel(e: Pick<Enrichment, 'seniority'>): string | null {
  return e.seniority ? label(SENIORITY, e.seniority) : null;
}

/**
 * The job's geographic area as a concise label from the top-level `regions` —
 * e.g. `Global`, `Europe`, `USA`. Meaningful for any work mode (a remote role's
 * reach or an onsite role's area). Null when regions is unknown (empty is
 * unknown, not global).
 */
export function regionLabel(job: Pick<Job, 'regions'>): string | null {
  if (!job.regions?.length) return null;
  return job.regions.map((r) => label(REGION, r)).join(', ');
}

/**
 * The short tag row shown on a list card's header: work arrangement, region,
 * employment type, and grade — only those that are stated, in that order.
 * Compact by design (the full facet set lives on the detail page).
 */
export function cardTags(job: Job): string[] {
  const e = job.enrichment;
  const tags: string[] = [];

  const arrangement = workArrangement(job);
  if (arrangement) tags.push(arrangement);
  const region = regionLabel(job);
  if (region) tags.push(region);
  if (e?.employment_type) tags.push(label(EMPLOYMENT, e.employment_type));
  if (e?.seniority) tags.push(label(SENIORITY, e.seniority));

  return tags;
}

/**
 * Ordered facets for the summary meta-row. Only facets with a stated value are
 * included, so an empty enrichment yields an empty list and the row hides
 * entirely. Order moves from work arrangement → role → eligibility → company,
 * mirroring the reference layout.
 */
export function summaryFacets(job: Job): Facet[] {
  const e = job.enrichment ?? {};
  const facets: Facet[] = [];

  // A scalar facet that maps to a search filter: one clickable value, its text
  // resolved from the code via the facet's label map.
  const link = (
    name: string,
    param: string,
    code: string | null | undefined,
    map: Record<string, string>,
  ) => {
    if (code) facets.push({ label: name, values: [{ text: label(map, code), href: filterHref(param, code) }] });
  };
  // An array facet (region/country/industry): one clickable value per code.
  const links = (
    name: string,
    param: string,
    codes: string[] | undefined,
    toText: (code: string) => string,
  ) => {
    if (codes?.length) {
      facets.push({ label: name, values: codes.map((c) => ({ text: toText(c), href: filterHref(param, c) })) });
    }
  };
  // A facet with no matching filter: plain, non-clickable text.
  const plain = (name: string, text: string | null | undefined) => {
    if (text) facets.push({ label: name, values: [{ text }] });
  };

  link('Work format', 'work_mode', job.work_mode, WORK_MODE);
  plain('Location', job.location);
  links('Region', 'regions', job.regions, (r) => label(REGION, r));
  link('Work type', 'employment_type', e.employment_type, EMPLOYMENT);
  link('Grade', 'seniority', e.seniority, SENIORITY);
  plain('Experience', e.experience_years_min != null ? `${e.experience_years_min}+ yrs` : null);
  // english_level carries a 'none' sentinel that must not render as a facet.
  const english = e.english_level && e.english_level !== 'none' ? e.english_level : null;
  link('English', 'english_level', english, ENGLISH_LEVEL);
  link('Category', 'category', e.category, CATEGORY);
  links('Country', 'countries', job.countries, (c) => c.toUpperCase());
  link('Relocation', 'relocation', e.relocation, RELOCATION);
  if (e.visa_sponsorship === true) {
    facets.push({ label: 'Visa', values: [{ text: 'Sponsored', href: filterHref('visa_sponsorship', 'true') }] });
  }
  link('Company', 'company_type', e.company_type, COMPANY_TYPE);
  plain('Size', e.company_size);
  links('Domains', 'domains', e.domains, (d) => label(DOMAINS, d));

  return facets;
}

// Presentation helpers for a job's AI enrichment. Pure functions that turn the
// controlled-vocabulary codes (validated server-side) into display labels, all of
// them read from labels.ts so this page cannot disagree with the filter panel.
// Unknown codes fall back to a sentence-cased form so a future vocabulary addition
// never renders blank — the SPA never re-validates, it only formats.

import type { Enrichment, Job } from './types';
import type { Card as JobCard } from './generated/contracts';
import { countryLabel } from './facets';
import {
  REGION_LABELS, SENIORITY_LABELS, EMPLOYMENT_LABELS, WORK_MODE_LABELS,
  CATEGORY_LABELS, DOMAIN_LABELS, COMPANY_TYPE_LABELS, ENGLISH_LEVEL_LABELS,
  RELOCATION_LABELS,
} from './labels';

/** One value within a facet row: its display text and, when the facet maps to a
 *  job-search filter, the /jobs URL that applies it. `flag` carries an ISO 3166-1
 *  alpha-2 code for the country facet, so the renderer shows a flag icon (with
 *  `text` as its accessible name) instead of the bare code. */
interface FacetValue {
  text: string;
  href?: string;
  flag?: string;
}

/** A labelled facet row. Most facets carry a single value; the array-valued ones
 *  (region, country, industry) carry one entry per code, each independently
 *  clickable. */
export interface Facet {
  label: string;
  values: FacetValue[];
}

const CURRENCY_SYMBOL: Record<string, string> = { USD: '$', EUR: '€', GBP: '£' };

const PERIOD_SUFFIX: Record<string, string> = {
  month: ' / mo',
  day: ' / day',
  hour: ' / hr',
  // `year` is the implicit default and reads cleaner without a suffix.
};

/** Sentence-case an unknown snake_case code (e.g. "data_engineering" → "Data
 *  engineering"). Deliberately different from labels.ts's titleCase: the facet rows on
 *  this page read as prose. Only reached by codes outside their map — the category
 *  vocabulary is labelled exhaustively, so it never lands here. */
function sentenceCase(value: string): string {
  const spaced = value.replace(/_/g, ' ');
  return spaced.charAt(0).toUpperCase() + spaced.slice(1);
}

/** Look a code up in its label map, sentence-casing anything outside the map. */
function label(map: Record<string, string>, value: string): string {
  return map[value] ?? sentenceCase(value);
}

/** The job-feed URL that filters by a single facet value. The feed lives at `/jobs`;
 *  param names match the search API (see facets.ts / filters.ts). */
export function filterHref(param: string, value: string): string {
  return `/jobs?${param}=${encodeURIComponent(value)}`;
}

/** Compact an amount the way companyDetails.ts's funding figures already read:
 *  1_200_000 → "1.2M", 30_000 → "30K", below a thousand printed in full. */
function compactAmount(n: number): string {
  if (n >= 1_000_000_000) return `${(n / 1_000_000_000).toFixed(n % 1_000_000_000 ? 1 : 0)}B`;
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(n % 1_000_000 ? 1 : 0)}M`;
  if (n >= 1_000) return `${Math.round(n / 1_000)}K`;
  return n.toLocaleString('en-US');
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
    amount = `${compactAmount(salary_min)} – ${compactAmount(salary_max)}`;
  } else if (salary_min != null) {
    amount = `from ${compactAmount(salary_min)}`;
  } else {
    amount = `up to ${compactAmount(salary_max as number)}`;
  }

  return tail ? `${amount} ${tail}` : amount;
}

/**
 * The work-arrangement label for compact contexts (list cards): the resolved
 * top-level `work_mode` (LLM value, else the one parsed from the location), or
 * null when neither stated it.
 */
export function workArrangement(job: Pick<Job, 'work_mode'>): string | null {
  return job.work_mode ? label(WORK_MODE_LABELS, job.work_mode) : null;
}

/** The seniority label (e.g. `Senior`, `C-level`), or null when unstated. */
export function seniorityLabel(e: Pick<Enrichment, 'seniority'>): string | null {
  return e.seniority ? label(SENIORITY_LABELS, e.seniority) : null;
}

/**
 * The job's geographic area as a concise label from the top-level `regions` —
 * e.g. `Global`, `Europe`, `USA`. Meaningful for any work mode (a remote role's
 * reach or an onsite role's area). Null when regions is unknown (empty is
 * unknown, not global).
 */
function regionLabel(job: Pick<Job, 'regions'>): string | null {
  if (!job.regions?.length) return null;
  return job.regions.map((r) => label(REGION_LABELS, r)).join(', ');
}

/**
 * The short tag row shown on a list card's header: work arrangement, region,
 * employment type, and grade — only those that are stated, in that order.
 * Compact by design (the full facet set lives on the detail page).
 */
export function cardTags(job: Job): string[] {
  const e = job.enrichment;
  return tagRow({
    work_mode: job.work_mode,
    regions: job.regions,
    employment_type: e?.employment_type,
    seniority: e?.seniority,
  });
}

/**
 * The same row, from the listing's card projection. The card carries these four facets flat —
 * the server resolved the dict-then-LLM geography before sending — where a full `Job` still
 * keeps two of them inside `enrichment`. One row builder, two shapes of input, so a tracked
 * application and a catalogue result cannot describe the same job differently.
 */
export function cardTagsFromCard(card: JobCard): string[] {
  return tagRow(card);
}

function tagRow(f: {
  work_mode?: string;
  regions?: string[];
  employment_type?: string;
  seniority?: string;
}): string[] {
  const tags: string[] = [];

  const arrangement = workArrangement({ work_mode: f.work_mode ?? '' });
  if (arrangement) tags.push(arrangement);
  const region = regionLabel({ regions: f.regions ?? [] });
  if (region) tags.push(region);
  if (f.employment_type) tags.push(label(EMPLOYMENT_LABELS, f.employment_type));
  if (f.seniority) tags.push(label(SENIORITY_LABELS, f.seniority));

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
  // An array facet (region/country/industry): one clickable value per code. When
  // `flag` is set, each value also carries its code so the renderer shows a flag icon.
  const links = (
    name: string,
    param: string,
    codes: string[] | undefined,
    toText: (code: string) => string,
    flag = false,
  ) => {
    if (codes?.length) {
      facets.push({
        label: name,
        values: codes.map((c) => ({ text: toText(c), href: filterHref(param, c), ...(flag ? { flag: c } : {}) })),
      });
    }
  };
  // A facet with no matching filter: plain, non-clickable text.
  const plain = (name: string, text: string | null | undefined) => {
    if (text) facets.push({ label: name, values: [{ text }] });
  };

  link('Work format', 'work_mode', job.work_mode, WORK_MODE_LABELS);
  plain('Location', job.location);
  links('Region', 'regions', job.regions, (r) => label(REGION_LABELS, r));
  link('Work type', 'employment_type', e.employment_type, EMPLOYMENT_LABELS);
  link('Grade', 'seniority', e.seniority, SENIORITY_LABELS);
  plain('Experience', e.experience_years_min != null ? `${e.experience_years_min}+ yrs` : null);
  // english_level carries a 'none' sentinel that must not render as a facet.
  const english = e.english_level && e.english_level !== 'none' ? e.english_level : null;
  link('English', 'english_level', english, ENGLISH_LEVEL_LABELS);
  link('Category', 'category', e.category, CATEGORY_LABELS);
  links('Country', 'countries', job.countries, countryLabel, true);
  link('Relocation', 'relocation', e.relocation, RELOCATION_LABELS);
  if (e.visa_sponsorship === true) {
    facets.push({ label: 'Visa', values: [{ text: 'Sponsored', href: filterHref('visa_sponsorship', 'true') }] });
  }
  link('Company', 'company_type', e.company_type, COMPANY_TYPE_LABELS);
  plain('Size', e.company_size);
  links('Domains', 'domains', e.domains, (d) => label(DOMAIN_LABELS, d));

  return facets;
}

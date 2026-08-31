// Pure helpers behind the /roles landing pages — the category × country product the
// two-axis search ("backend jobs in germany") has no page for. Kept free of fetch and
// Svelte so it is unit-testable in isolation; the routes fetch and feed these.
//
// Three of the functions here exist because a block that renders unconditionally
// would misdescribe the catalogue. The measurements that put them there are named at
// each one — they are properties of how sparse an annotation is, not style choices,
// so they are gates rather than formatting.
//
// Display text is NOT owned here: categories read labels.ts and countries read
// facets.ts, the same maps the filter panel reads, so a category cannot be spelled
// one way on an indexed page and another way in the app.

import type { InsightSalaryBand } from './api';
import { countryLabel, countrySlug } from './facets';
import { CATEGORY_VALUES } from './generated/contracts';
import { categoryLabel } from './labels';

/** A (category, country) pair is published only when it holds this many open
 *  postings. Above the /insights floor of 25: a two-axis page splits its evidence
 *  across more blocks than a one-axis page, so it needs more of it to fill them. */
export const MIN_PAIR_OPEN = 50;

/** A salary band under this sample size is withheld. A median over four postings and
 *  a median over three thousand render identically and are not the same claim. */
export const MIN_SALARY_SAMPLE = 10;

/** The english_level block renders only when the annotation covers this share of the
 *  pair. Below it the distribution describes which postings got annotated. */
export const MIN_ENGLISH_COVERAGE = 0.2;

/** How many neighbours each footer list links. A link block, not a directory. */
const NEIGHBOUR_LIMIT = 8;

/** The catch-all category names nothing anyone searches for, so it gets no page. */
const UNSEARCHABLE_CATEGORY = 'other';

// --- The two axes ------------------------------------------------------------

/** Vocabulary value → URL segment. Category values carry underscores and nothing
 *  else, so the mapping is a total, reversible character swap. */
export function categorySlug(category: string): string {
  return category.replaceAll('_', '-');
}

const CATEGORY_BY_SLUG = new Map(
  CATEGORY_VALUES.filter((c) => c !== UNSEARCHABLE_CATEGORY).map((c) => [categorySlug(c), c])
);

/** URL segment → vocabulary value, or undefined when the slug names no published
 *  category. Drives the route's 404. */
export function categoryFromSlug(slug: string): string | undefined {
  return CATEGORY_BY_SLUG.get(slug.toLowerCase());
}

export interface LandingCategory {
  category: string;
  slug: string;
  label: string;
}

/** Every category that gets pages, in vocabulary order. */
export function landingCategories(): LandingCategory[] {
  return [...CATEGORY_BY_SLUG.entries()].map(([slug, category]) => ({
    category,
    slug,
    label: categoryLabel(category),
  }));
}

// --- The publication gate ----------------------------------------------------

export interface PublishedCountry {
  code: string;
  slug: string;
  label: string;
  openCount: number;
}

/** The countries that earn a page under some category, read off that category's
 *  `countries` facet distribution. Two independent reasons to refuse: too few
 *  postings to fill a page, or no slug to address it by (the facet's key space is
 *  whatever the catalogue holds, which is not guaranteed to be ISO).
 *
 *  Evaluated against live counts on every render, so it self-heals in both
 *  directions — a country crossing the floor gains its page without a deploy. */
export function publishedCountries(countryCounts: Record<string, number>): PublishedCountry[] {
  const rows: PublishedCountry[] = [];
  for (const [code, openCount] of Object.entries(countryCounts)) {
    if (openCount < MIN_PAIR_OPEN) continue;
    const slug = countrySlug(code);
    if (!slug) continue;
    rows.push({ code, slug, label: countryLabel(code), openCount });
  }
  return rows.sort((a, b) => b.openCount - a.openCount);
}

/** Whether one pair clears the gate (drives the per-page 404). */
export function isPairPublished(countryCounts: Record<string, number>, code: string): boolean {
  return publishedCountries(countryCounts).some((c) => c.code === code);
}

// --- The honesty rules -------------------------------------------------------

/** The salary bands that may be shown, richest sample first. Bands below
 *  MIN_SALARY_SAMPLE are dropped rather than footnoted: a figure a reader has to
 *  discount is a figure that should not have been printed.
 *
 *  Currencies and periods are never merged — the endpoint returns one row per
 *  (currency, period) and each renders as its own line. */
export function publishableSalaryBands(bands: InsightSalaryBand[]): InsightSalaryBand[] {
  return bands
    .filter((b) => b.sample_size >= MIN_SALARY_SAMPLE)
    .sort((a, b) => b.sample_size - a.sample_size);
}

export interface EnglishRow {
  level: string;
  count: number;
  /** Share of the postings that DECLARE a level — not of the pair. */
  share: number;
}

/** The english-level breakdown, or null when too little of the pair is annotated to
 *  say anything about the market.
 *
 *  Measured: backend × Germany annotates 136 of 2041 postings. "C1 required in 90%"
 *  off that sample is a statement about which postings carry the annotation. The
 *  shares are of the declaring subset, and the caller labels them as such. */
export function englishBreakdown(
  levelCounts: Record<string, number>,
  total: number
): EnglishRow[] | null {
  const declared = Object.values(levelCounts).reduce((n, c) => n + c, 0);
  if (declared === 0) return null;
  if (total > 0 && declared / total < MIN_ENGLISH_COVERAGE) return null;
  return Object.entries(levelCounts)
    .map(([level, count]) => ({ level, count, share: count / declared }))
    .sort((a, b) => b.count - a.count);
}

/** The count of postings the reality signal calls fresh.
 *
 *  Deliberately the only number this module reads out of that facet. The same pair
 *  reads stale:1568 against fresh:470 — "77% stale" is true, and reads as a verdict
 *  on the catalogue rather than on the market, while carrying no extra signal for
 *  someone deciding where to apply. */
export function freshCount(realityCounts: Record<string, number>): number {
  return realityCounts.fresh ?? 0;
}

// --- The deterministic auto-intro (no LLM) -----------------------------------

export interface IntroInput {
  category: string;
  countryCode: string;
  total: number;
  fresh: number;
  topSkills: string[];
}

const count = (n: number) => n.toLocaleString('en-US');

/** The opening paragraph, composed from the retrieved figures. No model: a sentence
 *  that cannot be reproduced from the data is one nobody can check when it is wrong.
 *  Each clause is omitted rather than zero-filled, so the page never states an
 *  absence as a measurement. */
export function landingIntro({ category, countryCode, total, fresh, topSkills }: IntroInput): string {
  const what = categoryLabel(category);
  const where = countryLabel(countryCode);

  const sentences = [`There are ${count(total)} open ${what} jobs in ${where} on freehire right now.`];
  if (fresh > 0) {
    sentences.push(`${count(fresh)} of them were posted recently.`);
  }
  if (topSkills.length > 0) {
    const skills =
      topSkills.length === 1
        ? topSkills[0]
        : `${topSkills.slice(0, -1).join(', ')} and ${topSkills[topSkills.length - 1]}`;
    sentences.push(`The skills employers ask for most often are ${skills}.`);
  }
  return sentences.join(' ');
}

// --- Internal linking --------------------------------------------------------

/** The same category in other countries — the sibling links under a landing.
 *  Excludes the country being viewed, and everything the gate already refuses. */
export function neighbourCountries(
  countryCounts: Record<string, number>,
  currentCode: string
): PublishedCountry[] {
  return publishedCountries(countryCounts)
    .filter((c) => c.code !== currentCode)
    .slice(0, NEIGHBOUR_LIMIT);
}

export interface NeighbourCategory {
  category: string;
  slug: string;
  label: string;
  openCount: number;
}

/** Other categories in this country, read off the country-scoped `category` facet.
 *  Excludes the category being viewed and the catch-all. */
export function neighbourCategories(
  categoryCounts: Record<string, number>,
  currentCategory: string
): NeighbourCategory[] {
  const rows: NeighbourCategory[] = [];
  for (const [category, openCount] of Object.entries(categoryCounts)) {
    if (category === currentCategory) continue;
    if (!CATEGORY_BY_SLUG.has(categorySlug(category))) continue;
    if (openCount < MIN_PAIR_OPEN) continue;
    rows.push({ category, slug: categorySlug(category), label: categoryLabel(category), openCount });
  }
  return rows.sort((a, b) => b.openCount - a.openCount).slice(0, NEIGHBOUR_LIMIT);
}

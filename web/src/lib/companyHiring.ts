// Pure helpers behind the company page's Hiring tab. The numbers come from two places —
// the hiring endpoint (series, growth, lifetime) and the ordinary facet-count call scoped
// to the company (who they hire, what they pay) — and this module only shapes them.

import { dynamicLabel, FACETS } from './facets';
import type { CompanyHiring, CompanyResponse, FacetCounts } from './types';

/** Fewest postings stating a salary before the tab shows a pay range. Below it the range
 *  is whatever two postings happened to say, not how the company pays. */
const SALARY_SAMPLE_GATE = 3;

/** The n most frequent values of one facet distribution, most common first. Ties break
 *  by value so the order is stable between renders. Zero counts are not "values". */
export function topEntries(
  dist: Record<string, number> | undefined,
  n: number,
): { value: string; count: number }[] {
  return Object.entries(dist ?? {})
    .filter(([, count]) => count > 0)
    .sort(([a, x], [b, y]) => y - x || a.localeCompare(b))
    .slice(0, n)
    .map(([value, count]) => ({ value, count }));
}

/** The currency most of the company's postings are paid in. A range is only meaningful
 *  in one currency: across all of them, measured on prod, one employer spanned 60 to
 *  7,000,000 — hourly rupees beside yearly dollars. */
export function dominantCurrency(dist: Record<string, number> | undefined): string | null {
  return topEntries(dist, 1)[0]?.value ?? null;
}

/** The pay range across `sample` postings already narrowed to one currency and period:
 *  the lowest stated floor to the highest stated ceiling. A posting that states only one
 *  bound still counts, so either side falls back to the other. */
export function salaryRange(
  stats: FacetCounts['stats'],
  sample: number,
): { min: number; max: number } | null {
  if (sample < SALARY_SAMPLE_GATE) return null;
  const floor = stats.salary_min ?? stats.salary_max;
  const ceiling = stats.salary_max ?? stats.salary_min;
  if (!floor || !ceiling) return null;
  return { min: floor.min, max: ceiling.max };
}

/** A signed change in open postings. A true minus sign, not a hyphen. */
export function formatGrowth(growth: number): string {
  if (growth > 0) return `+${growth}`;
  if (growth < 0) return `−${-growth}`;
  return 'no change';
}

/** A facet value's display label, resolved exactly as the Jobs tab's filter panel
 *  resolves it — the static option list first, then the dynamic resolver (countries,
 *  skills) — so one value is never spelled two ways across the two tabs. */
export function breakdownLabel(param: string, value: string): string {
  const option = FACETS.find((f) => f.param === param)?.options?.find((o) => o.value === value);
  return option?.label ?? dynamicLabel(param, value);
}

/** `noindex, follow` for a company with nothing open and no movement in the window: its
 *  Hiring page is a heading over an empty chart. A failed read (null) is not an empty
 *  company and must not be spelled like one — the same rule `listingRobots` keeps. */
export function hiringRobots(hiring: CompanyHiring | null): string | undefined {
  if (!hiring || hiring.open > 0) return undefined;
  const moved = hiring.series.some((p) => p.added > 0 || p.removed > 0);
  return moved ? undefined : 'noindex, follow';
}

/** The share of observable applications that were answered, as a whole percent. */
export function replyRate(response: CompanyResponse): number {
  return Math.round((response.answered / response.applications) * 100);
}

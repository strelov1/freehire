// Pure helpers behind /salaries: the page's address (a role × grade × country scope
// plus the lines it is compared with), the distribution histogram drawn from the
// rollup's quantiles, and the geometry of the comparison chart. Kept free of fetch and
// Svelte so each is unit-tested; the route loads the data and feeds these.
import type { InsightSalaryBand, InsightSalaryMonth } from './api';
import type { SalaryScope } from './insights';
import { historySeries, nextMonth } from './salaryHistoryChart';

/** One comparison line: a role and a country, the grade shared with the main scope. */
interface SalaryLine {
  category: string;
  country: string;
}

export interface SalariesQuery {
  scope: SalaryScope;
  /** Lines compared with the scope, the scope itself not included. */
  compare: SalaryLine[];
  /** The currency and period figures are shown in, "USD-year"; '' = the richest band's. */
  unit: string;
}

/** At most this many lines beside the scope: five lines is where a chart stops reading. */
export const MAX_COMPARE = 4;

/** Read the page's address. `vs` lists comparison lines as `role.country`, either part
 *  empty for "every value" (`frontend.de`, `.pl`), comma-separated. A line repeated, or
 *  equal to the scope's own, is dropped: every line is keyed by what it names, and two
 *  lines naming the same thing would draw one over the other. */
export function readSalariesQuery(p: URLSearchParams): SalariesQuery {
  const get = (k: string) => (p.get(k) ?? '').trim().toLowerCase();
  const scope = { category: get('category'), seniority: get('seniority'), country: get('country') };
  const seen = new Set([`${scope.category}.${scope.country}`]);
  const compare: SalaryLine[] = [];
  for (const raw of (p.get('vs') ?? '').split(',')) {
    const text = raw.trim().toLowerCase();
    if (!text) continue;
    // Keyed by the parsed pair, not the text: `frontend.de.x` names the same line.
    const [category = '', country = ''] = text.split('.');
    const key = `${category}.${country}`;
    if (seen.has(key) || compare.length === MAX_COMPARE) continue;
    seen.add(key);
    compare.push({ category, country });
  }
  return { scope, compare, unit: (p.get('unit') ?? '').trim() };
}

/** The page's query string for a query ('' or '?…'), omitting every empty part; the
 *  route prefixes it with resolve('/salaries'). */
export function salariesSearch(q: SalariesQuery): string {
  const p = new URLSearchParams();
  if (q.scope.category) p.set('category', q.scope.category);
  if (q.scope.seniority) p.set('seniority', q.scope.seniority);
  if (q.scope.country) p.set('country', q.scope.country);
  if (q.compare.length) p.set('vs', q.compare.map((l) => `${l.category}.${l.country}`).join(','));
  if (q.unit) p.set('unit', q.unit);
  const s = p.toString();
  return s ? `?${s}` : '';
}

export const unitOf = (b: { currency: string; period: string }) => `${b.currency}-${b.period}`;

export interface HistogramBin {
  lo: number;
  hi: number;
  /** Share of ALL postings in the band (the bins cover its 5th–95th percentile, 0.9). */
  share: number;
}

/** The step nearest `raw` from 1, 2, 2.5 and 5 times a power of ten. */
function niceStep(raw: number): number {
  const pow = 10 ** Math.floor(Math.log10(raw));
  const f = raw / pow;
  for (const m of [1, 2, 2.5, 5]) if (f <= m) return m * pow;
  return 10 * pow;
}

/** A histogram of a band's middle 90% from its 19 quantiles (5th, 10th … 95th
 *  percentile), with round bin edges. Between two known quantiles the postings are
 *  spread evenly — an estimate, exact at every quantile, that never invents mass
 *  outside the measured range. [] without a full, non-flat quantile set — including
 *  none at all, from an API older than the field. */
export function histogramFromQuantiles(q: readonly number[] | undefined, target = 10): HistogramBin[] {
  if (!q) return [];
  const first = q[0];
  const last = q[q.length - 1];
  if (q.length !== 19 || first === undefined || last === undefined || last <= first) return [];
  // The share of postings at or below x, clamped to the 5–95% the quantiles cover.
  const cdf = (x: number): number => {
    if (x <= first) return 0.05;
    if (x >= last) return 0.95;
    let i = 0;
    while (i < 17 && (q[i + 1] ?? last) <= x) i++;
    const a = q[i] ?? first;
    const b = q[i + 1] ?? last;
    return 0.05 * (i + 1) + 0.05 * ((x - a) / (b - a));
  };
  const step = niceStep((last - first) / target);
  const bins: HistogramBin[] = [];
  for (let lo = Math.floor(first / step) * step; lo < last; lo += step) {
    bins.push({ lo, hi: lo + step, share: cdf(lo + step) - cdf(lo) });
  }
  return bins;
}

/** Every unit the page can show, richest first: the scope's bands, then any unit a
 *  line has history in, so a unit only the comparison carries can still be picked. */
export function pageUnits(bands: InsightSalaryBand[], histories: (InsightSalaryMonth[] | null)[]): string[] {
  const units = [...bands.map(unitOf), ...histories.flatMap((rows) => historySeries(rows ?? []).map(unitOf))];
  return [...new Set(units)];
}

/** The unit every figure on the page is in, the cards and the chart alike: the
 *  requested one when the page can show it, else the richest (the scope's first band,
 *  else the first unit a line has history in). '' when nothing has figures. */
export function pageUnit(
  bands: InsightSalaryBand[],
  requested: string,
  histories: (InsightSalaryMonth[] | null)[],
): string {
  const units = pageUnits(bands, histories);
  return units.includes(requested) ? requested : (units[0] ?? '');
}

/** One line's stored months in one unit, oldest first; [] when it has too few months. */
export function lineMonths(rows: InsightSalaryMonth[] | null, unit: string): InsightSalaryMonth[] {
  return historySeries(rows ?? []).find((s) => unitOf(s) === unit)?.months ?? [];
}

interface ComparePoint {
  x: number;
  y: number;
  month: string;
  row: InsightSalaryMonth;
  partial: boolean;
}

export interface CompareModel {
  width: number;
  height: number;
  topY: number;
  baselineY: number;
  slots: string[];
  slotX: number[];
  /** Per line, in the order given: its points and its runs of consecutive months. */
  lines: { points: ComparePoint[]; segments: ComparePoint[][] }[];
  min: number;
  max: number;
}

const WIDTH = 960;
const PLOT_H = 220;
const PAD = 28;

/** Lays several lines' median salaries over one month axis and one y-scale. A month a
 *  line did not store breaks that line, as on the single-line chart. null when no line
 *  has a month. */
export function buildCompareChart(lines: InsightSalaryMonth[][], currentMonth: string): CompareModel | null {
  const all = lines.flat();
  if (all.length === 0) return null;
  const months = all.map((r) => r.month).sort();
  const first = months[0] ?? currentMonth;
  const last = months[months.length - 1] ?? currentMonth;
  const slots = [first];
  while (slots[slots.length - 1] !== last && slots.length < 120) slots.push(nextMonth(slots[slots.length - 1] ?? last));

  const min = Math.min(...all.map((r) => r.p50));
  const max = Math.max(...all.map((r) => r.p50));
  const range = max - min;
  const baselineY = PAD + PLOT_H;
  const y = (v: number) => (range === 0 ? baselineY - PLOT_H / 2 : baselineY - ((v - min) / range) * PLOT_H);
  const stepX = slots.length > 1 ? (WIDTH - PAD * 2) / (slots.length - 1) : 0;
  const xOf = (m: string) => (slots.length > 1 ? PAD + slots.indexOf(m) * stepX : WIDTH / 2);

  return {
    width: WIDTH,
    height: baselineY + PAD,
    topY: PAD,
    baselineY,
    slots,
    slotX: slots.map(xOf),
    min,
    max,
    lines: lines.map((rows) => {
      const points = rows.map((row) => ({
        x: xOf(row.month),
        y: y(row.p50),
        month: row.month,
        row,
        partial: row.month === currentMonth,
      }));
      const segments: ComparePoint[][] = [];
      for (const p of points) {
        const run = segments[segments.length - 1];
        const prev = run?.[run.length - 1];
        if (run && prev && nextMonth(prev.month) === p.month) run.push(p);
        else segments.push([p]);
      }
      return { points, segments };
    }),
  };
}

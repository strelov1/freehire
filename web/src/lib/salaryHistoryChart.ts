// Pure geometry for the salary-history chart on /insights/salary/<category>: a median
// line and a p25–p75 band per month, for one (currency, period). Months are SLOTS on the
// x-axis, so a month the rollup did not store (under its sample floor) keeps its width
// and breaks the line — joining the months either side of it would draw a trend through
// a month nobody measured.
import type { InsightSalaryMonth } from './api';

/** A series needs this many stored months before it is drawn at all. */
export const MIN_HISTORY_POINTS = 3;

/** A month whose sample is at least this multiple of the month before it is flagged:
 *  the catalogue grows in jumps as boards are onboarded, and a jump that size changes
 *  WHICH postings are measured — measured on prod, backend's EUR/year median fell from
 *  €62,500 to €45,000 the month its sample went from 104 to 548. The change may be pay;
 *  it may be the mix. The chart says which months that applies to. */
export const MIX_SHIFT_RATIO = 3;

interface HistorySeries {
  currency: string;
  period: string;
  /** Stored months, oldest first. */
  months: InsightSalaryMonth[];
  /** Postings over all months — what ranks the series. */
  total: number;
}

/** Groups the endpoint's rows into one series per (currency, period), keeping only those
 *  with MIN_HISTORY_POINTS months, largest total sample first — the first is the chart's
 *  default. Currencies and periods are never combined, as the bands never combine them. */
export function historySeries(rows: InsightSalaryMonth[]): HistorySeries[] {
  const byKey = new Map<string, HistorySeries>();
  for (const r of rows) {
    const key = `${r.currency}/${r.period}`;
    let s = byKey.get(key);
    if (!s) byKey.set(key, (s = { currency: r.currency, period: r.period, months: [], total: 0 }));
    s.months.push(r);
    s.total += r.sample_size;
  }
  return [...byKey.values()]
    .filter((s) => s.months.length >= MIN_HISTORY_POINTS)
    .map((s) => ({ ...s, months: [...s.months].sort((a, b) => a.month.localeCompare(b.month)) }))
    .sort((a, b) => b.total - a.total || `${a.currency}${a.period}`.localeCompare(`${b.currency}${b.period}`));
}

interface ChartPoint {
  x: number;
  yMid: number;
  yLow: number;
  yHigh: number;
  month: string;
  row: InsightSalaryMonth;
  /** The current month: still filling up, drawn hollow. */
  partial: boolean;
  /** Its sample is MIX_SHIFT_RATIO times the previous month's: the step into it is
   *  drawn dashed. Only against an adjacent stored month — after a gap there is no
   *  neighbour to compare with. */
  mixShift: boolean;
}

interface SalaryChartModel {
  width: number;
  height: number;
  topY: number;
  baselineY: number;
  /** Every month from the first stored to the last, stored or not. */
  slots: string[];
  /** The x of each slot, for the month labels. */
  slotX: number[];
  points: ChartPoint[];
  /** Runs of points in consecutive months — each is drawn as its own line and band. */
  segments: ChartPoint[][];
  /** The y-axis range: lowest p25 and highest p75 in the series. */
  min: number;
  max: number;
}

const WIDTH = 960;
const PLOT_H = 220;
const PAD = 28;

/** "2026-03" → the next month's key, by calendar arithmetic on the key itself. */
function nextMonth(key: string): string {
  const [y, m] = key.split('-').map(Number) as [number, number];
  return m === 12 ? `${y + 1}-01` : `${y}-${String(m + 1).padStart(2, '0')}`;
}

/** Lays one series out. `currentMonth` (YYYY-MM, UTC) marks the month still in progress. */
export function buildSalaryChart(series: HistorySeries, currentMonth: string): SalaryChartModel {
  const baselineY = PAD + PLOT_H;
  const months = series.months;
  const first = months[0]?.month ?? currentMonth;
  const last = months[months.length - 1]?.month ?? currentMonth;

  const slots = [first];
  while (slots[slots.length - 1] !== last && slots.length < 120) slots.push(nextMonth(slots[slots.length - 1] ?? last));

  const min = Math.min(...months.map((m) => m.p25));
  const max = Math.max(...months.map((m) => m.p75));
  const range = max - min;
  // A flat series sits on the midline rather than the baseline, as skillDetailChart's does.
  const y = (v: number) => (range === 0 ? baselineY - PLOT_H / 2 : baselineY - ((v - min) / range) * PLOT_H);
  const stepX = slots.length > 1 ? (WIDTH - PAD * 2) / (slots.length - 1) : 0;
  const xOf = (month: string) => (slots.length > 1 ? PAD + slots.indexOf(month) * stepX : WIDTH / 2);

  const points = months.map((row, i) => {
    const prev = months[i - 1];
    const adjacent = prev !== undefined && nextMonth(prev.month) === row.month;
    return {
      x: xOf(row.month),
      yMid: y(row.p50),
      yLow: y(row.p25),
      yHigh: y(row.p75),
      month: row.month,
      row,
      partial: row.month === currentMonth,
      mixShift: adjacent && row.sample_size >= MIX_SHIFT_RATIO * prev.sample_size,
    };
  });

  const segments: ChartPoint[][] = [];
  for (const p of points) {
    const run = segments[segments.length - 1];
    const prev = run?.[run.length - 1];
    if (run && prev && nextMonth(prev.month) === p.month) run.push(p);
    else segments.push([p]);
  }

  const slotX = slots.map(xOf);
  return { width: WIDTH, height: baselineY + PAD, topY: PAD, baselineY, slots, slotX, points, segments, min, max };
}

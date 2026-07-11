// Pure geometry for the job-activity bar chart: turn a dense series of
// added/removed counts into positioned, scaled bars in viewBox units. Kept out of
// the Svelte component so the scaling math — the bug-prone part (max, zero rows,
// empty series) — is unit-testable without rendering. ActivityBars.svelte is then
// a dumb renderer of this model.

import type { ActivityPoint } from './types';

/** One period's pair of bars, in viewBox units. `x` is the left edge of each bar;
 *  heights grow up from the baseline (a zero count is a zero-height bar). */
export interface ActivityBar {
  period: string;
  added: number;
  removed: number;
  addedX: number;
  addedY: number;
  addedH: number;
  removedX: number;
  removedY: number;
  removedH: number;
  /** Centre x of the period slot (the seam between the two bars) — anchors the
   *  x-axis date tick and the hover highlight. */
  centerX: number;
}

/** The full chart model: the positioned bars plus the viewBox and baseline the
 *  component needs to draw axes and set the SVG size. */
export interface ActivityChartModel {
  bars: ActivityBar[];
  width: number;
  height: number;
  baselineY: number;
  /** The count the tallest bar represents (always ≥ 1 so scaling never divides by
   *  zero); labels the y-axis max. */
  max: number;
  barW: number;
  /** Width of one period slot — the hover highlight spans it. */
  slot: number;
}

const WIDTH = 960;
const PLOT_H = 240;
const PAD = 16;
/** Fraction of a period slot each of the two bars occupies (the rest is the gap
 *  between the pair and its neighbours). */
const BAR_FRACTION = 0.34;

/** Build the bar model for `points`. An empty series yields an empty bar list but
 *  a valid (drawable) frame. Heights are scaled to the largest single count across
 *  both series, so added and removed share one axis and stay comparable. */
export function buildActivityChart(points: ActivityPoint[]): ActivityChartModel {
  const baselineY = PAD + PLOT_H;
  const frame: Omit<ActivityChartModel, 'bars'> = {
    width: WIDTH,
    height: baselineY + PAD,
    baselineY,
    max: 1,
    barW: 0,
    slot: 0,
  };
  if (points.length === 0) {
    return { ...frame, bars: [] };
  }

  const max = Math.max(1, ...points.flatMap((p) => [p.added, p.removed]));
  const slot = (WIDTH - PAD * 2) / points.length;
  const barW = slot * BAR_FRACTION;

  const bars = points.map((p, i): ActivityBar => {
    const slotX = PAD + i * slot;
    const centerX = slotX + slot / 2;
    const addedX = centerX - barW; // left of centre
    const removedX = centerX; // right of centre
    const addedH = (p.added / max) * PLOT_H;
    const removedH = (p.removed / max) * PLOT_H;
    return {
      period: p.period,
      added: p.added,
      removed: p.removed,
      addedX,
      addedY: baselineY - addedH,
      addedH,
      removedX,
      removedY: baselineY - removedH,
      removedH,
      centerX,
    };
  });

  return { ...frame, bars, max, barW, slot };
}

/** Compact count formatting for axis/summary labels: 3354251 → "3.4M",
 *  697191 → "697K", 842 → "842". Full precision is left to the tooltip. */
export function formatCount(n: number): string {
  const abs = Math.abs(n);
  if (abs >= 1e6) return trimZero((n / 1e6).toFixed(1)) + 'M';
  if (abs >= 1e3) return trimZero((n / 1e3).toFixed(abs >= 1e5 ? 0 : 1)) + 'K';
  return String(n);
}

function trimZero(s: string): string {
  return s.replace(/\.0$/, '');
}

/** Choose which period indices get an x-axis date label. Few periods → label all;
 *  a long series is thinned to at most MAX_TICKS evenly-spaced labels, always
 *  including the first and last so the time span reads correctly. */
export function pickTickIndices(count: number): number[] {
  const MAX_TICKS = 12;
  if (count <= 0) return [];
  if (count <= MAX_TICKS) return Array.from({ length: count }, (_, i) => i);
  const step = (count - 1) / (MAX_TICKS - 1);
  const seen = new Set<number>();
  for (let i = 0; i < MAX_TICKS; i++) seen.add(Math.round(i * step));
  return [...seen].sort((a, b) => a - b);
}

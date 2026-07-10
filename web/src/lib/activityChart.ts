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
}

/** The full chart model: the positioned bars plus the viewBox and baseline the
 *  component needs to draw axes and set the SVG size. */
export interface ActivityChartModel {
  bars: ActivityBar[];
  width: number;
  height: number;
  baselineY: number;
  /** The count the tallest bar represents (always ≥ 1 so scaling never divides by
   *  zero); handy for a y-axis max label. */
  max: number;
  barW: number;
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
  };
  if (points.length === 0) {
    return { ...frame, bars: [] };
  }

  const max = Math.max(1, ...points.flatMap((p) => [p.added, p.removed]));
  const slot = (WIDTH - PAD * 2) / points.length;
  const barW = slot * BAR_FRACTION;

  const bars = points.map((p, i): ActivityBar => {
    const slotX = PAD + i * slot;
    const addedX = slotX + slot / 2 - barW; // left of centre
    const removedX = slotX + slot / 2; // right of centre
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
    };
  });

  return { ...frame, bars, max, barW };
}

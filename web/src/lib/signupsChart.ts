// Pure geometry for the new-members-per-day bar chart: turn the dense
// UserGrowthPoint series into positioned, scaled single-series bars in viewBox
// units. Same shape and constants as activityChart.ts's two-series model, kept as
// a separate function because this chart has one bar per day (the day's own
// `new` count) rather than an added/removed pair.

import type { UserGrowthPoint } from './types';

/** One day's bar, in viewBox units. `x` is the bar's left edge; height grows up
 *  from the baseline (a zero-signup day is a zero-height bar). */
export interface SignupsBar {
  date: string;
  new: number;
  x: number;
  y: number;
  h: number;
  /** Centre x of the day's slot — anchors the x-axis date tick and the hover
   *  highlight. */
  centerX: number;
}

/** The full chart model: the positioned bars plus the viewBox and baseline the
 *  component needs to draw axes and set the SVG size. */
export interface SignupsChartModel {
  bars: SignupsBar[];
  width: number;
  height: number;
  baselineY: number;
  /** The count the tallest bar represents (always ≥ 1 so scaling never divides by
   *  zero); labels the y-axis max. */
  max: number;
  barW: number;
  /** Width of one day's slot — the hover highlight spans it. */
  slot: number;
  /** Left/right margin reserved around the plot area — where the y-axis
   *  reference line and hover math anchor. */
  pad: number;
}

const WIDTH = 960;
const PLOT_H = 240;
const PAD = 16;
/** Fraction of a day's slot the bar occupies (the rest is the gap between bars).
 *  Wider than activityChart.ts's 0.34: that chart fits two bars per slot, so each
 *  stays narrow; this one draws a single bar per slot and can afford more of it. */
const BAR_FRACTION = 0.6;

/** Build the bar model for `points`. An empty series yields an empty bar list but
 *  a valid (drawable) frame. Heights are scaled to the largest single day's
 *  `new` count. */
export function buildSignupsChart(points: UserGrowthPoint[]): SignupsChartModel {
  const baselineY = PAD + PLOT_H;
  const frame: Omit<SignupsChartModel, 'bars'> = {
    width: WIDTH,
    height: baselineY + PAD,
    baselineY,
    max: 1,
    barW: 0,
    slot: 0,
    pad: PAD,
  };
  if (points.length === 0) {
    return { ...frame, bars: [] };
  }

  const max = Math.max(1, ...points.map((p) => p.new));
  const slot = (WIDTH - PAD * 2) / points.length;
  const barW = slot * BAR_FRACTION;

  const bars = points.map((p, i): SignupsBar => {
    const slotX = PAD + i * slot;
    const centerX = slotX + slot / 2;
    const h = (p.new / max) * PLOT_H;
    return {
      date: p.date,
      new: p.new,
      x: centerX - barW / 2,
      y: baselineY - h,
      h,
      centerX,
    };
  });

  return { ...frame, bars, max, barW, slot };
}

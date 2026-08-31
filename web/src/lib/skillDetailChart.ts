// Pure geometry for the per-skill detail page's larger line chart. Same
// scaling rules as skillPulseSparkline.ts (never divide by zero, a flat
// series sits on the midline rather than the baseline) but at a size that
// carries axis labels and a hover tooltip, and retaining each point's own
// week/count for that tooltip — the compact sparkline throws those away.

import type { SkillPulsePoint } from './api';
import { must } from './utils';

interface DetailChartPoint {
  x: number;
  y: number;
  weekStart: string;
  openCount: number;
}

export interface SkillDetailChartModel {
  points: DetailChartPoint[];
  width: number;
  height: number;
  /** Plot-area top/bottom in viewBox units — the caller draws the y-axis max
   *  label at `topY` and the baseline at `baselineY`. */
  topY: number;
  baselineY: number;
  /** The largest count in the series (never zero), for the y-axis max label. */
  max: number;
}

const WIDTH = 960;
const PLOT_H = 240;
const PAD = 24;

/** Build the detail-chart model for one skill's series. An empty series draws
 *  nothing; a single-point series places one point at the frame's centre; a
 *  flat series (min === max) sits every point on the plot's vertical midline
 *  rather than the baseline (see skillPulseSparkline.ts for why). */
export function buildSkillDetailChart(series: SkillPulsePoint[]): SkillDetailChartModel {
  const baselineY = PAD + PLOT_H;
  const frame = { width: WIDTH, height: baselineY + PAD, topY: PAD, baselineY, max: 1 };
  if (series.length === 0) {
    return { ...frame, points: [] };
  }

  const counts = series.map((p) => p.open_count);
  const max = Math.max(1, ...counts);
  const min = Math.min(...counts);
  const range = max - min;

  if (series.length === 1) {
    const only = must(series[0]);
    return {
      ...frame,
      max,
      points: [{ x: WIDTH / 2, y: baselineY - PLOT_H / 2, weekStart: only.week_start, openCount: only.open_count }],
    };
  }

  const stepX = (WIDTH - PAD * 2) / (series.length - 1);
  const points = series.map((p, i) => ({
    x: PAD + i * stepX,
    y: range === 0 ? baselineY - PLOT_H / 2 : baselineY - ((p.open_count - min) / range) * PLOT_H,
    weekStart: p.week_start,
    openCount: p.open_count,
  }));

  return { width: WIDTH, height: baselineY + PAD, topY: PAD, baselineY, max, points };
}

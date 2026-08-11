// Pure geometry for the per-skill demand sparkline on /my/market-pulse: turn a
// weekly open_count series into a scaled polyline in a small fixed viewBox. Kept
// out of the Svelte component so the scaling math (empty/single-point series, a
// flat series never dividing by zero) is unit-testable without rendering —
// mirrors activityChart.ts for ActivityBars.svelte.

import type { SkillPulsePoint } from './api';

export interface SparklineModel {
  width: number;
  height: number;
  /** SVG `points` attribute for a `<polyline>`; empty when there is nothing to
   *  connect (zero or one snapshot). */
  points: string;
  /** Coordinates of the most recent snapshot — the accent dot always has
   *  somewhere to sit, even with a single point. */
  lastX: number;
  lastY: number;
}

const WIDTH = 120;
const HEIGHT = 32;
const PAD_Y = 3;

/** Build the sparkline model for one skill's series. An empty series draws
 *  nothing (the dot sits at the frame's centre-right, never reached because the
 *  caller only renders a card once at least one snapshot exists); a single-point
 *  series draws only the accent dot; a flat series (min === max) still fills the
 *  frame height's midline rather than dividing by zero. */
export function buildSparkline(series: SkillPulsePoint[]): SparklineModel {
  if (series.length === 0) {
    return { width: WIDTH, height: HEIGHT, points: '', lastX: WIDTH, lastY: HEIGHT / 2 };
  }
  if (series.length === 1) {
    return { width: WIDTH, height: HEIGHT, points: '', lastX: WIDTH - 1, lastY: HEIGHT / 2 };
  }

  const counts = series.map((p) => p.open_count);
  const min = Math.min(...counts);
  const max = Math.max(...counts);
  const range = max - min;
  const stepX = WIDTH / (series.length - 1);

  // A flat series (range === 0) sits on the frame's vertical midline for every
  // point — pinning it to the baseline instead would read as "near zero" even
  // when the level is high, misrepresenting "unchanged" as "low".
  const coords = series.map((p, i) => ({
    x: i * stepX,
    y: range === 0 ? HEIGHT / 2 : HEIGHT - PAD_Y - ((p.open_count - min) / range) * (HEIGHT - PAD_Y * 2),
  }));

  // .at(-1) is never undefined here — the `series.length === 1` branch above
  // already returned, so coords has at least two entries.
  const last = coords.at(-1) ?? { x: WIDTH, y: HEIGHT / 2 };
  return {
    width: WIDTH,
    height: HEIGHT,
    points: coords.map((c) => `${c.x.toFixed(1)},${c.y.toFixed(1)}`).join(' '),
    lastX: last.x,
    lastY: last.y,
  };
}

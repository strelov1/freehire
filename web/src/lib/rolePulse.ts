// Pure helpers behind the market pulse on /insights/roles/<category>. The endpoint
// serves raw figures for the last 30 days and the 30 before; this module turns them
// into the three figures the page shows and decides which are too thin to show at all.
// The floors live here and only here — the endpoint deliberately applies none.

import type { InsightRolePulse } from './api';

/** Fewest posting-days of exposure, and fewest views, before views-per-posting means
 *  anything: below them the ratio is a handful of visits divided by a handful of days. */
const MIN_EXPOSURE_DAYS = 300;
const MIN_VIEWS = 50;
/** Fewest qualifying closes before a median close time is published. */
const MIN_CLOSES = 30;
/** Fewest postings in the earlier window before inflow gets a direction. */
const MIN_INFLOW = 30;
/** A change smaller than this, either way, reads as "about the same". */
const FLAT_BAND = 0.05;

type Direction = 'up' | 'down' | 'flat';

interface PulseFigure {
  key: 'competition' | 'time_to_close' | 'inflow';
  label: string;
  /** The current window's figure, or null when it is below the floor. */
  value: string | null;
  /** What the figure counts, in one line. */
  detail: string;
  /** The earlier window's figure, present only when BOTH windows clear the floor. */
  previous: string | null;
  direction: Direction | null;
}

function directionOf(current: number, previous: number): Direction {
  if (previous === 0) return current > 0 ? 'up' : 'flat';
  const change = (current - previous) / previous;
  if (Math.abs(change) < FLAT_BAND) return 'flat';
  return change > 0 ? 'up' : 'down';
}

/** Views per open posting per 30 days, or null below the floor. */
function viewsPerPosting(views: number, exposureDays: number): number | null {
  if (exposureDays < MIN_EXPOSURE_DAYS || views < MIN_VIEWS) return null;
  return (views * 30) / exposureDays;
}

const fmtCount = (n: number) => n.toLocaleString('en-US');
const fmtRatio = (n: number) => n.toLocaleString('en-US', { maximumFractionDigits: 1 });
const fmtDays = (n: number) => `${Math.round(n)} days`;

/** The three figures, always in the same order: competition, time to close, inflow. */
export function rolePulseFigures(p: InsightRolePulse): PulseFigure[] {
  const c = p.competition;
  const comp = viewsPerPosting(c.views, c.exposure_days);
  const compPrev = viewsPerPosting(c.views_prev, c.exposure_days_prev);

  const t = p.time_to_close;
  const close = t.closed >= MIN_CLOSES ? t.median_days : null;
  const closePrev = t.closed_prev >= MIN_CLOSES ? t.median_days_prev : null;

  const i = p.inflow;
  const inflowComparable = i.count_prev >= MIN_INFLOW;

  return [
    {
      key: 'competition',
      label: 'Competition',
      value: comp === null ? null : fmtRatio(comp),
      detail: 'page views per open posting a month — one visitor counts once per posting per day',
      previous: comp !== null && compPrev !== null ? fmtRatio(compPrev) : null,
      direction: comp !== null && compPrev !== null ? directionOf(comp, compPrev) : null,
    },
    {
      key: 'time_to_close',
      label: 'Time to close',
      value: close === null ? null : fmtDays(close),
      detail:
        close === null
          ? 'median time a posting stays open'
          : `median time a posting stays open, over ${fmtCount(t.closed)} closed in the last 30 days`,
      previous: close !== null && closePrev !== null ? fmtDays(closePrev) : null,
      direction: close !== null && closePrev !== null ? directionOf(close, closePrev) : null,
    },
    {
      key: 'inflow',
      label: 'New postings',
      value: fmtCount(i.count),
      detail: 'published in the last 30 days',
      previous: inflowComparable ? fmtCount(i.count_prev) : null,
      direction: inflowComparable ? directionOf(i.count, i.count_prev) : null,
    },
  ];
}

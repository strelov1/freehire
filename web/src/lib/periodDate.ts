// Display formatting for PeriodDate (see types.ts) — the frontend counterpart to
// internal/candidate/perioddate.PeriodDate's Format/FormatRange. There is no Parse here:
// the backend is the only place free text is ever read (the one-off backfill and the
// jsonb self-healing decode), and the frontend only ever produces a PeriodDate through
// PeriodDateInput.svelte's own pick-or-type controls.

import type { PeriodDate } from './types';

const MONTH_NAMES = [
  'Jan',
  'Feb',
  'Mar',
  'Apr',
  'May',
  'Jun',
  'Jul',
  'Aug',
  'Sep',
  'Oct',
  'Nov',
  'Dec',
];

/** Renders a single boundary, e.g. "Mar 2021" or "2018" for a year-only date. */
export function formatPeriodDate(d: PeriodDate | undefined): string {
  if (!d || d.year <= 0) return '';
  if (d.month === undefined) return String(d.year);
  return `${MONTH_NAMES[d.month - 1]} ${d.year}`;
}

/** Joins a start/end pair the way every display line in this app does: " – " between two
 *  present sides, just the one side alone, or "" when neither is set. `current` renders
 *  the end side as "Present" instead of leaving it blank. */
export function formatPeriodRange(
  start: PeriodDate | undefined,
  end: PeriodDate | undefined,
  current?: boolean,
): string {
  const a = formatPeriodDate(start);
  const b = current ? 'Present' : formatPeriodDate(end);
  if (a && b) return `${a} – ${b}`;
  return a || b;
}

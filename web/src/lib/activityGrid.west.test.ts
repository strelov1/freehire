// The mirror of activityGrid.test.ts, run west of UTC.
//
// It is a separate file because TZ is a per-process setting and vitest gives each file its
// own worker — the same reason calendarModel.west.test.ts exists, and the same trap. One zone
// is not a test of timezone handling: a grid built from UTC accessors passes every eastern
// assertion that a correct one does, for the opposite instant, and the failure only shows up
// on the other side of Greenwich.
//
// Here it matters more than on the calendar. There a misfiled event lands on the wrong square;
// here it can break a streak that is running or invent one that is not.
process.env.TZ = 'America/Los_Angeles'; // UTC-7 in summer

import { describe, expect, it } from 'vitest';

import { buildActivityGrid, rangeForWindow, WINDOW_DAYS } from './activityGrid';
import type { TimelineEvent } from './types';

const event = (occurredAt: string, id = 1): TimelineEvent =>
  ({ id, kind: 'applied', source: 'user', observed: true, occurred_at: occurredAt, company_slug: 'acme' }) as TimelineEvent;

const TODAY = new Date(2026, 8, 16); // 16 September 2026, local

describe('buildActivityGrid west of UTC', () => {
  it('files an early-morning UTC instant on the previous local day', () => {
    // 05:00 UTC on 15 September is 22:00 on 14 September in Los Angeles. Reading the UTC
    // date would file it a day late — the mirror of the eastern case, and invisible without
    // this file.
    const grid = buildActivityGrid([event('2026-09-15T05:00:00Z')], TODAY);

    expect(grid.days.find((d) => d.key === '2026-09-14')?.count).toBe(1);
    expect(grid.days.find((d) => d.key === '2026-09-15')?.count).toBe(0);
  });

  // The streak is the reason the day boundary matters here more than anywhere else. These
  // three instants are three consecutive local days ending today in Los Angeles; read as UTC
  // dates they are 15, 16 and 17 September, which puts one of them in the FUTURE — outside
  // the window entirely — and reports a streak of one.
  it('does not break a live streak on the day boundary', () => {
    const grid = buildActivityGrid(
      [event('2026-09-15T05:00:00Z', 1), event('2026-09-16T05:00:00Z', 2), event('2026-09-17T05:00:00Z', 3)],
      TODAY,
    );

    expect(grid.currentStreak).toBe(3);
    expect(grid.total).toBe(3);
  });

  it('still ends on the reader own today and spans the window', () => {
    const grid = buildActivityGrid([], TODAY);
    const inWindow = grid.days.filter((d) => d.inWindow);

    expect(inWindow).toHaveLength(WINDOW_DAYS);
    expect(inWindow.at(-1)?.key).toBe('2026-09-16');
    expect(inWindow.at(-1)?.isToday).toBe(true);
  });

  // Los Angeles leaves summer time on 1 November 2025, inside this window.
  it('gives every cell its own consecutive calendar date across a clock change', () => {
    const grid = buildActivityGrid([], TODAY);
    const keys = grid.days.map((d) => d.key);

    expect(new Set(keys).size).toBe(keys.length);
  });

  // The endpoint's cap is an absolute duration and a calendar day is 25 hours when the clocks
  // go back, so a window is refused on the days whose span happens to contain two autumn
  // transitions — and WHICH days those are is a property of the zone. Warsaw's fall in late
  // October is not Los Angeles's in early November, so the eastern suite's copy of this does
  // not cover it. Walked over a whole year, because picking one day picks a day that passes.
  it('asks for a span the endpoint will answer, on every day of the year', () => {
    const CAP_MS = 366 * 86_400_000;
    for (let i = 0; i < 366; i++) {
      const day = new Date(2026, 0, 1 + i);
      const { from, to } = rangeForWindow(day);
      expect(Date.parse(to) - Date.parse(from), day.toDateString()).toBeLessThanOrEqual(CAP_MS);
    }
  });
});

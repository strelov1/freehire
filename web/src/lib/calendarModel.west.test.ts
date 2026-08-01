// The mirror of calendarModel.test.ts, run west of UTC.
//
// It is a separate file because TZ is a per-process setting and vitest gives each file its
// own worker. One zone is not a test of timezone handling: a grid built from UTC accessors
// passes every eastern assertion that a correct one does for the opposite instant, and the
// failure only shows up on the other side of Greenwich.
process.env.TZ = 'America/Los_Angeles'; // UTC-7 in summer

import { describe, it, expect } from 'vitest';
import { buildCalendarMonth, rangeForMonth } from './calendarModel';
import type { TimelineEvent } from './types';

const event = (occurredAt: string, id = 1): TimelineEvent => ({
  id,
  kind: 'employer_reply',
  source: 'mail_gmail',
  observed: true,
  occurred_at: occurredAt,
  company_slug: 'derq',
});

describe('buildCalendarMonth west of UTC', () => {
  it('places an early-morning UTC instant on the previous local day', () => {
    // 01:00 UTC on 13 August is 18:00 on 12 August in Los Angeles. Reading the UTC date
    // would file it a day late — the mirror of the eastern case, and invisible without it.
    const month = buildCalendarMonth(2026, 7, [event('2026-08-13T01:00:00Z')]);

    expect(month.days.find((d) => d.key === '2026-08-12')?.events).toHaveLength(1);
    expect(month.days.find((d) => d.key === '2026-08-13')?.events).toHaveLength(0);
  });

  it('reaches back far enough for the first cell’s own evening', () => {
    // The first cell of the August grid is 27 July locally; an event at 03:00 UTC on
    // 28 July belongs to its evening. A range cut to the grid in UTC would miss it.
    const { from } = rangeForMonth(2026, 7);

    expect(new Date(from).getTime()).toBeLessThan(new Date('2026-07-27T00:00:00-07:00').getTime());
  });

  it('counts only the month’s own days, not the neighbouring pad cells', () => {
    // September 2026 starts on a Tuesday, so its grid leads with 31 August. An event there
    // is not September's, and counting it would suppress the message saying so.
    const month = buildCalendarMonth(2026, 8, [event('2026-08-31T18:00:00Z')]);

    expect(month.days.find((d) => d.key === '2026-08-31')?.events).toHaveLength(1);
    expect(month.total).toBe(0);
  });
});

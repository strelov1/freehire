import { describe, expect, test } from 'vitest';

import {
  dayStatuses,
  groupCalendarByLocalDay,
  splitAvailability,
  weekDraft,
  weekRows,
  nextWeekRange,
  exceptionDayLabel,
  weekdayLabel,
  weekdayOrder,
  weekdayShortLabel,
} from './mentorshipCalendar';
import type { MentorAvailabilityRule, MentorCalendarInterval } from './types';

function rule(over: Partial<MentorAvailabilityRule> = {}): MentorAvailabilityRule {
  return { id: 1, weekday: 1, date: null, start: '18:00', end: '21:00', closure: false, ...over };
}

describe('weekdays, stored and displayed', () => {
  // The one place the two conventions meet. Storage is Go's time.Weekday, where 0 is
  // SUNDAY; the calendar draws Monday first. Getting this backwards shifts a mentor's
  // whole week by a day and looks like they typed it wrong.
  test('storage numbers name the right days', () => {
    expect(weekdayLabel(0)).toBe('Sunday');
    expect(weekdayLabel(1)).toBe('Monday');
    expect(weekdayLabel(6)).toBe('Saturday');
  });

  test('the display order runs Monday to Sunday, in storage numbers', () => {
    expect(weekdayOrder()).toEqual([1, 2, 3, 4, 5, 6, 0]);
  });

  // The calendar's column headings come from here rather than from a second list in the
  // component. This asserts the headings a Monday-first grid actually draws, so the two
  // cannot drift into disagreeing about which column is which.
  test('the short labels spell the calendar header', () => {
    expect(weekdayOrder().map(weekdayShortLabel)).toEqual(['Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat', 'Sun']);
  });

  // The two together: reading the display order through the labels must spell the week,
  // which is the assertion that fails if either side is fixed without the other.
  test('the order and the labels agree', () => {
    expect(weekdayOrder().map(weekdayLabel)).toEqual([
      'Monday',
      'Tuesday',
      'Wednesday',
      'Thursday',
      'Friday',
      'Saturday',
      'Sunday',
    ]);
  });
});

describe('splitting availability into a week and its exceptions', () => {
  test('a rule is weekly or dated, and lands in exactly one half', () => {
    const split = splitAvailability([rule({ id: 1, weekday: 1 }), rule({ id: 2, weekday: null, date: '2026-10-16' })]);
    expect(split.weekly.map((r) => r.id)).toEqual([1]);
    expect(split.dated.map((r) => r.id)).toEqual([2]);
  });

  // Weekly rules come back in the DISPLAY order, so a mentor reads their week as a week.
  test('weekly rules are ordered Monday first, then by start time', () => {
    const split = splitAvailability([
      rule({ id: 1, weekday: 0, start: '09:00' }),
      rule({ id: 2, weekday: 1, start: '18:00' }),
      rule({ id: 3, weekday: 1, start: '09:00' }),
    ]);
    expect(split.weekly.map((r) => r.id)).toEqual([3, 2, 1]);
  });

  test('dated rules are ordered by date', () => {
    const split = splitAvailability([
      rule({ id: 1, weekday: null, date: '2026-11-02' }),
      rule({ id: 2, weekday: null, date: '2026-10-16' }),
    ]);
    expect(split.dated.map((r) => r.id)).toEqual([2, 1]);
  });

  test('an empty schedule splits into two empty halves', () => {
    expect(splitAvailability([])).toEqual({ weekly: [], dated: [] });
  });
});

function interval(startsAt: string, endsAt: string, status: MentorCalendarInterval['status']): MentorCalendarInterval {
  return { starts_at: startsAt, ends_at: endsAt, status };
}

describe('grouping a mentor own-calendar into local days', () => {
  test('an interval wholly inside one day is grouped under it alone', () => {
    const grouped = groupCalendarByLocalDay([interval('2026-09-08T16:00:00Z', '2026-09-08T17:00:00Z', 'free')], 'UTC');
    expect([...grouped.keys()]).toEqual(['2026-09-08']);
  });

  // The whole reason this differs from groupSlotsByLocalDay: a mentor with no
  // availability rules yet gets back ONE closed interval spanning the entire requested
  // window, not one per day. Bucketing only by start day would leave every day after the
  // first showing no data at all, when the source data says they are closed too.
  test('an interval spanning several days appears under every day it touches', () => {
    const grouped = groupCalendarByLocalDay(
      [interval('2026-09-01T00:00:00Z', '2026-09-04T00:00:00Z', 'closed')],
      'UTC',
    );
    expect([...grouped.keys()]).toEqual(['2026-09-01', '2026-09-02', '2026-09-03']);
  });

  // ends_at is exclusive, matching the domain's half-open intervals: an interval ending
  // exactly at a day's start does not touch that day.
  test('an interval ending exactly at midnight does not touch that day', () => {
    const grouped = groupCalendarByLocalDay([interval('2026-09-08T22:00:00Z', '2026-09-09T00:00:00Z', 'busy')], 'UTC');
    expect([...grouped.keys()]).toEqual(['2026-09-08']);
  });

  test('grouping is in the given timezone, not UTC', () => {
    // 22:00 UTC is already the 9th in Europe/Berlin (UTC+2 in September).
    const grouped = groupCalendarByLocalDay(
      [interval('2026-09-08T22:00:00Z', '2026-09-08T23:00:00Z', 'free')],
      'Europe/Berlin',
    );
    expect([...grouped.keys()]).toEqual(['2026-09-09']);
  });

  test('an empty interval list groups into nothing', () => {
    expect(groupCalendarByLocalDay([], 'UTC').size).toBe(0);
  });
});

describe('which statuses a day shows, in priority order', () => {
  test('booked outranks busy, free and closed', () => {
    const day = [
      interval('2026-09-08T09:00:00Z', '2026-09-08T10:00:00Z', 'closed'),
      interval('2026-09-08T18:00:00Z', '2026-09-08T19:00:00Z', 'free'),
      interval('2026-09-08T20:00:00Z', '2026-09-08T21:00:00Z', 'booked'),
    ];
    expect(dayStatuses(day)).toEqual(['booked', 'free', 'closed']);
  });

  test('a day with only closed time reports just closed', () => {
    expect(dayStatuses([interval('2026-09-08T00:00:00Z', '2026-09-09T00:00:00Z', 'closed')])).toEqual(['closed']);
  });

  test('a day with no intervals reports nothing', () => {
    expect(dayStatuses([])).toEqual([]);
  });
});

describe('the weekly schedule editor', () => {
  const slotRule = (id: number, weekday: number, start: string, end: string): MentorAvailabilityRule => ({
    id,
    weekday,
    date: null,
    start,
    end,
    closure: false,
  });

  test('draws all seven days Monday first, each with its own ranges in order', () => {
    const draft = weekDraft([
      slotRule(1, 0, '10:00', '12:00'),
      slotRule(2, 2, '19:00', '21:00'),
      slotRule(3, 2, '08:00', '09:00'),
    ]);
    expect(draft.map((d) => d.weekday)).toEqual([1, 2, 3, 4, 5, 6, 0]);
    expect(draft[1]?.ranges).toEqual([
      { start: '08:00', end: '09:00' },
      { start: '19:00', end: '21:00' },
    ]);
    expect(draft[6]?.ranges).toEqual([{ start: '10:00', end: '12:00' }]);
    expect(draft[0]?.ranges).toEqual([]);
  });

  test('flattens back into the rows the replace-week endpoint takes', () => {
    const rows = weekRows(weekDraft([slotRule(1, 0, '10:00', '12:00'), slotRule(2, 1, '18:00', '20:00')]));
    expect(rows).toEqual([
      { weekday: 1, start: '18:00', end: '20:00' },
      { weekday: 0, start: '10:00', end: '12:00' },
    ]);
  });

  test('a new range follows the last one, and never wraps past midnight', () => {
    expect(nextWeekRange([])).toEqual({ start: '18:00', end: '20:00' });
    expect(nextWeekRange([{ start: '09:00', end: '11:30' }])).toEqual({ start: '11:30', end: '13:30' });
    expect(nextWeekRange([{ start: '20:00', end: '23:00' }])).toEqual({ start: '18:00', end: '20:00' });
  });

  test('an exception names its day without reading the date in a zone', () => {
    expect(exceptionDayLabel('2026-11-04')).toBe('Wed 4 Nov 2026');
    expect(exceptionDayLabel('nonsense')).toBe('nonsense');
  });
});

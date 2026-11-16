import { describe, expect, test } from 'vitest';
import { slot } from './mentorshipTestUtils';
import {
  addMonths,
  daysInMonth,
  daysWithSlots,
  monthGrid,
  monthOf,
  slotWindowForMonth,
  todayIn,
  groupSlotsByLocalDay,
  slotLocalDay,
  slotLocalTime,
} from './mentorship';

describe('reading a slot in the viewer zone', () => {
  // THE trap this whole module exists to avoid. `local_start` is already in the viewer's
  // zone, so its own date IS the day it belongs to. Route it through a Date and the value
  // is re-interpreted against whatever zone the browser is in: for Tokyo's 16th at 01:00
  // the instant is still the 15th in UTC, and a naive
  // `new Date(local_start).toISOString().slice(0, 10)` files the slot under the wrong day.
  test('the local day is read from the label, not recomputed from the instant', () => {
    expect(slotLocalDay(slot('2026-10-16T01:00:00+09:00'))).toBe('2026-10-16');
    // Proof the naive route really does disagree, so this test is guarding something.
    expect(new Date('2026-10-16T01:00:00+09:00').toISOString().slice(0, 10)).toBe('2026-10-15');
  });

  test('the wall clock is read from the label too', () => {
    expect(slotLocalTime(slot('2026-10-16T01:30:00+09:00'))).toBe('01:30');
  });

  test('a zoneless label is still read as written', () => {
    expect(slotLocalDay(slot('2026-03-29T02:00:00Z'))).toBe('2026-03-29');
    expect(slotLocalTime(slot('2026-03-29T02:00:00Z'))).toBe('02:00');
  });
});

describe('grouping slots into calendar days', () => {
  test('slots are grouped under their own local day, in order', () => {
    const grouped = groupSlotsByLocalDay([
      slot('2026-10-15T11:00:00+03:00'),
      slot('2026-10-16T09:00:00+03:00'),
      slot('2026-10-15T17:00:00+03:00'),
    ]);
    expect([...grouped.keys()]).toEqual(['2026-10-15', '2026-10-16']);
    expect(grouped.get('2026-10-15')?.length).toBe(2);
  });

  // The autumn transition repeats an hour, so two DIFFERENT instants carry the same
  // wall-clock label and differ only by their offset. Both are real hours the mentor
  // stated; collapsing them would silently withhold one, and the spec says neither is.
  test('the repeated autumn hour keeps both slots', () => {
    const grouped = groupSlotsByLocalDay([
      slot('2026-10-25T02:00:00+03:00', '+03:00'),
      slot('2026-10-25T02:00:00+02:00', '+02:00'),
    ]);
    expect(grouped.get('2026-10-25')?.length).toBe(2);
  });

  test('an empty slot list groups into nothing', () => {
    expect(groupSlotsByLocalDay([]).size).toBe(0);
  });

  // What the month grid greys out. Derived from the same grouping rather than measured
  // separately, so a day can never be enabled in the grid and empty when opened.
  test('the days with slots are exactly the grouped keys', () => {
    const slots = [slot('2026-10-15T11:00:00+03:00'), slot('2026-10-20T11:00:00+03:00')];
    expect(daysWithSlots(slots)).toEqual(new Set(['2026-10-15', '2026-10-20']));
  });
});

describe('the month a day belongs to', () => {
  test('a day names its own month', () => {
    expect(monthOf('2026-10-15')).toBe('2026-10');
  });

  test('stepping forward rolls the year over', () => {
    expect(addMonths('2026-11', 1)).toBe('2026-12');
    expect(addMonths('2026-12', 1)).toBe('2027-01');
  });

  test('stepping back rolls the year under', () => {
    expect(addMonths('2026-01', -1)).toBe('2025-12');
  });

  test('the month is always two digits', () => {
    expect(addMonths('2026-08', 1)).toBe('2026-09');
  });
});

describe('days in a month', () => {
  test('the ordinary lengths', () => {
    expect(daysInMonth('2026-01')).toBe(31);
    expect(daysInMonth('2026-04')).toBe(30);
    expect(daysInMonth('2026-02')).toBe(28);
  });

  // The three leap rules, each with a case that only it decides. 2100 is the one a bare
  // "divisible by four" gets wrong, and it is inside the horizon of nothing — but the rule
  // costs one character and being wrong about it is the kind of thing nobody re-derives.
  test('the leap-year rules', () => {
    expect(daysInMonth('2028-02')).toBe(29);
    expect(daysInMonth('2000-02')).toBe(29);
    expect(daysInMonth('2100-02')).toBe(28);
  });
});

describe('the month grid', () => {
  // Monday-first, which is what the approved layout shows and what most of this
  // catalogue's audience reads.
  test('a month starting mid-week is padded to Monday', () => {
    // 2026-10-01 is a Thursday, so the first row carries three blanks.
    const weeks = monthGrid('2026-10');
    expect(weeks[0]?.slice(0, 4)).toEqual(['', '', '', '2026-10-01']);
  });

  test('a month starting on a Monday needs no padding', () => {
    // 2026-06-01 is a Monday.
    expect(monthGrid('2026-06')[0]?.[0]).toBe('2026-06-01');
  });

  test('every row is a full week and every day appears once', () => {
    const weeks = monthGrid('2026-10');
    for (const week of weeks) expect(week.length).toBe(7);
    const days = weeks.flat().filter(Boolean);
    expect(days.length).toBe(31);
    expect(new Set(days).size).toBe(31);
    expect(days[0]).toBe('2026-10-01');
    expect(days[30]).toBe('2026-10-31');
  });

  // The worst case for a Monday-first grid: 2026-02-01 is a Sunday, so it lands in the
  // LAST column and the first row is six blanks. A Sunday-first grid would have needed
  // none, which is exactly why the convention has to be decided once and tested.
  test('a month starting on a Sunday is padded by six', () => {
    const weeks = monthGrid('2026-02');
    expect(weeks[0]).toEqual(['', '', '', '', '', '', '2026-02-01']);
    expect(weeks.flat().filter(Boolean).length).toBe(28);
    expect(weeks.at(-1)?.some((d) => d !== '')).toBe(true);
  });
});

describe('today, in the zone that was actually used', () => {
  // "Today" is a question about a zone, not about a machine. At 16:00 UTC it is already
  // tomorrow in Tokyo, so a calendar that opens on the server's day opens on the wrong
  // month for a seeker there for eight hours out of every twenty-four.
  test('the same instant is a different day in two zones', () => {
    const instant = new Date('2026-10-15T16:00:00Z');
    expect(todayIn('Asia/Tokyo', instant)).toBe('2026-10-16');
    expect(todayIn('UTC', instant)).toBe('2026-10-15');
    expect(todayIn('America/Los_Angeles', instant)).toBe('2026-10-15');
  });

  // Just past midnight UTC is still the previous day west of it — the mirror case, so a
  // wrong sign in the conversion cannot pass both.
  test('the day before midnight UTC still reads as yesterday in the west', () => {
    const instant = new Date('2026-10-15T00:30:00Z');
    expect(todayIn('America/Los_Angeles', instant)).toBe('2026-10-14');
    expect(todayIn('UTC', instant)).toBe('2026-10-15');
  });
});

describe('the slot window a month needs', () => {
  // The endpoint takes INSTANTS and answers in the viewer's zone, so a window cut exactly
  // on UTC month boundaries loses the month's first local morning east of UTC and its last
  // local evening west of it. A day of padding on each side costs nothing and closes both.
  test('the window covers the month with a day of padding on each side', () => {
    expect(slotWindowForMonth('2026-10')).toEqual({
      from: '2026-09-30T00:00:00Z',
      to: '2026-11-01T00:00:00Z',
    });
  });

  test('padding rolls across a year boundary', () => {
    expect(slotWindowForMonth('2026-01')).toEqual({
      from: '2025-12-31T00:00:00Z',
      to: '2026-02-01T00:00:00Z',
    });
  });

  test('padding lands on the right day after a leap February', () => {
    expect(slotWindowForMonth('2028-03').from).toBe('2028-02-29T00:00:00Z');
  });

  // The endpoint refuses a span wider than 62 days; a padded month is at most 33.
  test('the window never exceeds what the endpoint will compute', () => {
    for (const month of ['2026-01', '2026-02', '2028-02', '2026-04']) {
      const { from, to } = slotWindowForMonth(month);
      const days = (Date.parse(to) - Date.parse(from)) / 86_400_000;
      expect(days).toBeLessThanOrEqual(62);
    }
  });
});

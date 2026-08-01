// The reader's timezone is the whole point of this module, so the suite runs in one that
// is not UTC. Set before anything touches Date: Node reads TZ lazily, but only the first
// use of a given date-time formatting path is guaranteed to see a later change.
process.env.TZ = 'Europe/Warsaw'; // UTC+2 in summer

import { describe, it, expect } from 'vitest';
import { buildCalendarMonth, rangeForMonth, splitDayEvents } from './calendarModel';
import type { TimelineEvent } from './types';

const event = (occurredAt: string, over: Partial<TimelineEvent> = {}): TimelineEvent => ({
  id: 1,
  kind: 'employer_reply',
  source: 'mail_gmail',
  observed: true,
  occurred_at: occurredAt,
  company_slug: 'derq',
  ...over,
});

// requireDay asserts a day exists at a key and returns it narrowed, so the tests read
// straight-line under noUncheckedIndexedAccess.
function dayAt(month: ReturnType<typeof buildCalendarMonth>, key: string) {
  const day = month.days.find((d) => d.key === key);
  if (!day) throw new Error(`expected a day ${key} in the grid, got ${month.days.map((d) => d.key).join(', ')}`);
  return day;
}

describe('buildCalendarMonth', () => {
  it('places an instant on the reader’s day, not on UTC’s', () => {
    // 23:40 UTC on 12 August is 01:40 on 13 August in Warsaw. A grid built from the UTC
    // date would file it a day early, and the reader would look for it on the wrong row.
    const month = buildCalendarMonth(2026, 7, [event('2026-08-12T23:40:00Z')]);

    expect(dayAt(month, '2026-08-13').events).toHaveLength(1);
    expect(dayAt(month, '2026-08-12').events).toHaveLength(0);
  });

  it('gives every cell of the grid, empty ones included', () => {
    const month = buildCalendarMonth(2026, 7, []);

    expect(month.days.length % 7).toBe(0);
    expect(month.weeks.every((w) => w.length === 7)).toBe(true);
    expect(month.days.every((d) => d.events.length === 0)).toBe(true);
    // August 2026 starts on a Saturday, so a Monday-first grid leads with July's tail.
    expect(month.days[0]?.inMonth).toBe(false);
    expect(dayAt(month, '2026-08-01').inMonth).toBe(true);
  });

  it('fills the leading and trailing cells that belong to neighbouring months', () => {
    const month = buildCalendarMonth(2026, 7, [
      event('2026-07-31T09:00:00Z', { id: 2 }),
      event('2026-09-01T09:00:00Z', { id: 3 }),
    ]);

    expect(dayAt(month, '2026-07-31').events).toHaveLength(1);
    expect(dayAt(month, '2026-07-31').inMonth).toBe(false);
    expect(dayAt(month, '2026-09-01').events).toHaveLength(1);
  });

  it('orders a day’s events oldest first', () => {
    const month = buildCalendarMonth(2026, 7, [
      event('2026-08-13T18:00:00Z', { id: 20 }),
      event('2026-08-13T07:00:00Z', { id: 10 }),
    ]);

    expect(dayAt(month, '2026-08-13').events.map((e) => e.id)).toEqual([10, 20]);
  });

  it('keeps an event whose kind it does not recognise', () => {
    const month = buildCalendarMonth(2026, 7, [
      event('2026-08-13T09:00:00Z', { kind: 'interview_scheduled' }),
    ]);

    expect(dayAt(month, '2026-08-13').events).toHaveLength(1);
  });

  it('lists only the days that hold events, for the narrow layout', () => {
    const month = buildCalendarMonth(2026, 7, [
      event('2026-08-03T09:00:00Z', { id: 1 }),
      event('2026-08-20T09:00:00Z', { id: 2 }),
    ]);

    expect(month.daysWithEvents.map((d) => d.key)).toEqual(['2026-08-03', '2026-08-20']);
  });
});

describe('rangeForMonth', () => {
  it('covers the whole grid and a day either side of it', () => {
    const month = buildCalendarMonth(2026, 7, []);
    const { from, to } = rangeForMonth(2026, 7);

    const firstCell = new Date(`${month.days[0]?.key}T00:00:00`).getTime();
    const lastCell = new Date(`${month.days[month.days.length - 1]?.key}T00:00:00`).getTime();
    const day = 24 * 60 * 60 * 1000;

    // A grid cell's own events must be inside the requested span, with margin to spare:
    // without it the first and last rows are systematically short.
    expect(new Date(from).getTime()).toBeLessThanOrEqual(firstCell - day);
    expect(new Date(to).getTime()).toBeGreaterThanOrEqual(lastCell + day);
  });

  it('asks for a span the API will accept', () => {
    const { from, to } = rangeForMonth(2026, 7);
    const days = (new Date(to).getTime() - new Date(from).getTime()) / (24 * 60 * 60 * 1000);

    expect(days).toBeGreaterThan(0);
    expect(days).toBeLessThanOrEqual(366); // apptimeline.MaxRangeDays
  });
});

describe('splitDayEvents', () => {
  it('reports what a full cell could not show', () => {
    const many = Array.from({ length: 7 }, (_, i) => event('2026-08-13T09:00:00Z', { id: i }));
    const { shown, remaining } = splitDayEvents(many, 4);

    expect(shown).toHaveLength(4);
    expect(remaining).toBe(3);
  });

  it('reports nothing remaining when the cell fits', () => {
    const { shown, remaining } = splitDayEvents([event('2026-08-13T09:00:00Z')], 4);

    expect(shown).toHaveLength(1);
    expect(remaining).toBe(0);
  });
});

// Pure month arithmetic for the tracking calendar: turn a flat series of application
// events into the grid a month is drawn on, and work out the range to fetch for it.
//
// Kept out of the Svelte component for the reason activityChart.ts gives — the bug-prone
// part is the arithmetic, and here that part is the timezone. `occurred_at` is an instant;
// which cell it belongs to depends on the reader's clock, and only the browser knows that.
// Everything below therefore reads a Date through its LOCAL accessors and never through
// the UTC ones, and is testable without rendering.

import type { TimelineEvent } from './types';

const MS_PER_DAY = 24 * 60 * 60 * 1000;

/** One cell of the grid. */
export interface CalendarDay {
  /** The reader's own calendar date, YYYY-MM-DD — the grouping key, and stable to compare. */
  key: string;
  date: Date;
  dayOfMonth: number;
  /** False for the neighbouring month's days that pad the first and last rows. */
  inMonth: boolean;
  isToday: boolean;
  /** That day's events, oldest first. */
  events: TimelineEvent[];
}

/** A drawable month. */
export interface CalendarMonth {
  year: number;
  /** 0-based, as Date uses. */
  month: number;
  /** Whole weeks, Monday first — every row is seven cells. */
  weeks: CalendarDay[][];
  /** The same cells, flat and in order. */
  days: CalendarDay[];
  /** Only the cells holding something, for the narrow layout that lists days. */
  daysWithEvents: CalendarDay[];
  /** How many events the grid holds in total — an empty month is not an empty account. */
  total: number;
}

function pad(n: number): string {
  return String(n).padStart(2, '0');
}

/** The reader's own calendar date for an instant. Local accessors, deliberately: the
 *  UTC ones would file a late-evening event on the previous day for anyone east of
 *  Greenwich, and on the next one for anyone west of it. */
export function dayKey(d: Date): string {
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
}

/** Monday-first weekday index, 0–6. */
function mondayIndex(d: Date): number {
  return (d.getDay() + 6) % 7;
}

/** The first and last cells of a month's grid, in the reader's zone. The grid runs whole
 *  weeks, so it reaches into the neighbouring months — and those cells show events too. */
function gridBounds(year: number, month: number): { first: Date; last: Date } {
  const firstOfMonth = new Date(year, month, 1);
  const lastOfMonth = new Date(year, month + 1, 0);
  return {
    first: new Date(year, month, 1 - mondayIndex(firstOfMonth)),
    last: new Date(year, month + 1, 0 + (6 - mondayIndex(lastOfMonth))),
  };
}

/** Adds whole days by calendar arithmetic rather than by milliseconds, so a DST boundary
 *  inside the month does not shift every later cell by an hour. */
function addDays(d: Date, days: number): Date {
  return new Date(d.getFullYear(), d.getMonth(), d.getDate() + days);
}

/** buildCalendarMonth groups a series into the cells of one month's grid.
 *
 *  `events` may cover more than the month — the fetch deliberately asks for margin — and
 *  anything outside the grid is simply not placed. */
export function buildCalendarMonth(
  year: number,
  month: number,
  events: TimelineEvent[],
  today: Date = new Date(),
): CalendarMonth {
  const byDay = new Map<string, TimelineEvent[]>();
  for (const e of events) {
    const key = dayKey(new Date(e.occurred_at));
    const bucket = byDay.get(key);
    if (bucket) bucket.push(e);
    else byDay.set(key, [e]);
  }
  for (const bucket of byDay.values()) {
    bucket.sort((a, b) => a.occurred_at.localeCompare(b.occurred_at));
  }

  const { first, last } = gridBounds(year, month);
  const cells = Math.round((last.getTime() - first.getTime()) / MS_PER_DAY) + 1;
  const todayKey = dayKey(today);

  const days: CalendarDay[] = [];
  for (let i = 0; i < cells; i++) {
    const date = addDays(first, i);
    const key = dayKey(date);
    days.push({
      key,
      date,
      dayOfMonth: date.getDate(),
      inMonth: date.getMonth() === month && date.getFullYear() === year,
      isToday: key === todayKey,
      events: byDay.get(key) ?? [],
    });
  }

  const weeks: CalendarDay[][] = [];
  for (let i = 0; i < days.length; i += 7) weeks.push(days.slice(i, i + 7));

  return {
    year,
    month,
    weeks,
    days,
    daysWithEvents: days.filter((d) => d.events.length > 0),
    total: days.reduce((n, d) => n + d.events.length, 0),
  };
}

/** rangeForMonth is the span to fetch so every cell of the month's grid is complete.
 *
 *  It covers the grid — which reaches into the neighbouring months — plus a day either
 *  side. The margin is what makes the edges honest: an event that occurred late on the
 *  day before the first cell in UTC belongs to the first cell for a reader ahead of it,
 *  and a range cut to the grid would leave it out. */
export function rangeForMonth(year: number, month: number): { from: string; to: string } {
  const { first, last } = gridBounds(year, month);
  const from = addDays(first, -1);
  const to = addDays(last, 2); // start of the day after the margin day…
  return { from: from.toISOString(), to: new Date(to.getTime() - 1).toISOString() }; // …less an instant
}

/** splitDayEvents divides a cell's events into the ones it shows and a count of the rest.
 *  A bulk-apply evening is ordinary behaviour, and a cell that silently dropped the tail
 *  would under-report the day. */
export function splitDayEvents(
  events: TimelineEvent[],
  cap: number,
): { shown: TimelineEvent[]; remaining: number } {
  return { shown: events.slice(0, cap), remaining: Math.max(0, events.length - cap) };
}

/** The month's own name, in the reader's locale. */
export function monthLabel(year: number, month: number): string {
  return new Date(year, month, 1).toLocaleDateString(undefined, { month: 'long', year: 'numeric' });
}

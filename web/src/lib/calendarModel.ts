// Pure month arithmetic for the tracking calendar: turn a flat series of application
// events into the grid a month is drawn on, and work out the range to fetch for it.
//
// Kept out of the Svelte component for the reason activityChart.ts gives — the bug-prone
// part is the arithmetic, and here that part is the timezone. `occurred_at` is an instant;
// which cell it belongs to depends on the reader's clock, and only the browser knows that.
// Everything below therefore reads a Date through its LOCAL accessors and never through
// the UTC ones, and is testable without rendering.

import type { ScheduledInterview, TimelineEvent } from './types';

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
  /** That day's events, oldest first — what happened. */
  events: TimelineEvent[];
  /** That day's arranged meetings, oldest first — what is going to. Kept apart from the
   *  events rather than merged into one list, because the view must be able to draw the
   *  difference: one is a record, the other is an appointment that can still move. */
  interviews: ScheduledInterview[];
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
  /** How much this month's OWN days hold, events and arranged meetings together.
   *
   *  Deliberately not the whole grid: a September whose only mark sits in its 31 August
   *  pad cell holds nothing of its own, and counting the pad would suppress the message
   *  saying so. And deliberately not events alone: a month whose only content is an
   *  interview next week is not an empty month, and saying nothing is recorded would be
   *  both wrong and discouraging. */
  total: number;
}

function pad(n: number): string {
  return String(n).padStart(2, '0');
}

/** The reader's own calendar date for an instant. Local accessors, deliberately: the
 *  UTC ones would file a late-evening event on the previous day for anyone east of
 *  Greenwich, and on the next one for anyone west of it. */
function dayKey(d: Date): string {
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
}

/** Buckets a series by the reader's local day, oldest first within each day.
 *
 *  Sorted by instant rather than by string. Postgres keeps microseconds and Go trims
 *  trailing zeros, so one second can hold both "09:00:00Z" and "09:00:00.482913Z" — and
 *  lexically '.' sorts before 'Z', which would put the later one first. */
function groupByLocalDay<T>(items: T[], instantOf: (item: T) => string): Map<string, T[]> {
  const byDay = new Map<string, T[]>();
  for (const item of items) {
    const key = dayKey(new Date(instantOf(item)));
    const bucket = byDay.get(key);
    if (bucket) bucket.push(item);
    else byDay.set(key, [item]);
  }
  for (const bucket of byDay.values()) {
    bucket.sort((a, b) => Date.parse(instantOf(a)) - Date.parse(instantOf(b)));
  }
  return byDay;
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
  interviews: ScheduledInterview[] = [],
  today: Date = new Date(),
): CalendarMonth {
  const byDay = groupByLocalDay(events, (e) => e.occurred_at);
  const meetingsByDay = groupByLocalDay(interviews, (i) => i.starts_at);

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
      interviews: meetingsByDay.get(key) ?? [],
    });
  }

  const weeks: CalendarDay[][] = [];
  for (let i = 0; i < days.length; i += 7) weeks.push(days.slice(i, i + 7));

  return {
    year,
    month,
    weeks,
    days,
    daysWithEvents: days.filter((d) => d.events.length + d.interviews.length > 0),
    total: days.reduce((n, d) => n + (d.inMonth ? d.events.length + d.interviews.length : 0), 0),
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

/** splitDayEvents divides a cell's marks into the ones it shows and a count of the rest.
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

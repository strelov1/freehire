// The calendar and weekly-schedule arithmetic of the mentorship screens, split out
// of mentorship.ts: month grids, local days, day statuses and availability weeks.
import type { MentorAvailabilityRule, MentorCalendarInterval } from './types';

export const pad2 = (n: number) => String(n).padStart(2, '0');

/** The `YYYY-MM` a `YYYY-MM-DD` belongs to. */
export function monthOf(day: string): string {
  return day.slice(0, 7);
}

/** Step a `YYYY-MM` by whole months, rolling the year. */
export function addMonths(month: string, delta: number): string {
  const year = Number(month.slice(0, 4));
  const index = Number(month.slice(5, 7)) - 1 + delta;
  return `${year + Math.floor(index / 12)}-${pad2((((index % 12) + 12) % 12) + 1)}`;
}

/** Length of a `YYYY-MM`, Gregorian leap rules included — divisible by 4, except
 *  centuries, except those divisible by 400. */
export function daysInMonth(month: string): number {
  const year = Number(month.slice(0, 4));
  const m = Number(month.slice(5, 7));
  if (m === 2) return year % 4 === 0 && (year % 100 !== 0 || year % 400 === 0) ? 29 : 28;
  // Written out rather than indexed into a table of lengths: with
  // `noUncheckedIndexedAccess` a table lookup is `number | undefined` and needs either a
  // fallback that can never fire or a throw, both of which are more code than the four
  // thirty-day months.
  return m === 4 || m === 6 || m === 9 || m === 11 ? 30 : 31;
}

/** Weekday of a `YYYY-MM-DD` as 0=Monday … 6=Sunday, by Sakamoto's method. */
function weekdayMondayFirst(year: number, month: number, day: number): number {
  const offsets = [0, 3, 2, 5, 0, 3, 5, 1, 4, 6, 2, 4];
  const offset = offsets[month - 1];
  // Unreachable for a well-formed `YYYY-MM`, and loud rather than silent if the caller
  // ever hands over something else: a wrong offset here does not fail, it draws a whole
  // month under the wrong weekdays, which reads as a design choice.
  if (offset === undefined) throw new RangeError(`month out of range: ${month}`);
  const y = month < 3 ? year - 1 : year;
  const sunday0 = (y + Math.floor(y / 4) - Math.floor(y / 100) + Math.floor(y / 400) + offset + day) % 7;
  return (sunday0 + 6) % 7;
}

/** A `YYYY-MM` as rows of seven, Monday first, padded with `''` at both ends so every row
 *  is a full week and each column is one weekday. Monday-first because that is what this
 *  catalogue's audience reads; the convention is decided here once and nowhere else. */
export function monthGrid(month: string): string[][] {
  const year = Number(month.slice(0, 4));
  const m = Number(month.slice(5, 7));
  const cells: string[] = Array(weekdayMondayFirst(year, m, 1)).fill('');
  for (let day = 1; day <= daysInMonth(month); day++) cells.push(`${month}-${pad2(day)}`);
  while (cells.length % 7 !== 0) cells.push('');

  const weeks: string[][] = [];
  for (let i = 0; i < cells.length; i += 7) weeks.push(cells.slice(i, i + 7));
  return weeks;
}

/** Read one field out of an `Intl` parts list. Shared by the two callers below rather
 *  than spelled twice — they format different things and read them the same way. */
export function part(parts: Intl.DateTimeFormatPart[], type: string): string {
  return parts.find((p) => p.type === type)?.value ?? '';
}

/** Today's date in a named zone, `YYYY-MM-DD`.
 *
 *  "Today" is a question about a zone, not about a machine: at 16:00 UTC it is already
 *  tomorrow in Tokyo. Ask it with the zone the SLOT RESPONSE reported, not the one the
 *  browser guessed — an unrecognised name was answered in UTC, and a calendar opened on
 *  the browser's month would then disagree with every time printed in it.
 *
 *  `formatToParts` rather than a locale that happens to print ISO: `en-CA` and `sv-SE`
 *  both do today, and neither promises to. */
export function todayIn(timezone: string, now: Date = new Date()): string {
  const parts = new Intl.DateTimeFormat('en-US', {
    timeZone: timezone,
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
  }).formatToParts(now);
  return `${part(parts, 'year')}-${part(parts, 'month')}-${part(parts, 'day')}`;
}

// ---- the mentor's own calendar ------------------------------------------------------
//
// Unlike a slot, a calendar interval can span many days — a mentor with no availability
// rules yet gets back ONE `closed` interval covering the whole requested window, not one
// per day. Bucketing only by start day (the way groupSlotsByLocalDay does) would leave
// every day after the first showing no data at all, so grouping here walks every day an
// interval touches instead.

/** The calendar day right after a `YYYY-MM-DD`, rolling months and years. Plain
 *  calendar-date arithmetic — no zone, because a day already resolved from an instant
 *  does not need resolving twice. */
function nextDay(day: string): string {
  const month = day.slice(0, 7);
  const date = Number(day.slice(8, 10));
  if (date < daysInMonth(month)) return `${month}-${String(date + 1).padStart(2, '0')}`;
  return `${addMonths(month, 1)}-01`;
}

/** Groups a mentor's own resolved-calendar intervals by every local day they touch, in a
 *  given zone. `ends_at` is exclusive, matching the domain's half-open intervals — one
 *  ending exactly at a day's start does not touch that day. */
export function groupCalendarByLocalDay(
  intervals: MentorCalendarInterval[],
  timezone: string,
): Map<string, MentorCalendarInterval[]> {
  const byDay = new Map<string, MentorCalendarInterval[]>();
  for (const iv of intervals) {
    const startDay = todayIn(timezone, new Date(iv.starts_at));
    const lastInstant = new Date(Date.parse(iv.ends_at) - 1);
    const endDay = todayIn(timezone, lastInstant);
    for (let day = startDay; day <= endDay; day = nextDay(day)) {
      const bucket = byDay.get(day);
      if (bucket) bucket.push(iv);
      else byDay.set(day, [iv]);
    }
  }
  return byDay;
}

/** Which statuses a day shows, in the same priority the domain partition itself carves
 *  the window in — booked over busy over free over closed. Drives the month grid's
 *  compact per-day indicator; the expanded day view shows the actual intervals. */
export function dayStatuses(dayIntervals: MentorCalendarInterval[]): MentorCalendarInterval['status'][] {
  const present = new Set(dayIntervals.map((iv) => iv.status));
  return (['booked', 'busy', 'free', 'closed'] as const).filter((s) => present.has(s));
}

// ---- the mentor's own schedule ------------------------------------------------------
//
// Two conventions meet here and they disagree. A stored `weekday` is Go's `time.Weekday`,
// where 0 is SUNDAY; the calendar draws Monday first. Both are correct in their own place,
// so neither is "fixed" — they are mapped, once, below. Getting it backwards shifts a
// mentor's whole week by a day, which reads as though they typed it wrong.

const WEEKDAY_NAMES = ['Sunday', 'Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday'];

/** The three-letter column heading for a STORED weekday number, for a grid too narrow to
 *  spell the day out. Here rather than in the calendar component, because the module says
 *  it owns this convention and a second list in a component makes that untrue. */
export function weekdayShortLabel(weekday: number): string {
  return weekdayLabel(weekday).slice(0, 3);
}

/** The name of a STORED weekday number (0 = Sunday). */
export function weekdayLabel(weekday: number): string {
  return WEEKDAY_NAMES[weekday] ?? '';
}

/** The stored weekday numbers in DISPLAY order — Monday first, Sunday last. */
export function weekdayOrder(): number[] {
  return [1, 2, 3, 4, 5, 6, 0];
}

/** A schedule split the way a mentor reads it: the recurring week, and the exceptions to
 *  it. A rule carries a weekday or a date and never both, so the split is total.
 *
 *  The week comes back in display order rather than storage order, and the dates in
 *  chronological order — the endpoint promises neither. */
export function splitAvailability(rules: MentorAvailabilityRule[]): {
  weekly: MentorAvailabilityRule[];
  dated: MentorAvailabilityRule[];
} {
  const position = new Map(weekdayOrder().map((weekday, index) => [weekday, index]));
  const weekly = rules
    .filter((r) => r.weekday != null)
    .sort(
      (a, b) =>
        (position.get(a.weekday as number) ?? 0) - (position.get(b.weekday as number) ?? 0) ||
        a.start.localeCompare(b.start),
    );
  const dated = rules.filter((r) => r.date != null).sort((a, b) => (a.date ?? '').localeCompare(b.date ?? ''));
  return { weekly, dated };
}

/** One editable range of a weekly schedule, wall clock "HH:MM". */
export interface WeekRange {
  start: string;
  end: string;
}

/** The recurring week as the editor draws it: all seven days in display order, each with
 *  its ranges (none for a day off). Built from the stored weekly rules. */
export function weekDraft(weekly: MentorAvailabilityRule[]): { weekday: number; ranges: WeekRange[] }[] {
  const { weekly: sorted } = splitAvailability(weekly);
  return weekdayOrder().map((weekday) => ({
    weekday,
    ranges: sorted.filter((r) => r.weekday === weekday).map((r) => ({ start: r.start, end: r.end })),
  }));
}

/** A draft week flattened into the rows the replace-week endpoint takes, in display order. */
export function weekRows(
  draft: { weekday: number; ranges: WeekRange[] }[],
): { weekday: number; start: string; end: string }[] {
  return draft.flatMap((day) => day.ranges.map((r) => ({ weekday: day.weekday, start: r.start, end: r.end })));
}

function wallClock(minutes: number): string {
  return `${String(Math.floor(minutes / 60)).padStart(2, '0')}:${String(minutes % 60).padStart(2, '0')}`;
}

/** The range a day gains when another is added: two hours from where the day's last range
 *  ends, or the early evening on an empty day. A range that would run past midnight falls
 *  back to the evening default rather than wrapping. */
export function nextWeekRange(ranges: WeekRange[]): WeekRange {
  const last = ranges[ranges.length - 1];
  const fallback = { start: '18:00', end: '20:00' };
  if (!last) return fallback;
  const m = /^(\d{2}):(\d{2})/.exec(last.end);
  if (!m) return fallback;
  const startMinutes = Number(m[1]) * 60 + Number(m[2]);
  const endMinutes = startMinutes + 120;
  if (endMinutes >= 24 * 60) return fallback;
  return { start: wallClock(startMinutes), end: wallClock(endMinutes) };
}

/** A dated exception's day as "Tue 4 Nov 2026", read from its YYYY-MM-DD without a zone. */
export function exceptionDayLabel(date: string): string {
  const m = /^(\d{4})-(\d{2})-(\d{2})$/.exec(date);
  if (!m) return date;
  const at = new Date(Date.UTC(Number(m[1]), Number(m[2]) - 1, Number(m[3])));
  const day = weekdayShortLabel(at.getUTCDay());
  const month = at.toLocaleString('en-GB', { month: 'short', timeZone: 'UTC' });
  return `${day} ${Number(m[3])} ${month} ${m[1]}`;
}

// ---- a booked session ---------------------------------------------------------------

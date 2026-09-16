// Pure arithmetic for the tracking section's activity grid: turn a year of ledger events
// into the squares it is drawn as, and decide which of those events count toward one.
//
// Kept out of the Svelte component for the reason calendarModel.ts gives — the bug-prone
// part is the timezone — and here the stakes are a step higher. On the calendar a misfiled
// event appears on the wrong square; here it can break or invent a streak, which is the one
// figure this view exists to publish.

import type { ApplicationEventKind, ApplicationEventSource } from './generated/contracts';
import type { TimelineEvent } from './types';

/** What a kind is, for counting purposes.
 *
 *  - `candidate` — the candidate's work whoever recorded it. `applied` read out of their own
 *    inbox is still their application; the mail path observed it, it did not perform it.
 *  - `source-decides` — a kind both the candidate and the platform produce, so the source
 *    settles it. Today `stage_set` alone.
 *  - `someone-else` — never the candidate's work, whatever the source says. */
type KindVerdict = 'candidate' | 'source-decides' | 'someone-else';

/** The verdict per kind. Exported and total over `APPLICATION_EVENT_KINDS` — a kind added in
 *  Go and forgotten here fails a test rather than quietly scoring zero, which would be
 *  indistinguishable from a day the candidate did nothing. */
export const KIND_VERDICT: Record<ApplicationEventKind, KindVerdict> = {
  applied: 'candidate',
  follow_up_sent: 'candidate',
  stage_set: 'source-decides',
  // An employer writing is their action, not ours. It belongs in the day panel, and the
  // pipeline view is where "am I getting responses?" is answered.
  employer_reply: 'someone-else',
  // Getting an interview is an outcome, and the arranging is usually the employer's. A
  // square measures the work, not what the work earned.
  interview_scheduled: 'someone-else',
};

/** Whether a source means a PERSON acted. Exported and total over
 *  `APPLICATION_EVENT_SOURCES` for the same reason as the kinds above. */
export const SOURCE_IS_THE_CANDIDATE: Record<ApplicationEventSource, boolean> = {
  user: true,
  assistant: true,
  // The candidate configured auto-apply and approved the tailored CV it submits. Theirs.
  auto_apply: true,
  // The mail and calendar sources date somebody ELSE's behaviour; reading an inbox is not an
  // action. `system` records only that the platform noticed something — on production it is
  // the single largest non-candidate category, and almost all of it is a listing being
  // noticed closed.
  mail_gmail: false,
  mail_hosted: false,
  mail_external: false,
  calendar_google: false,
  system: false,
};

/** Whether an event is work the CANDIDATE did, and so shades a square.
 *
 *  A closed list, not a denylist. A denylist admits whatever the ledger gains next, and what
 *  it is most likely to gain next is another observation of somebody else's behaviour —
 *  which would inflate every square while every test stayed green. A closed list fails the
 *  other way: a genuine new candidate action goes uncounted, which is visible, and which the
 *  exhaustiveness test holds to the generated vocabularies rather than to anyone's memory. */
export function countsAsCandidateAction(event: TimelineEvent): boolean {
  const verdict = KIND_VERDICT[event.kind as ApplicationEventKind];
  if (verdict === 'candidate') return true;
  if (verdict === 'source-decides') return SOURCE_IS_THE_CANDIDATE[event.source as ApplicationEventSource] === true;
  return false;
}

/** How many days the grid covers, ending today.
 *
 *  Bounded by the endpoint, not chosen for looks. `apptimeline.MaxRangeDays` refuses a request
 *  outright — 400, not a trimmed answer — when `to.Sub(from) > 366*24h`, and the fetch adds a
 *  day of margin at each end, so the REQUEST spans `WINDOW_DAYS + 2` calendar days.
 *
 *  The trap is that the cap is an ABSOLUTE duration and a calendar day is not always 24 hours.
 *  A span of 366 dates that contains two autumn clock changes lasts 366 days and two hours,
 *  and is refused. 364 therefore looked exact and was wrong: in Warsaw on 24 and 25 October
 *  2026 — and in Los Angeles around 1 November — every reader would have got the error state.
 *
 *  363 leaves a day of slack, which is more than any timezone's transitions can consume, and
 *  `rangeForWindow`'s own test walks all 366 days of a year rather than asserting on one:
 *  picking a single day to check picks a day that passes. */
export const WINDOW_DAYS = 363;

/** The number of shades above zero. */
export const LEVELS = 4;

/** Above this busiest-day figure the scale stretches; at or below it, a count reads as
 *  itself. Set at twice the number of shades: below it there are fewer distinct counts than
 *  shades, so proportions would be inventing distinctions the data does not hold. */
const STRETCH_ABOVE = LEVELS * 2;

/** Which shade a day is drawn in: 0 for a day with no counting actions, otherwise 1..LEVELS.
 *
 *  `busiest` is the reader's OWN busiest day in the window, because no absolute scale fits
 *  this data. Measured on production, the median caller records 5 counting actions in a whole
 *  year and p95 records 11, while the heaviest records 655 — a scale calibrated for 655 draws
 *  almost every real grid as a uniform palest green, and one calibrated for 5 saturates the
 *  moment anybody bulk-applies.
 *
 *  Two regimes, because a purely proportional scale fails at the bottom: for the person whose
 *  best day was one application, `count / busiest` is 1 on every active day, and they see a
 *  solid dark year that says nothing. So a small maximum is read literally — one action is
 *  one shade — and only a large one is divided into quarters.
 *
 *  A count above zero never returns zero, whatever `busiest` says. A real action drawn as a
 *  rest day would break, on screen, a streak the person did not break. */
export function levelFor(count: number, busiest: number): number {
  if (count <= 0) return 0;
  if (busiest <= STRETCH_ABOVE) return Math.min(count, LEVELS);
  return Math.min(LEVELS, Math.max(1, Math.ceil((count / busiest) * LEVELS)));
}

/** rangeForWindow is the span to fetch so every square the reader draws is complete.
 *
 *  The window plus a day of margin either side. The margin is what makes the edges honest:
 *  an event that occurred late on the day before the first square in UTC belongs to that
 *  square for a reader ahead of Greenwich, and a range cut to the window would leave it out.
 *  It is also what bounds `WINDOW_DAYS` — see the note there.
 *
 *  Shared by the server load and the component's own fallback fetch, so the two can never ask
 *  for different spans and disagree about what the grid is missing. */
export function rangeForWindow(today: Date = new Date()): { from: string; to: string } {
  const start = new Date(today.getFullYear(), today.getMonth(), today.getDate());
  // The window's first day is `WINDOW_DAYS - 1` back; one more is the margin day before it.
  const from = addDays(start, -WINDOW_DAYS);
  const to = addDays(start, 2); // start of the day after the margin day…
  return { from: from.toISOString(), to: new Date(to.getTime() - 1).toISOString() }; // …less an instant
}

/** One square. */
export interface ActivityDay {
  /** The reader's own calendar date, YYYY-MM-DD — the grouping key, and stable to compare. */
  key: string;
  date: Date;
  /** How many of that day's events were the candidate's own work. */
  count: number;
  /** The shade, 0 for a day with no counting actions. */
  level: number;
  /** EVERY event that day, oldest first — including the ones that do not count. A square
   *  measures effort; the panel shows history, and a day whose only event was an employer's
   *  reply must still be readable. */
  events: TimelineEvent[];
  isToday: boolean;
  /** False for the cells that only exist to keep the first and last rows seven long. They are
   *  not days we measured, and drawing them as level-zero would claim otherwise. */
  inWindow: boolean;
}

/** One column: a whole week, Monday first. */
export interface ActivityWeek {
  /** The first day's key — unique across the grid, and what an each block is keyed on. */
  key: string;
  /** Seven days, always. */
  days: ActivityDay[];
  /** Set only on the column that INTRODUCES a month, and null on every other — the caption
   *  rail reads this straight down the row of columns.
   *
   *  The month is the one the week ENDS in: a column straddling the 1st sits mostly in the
   *  new month, and labelling it with the old one puts the caption a full column left of what
   *  it names. A Date rather than a string, because the month's NAME depends on the reader's
   *  locale and this module does not know it — the model decides which column is labelled,
   *  the component decides what the label says. */
  monthStart: Date | null;
}

/** A drawable year. */
export interface ActivityGrid {
  /** Whole weeks, Monday first — every column is seven cells, as the calendar's rows are. */
  weeks: ActivityWeek[];
  /** The same cells, flat and in order. */
  days: ActivityDay[];
  /** Counting actions inside the window. */
  total: number;
  /** Consecutive active days ending today — or ending yesterday while today is still
   *  running. See `streaksFor`. */
  currentStreak: number;
  /** The longest run of consecutive active days anywhere in the window. */
  longestStreak: number;
}

function pad(n: number): string {
  return String(n).padStart(2, '0');
}

/** The reader's own calendar date for an instant. Local accessors, deliberately: the UTC ones
 *  would file a late-evening event on the previous day for anyone east of Greenwich and on the
 *  next one for anyone west of it.
 *
 *  calendarModel.ts holds its own copy of this rule, private to it. Two copies, because the
 *  alternative is a module of six lines that both import, and because each is held to it by
 *  its own pair of timezone suites (`*.test.ts` east, `*.west.test.ts` west) — a drift between
 *  them cannot be silent. Change one and look at the other. */
function dayKey(d: Date): string {
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
}

/** Adds whole days by calendar arithmetic rather than by milliseconds, so a daylight-saving
 *  boundary inside the window does not shift every later square by an hour — and eventually
 *  repeat or skip a date. */
function addDays(d: Date, days: number): Date {
  return new Date(d.getFullYear(), d.getMonth(), d.getDate() + days);
}

/** Monday-first weekday index, 0–6. */
function mondayIndex(d: Date): number {
  return (d.getDay() + 6) % 7;
}

/** The current and longest runs of consecutive active days.
 *
 *  The current run ends at today when today has actions, and at YESTERDAY when it does not.
 *  A naive "days ending today" resets at every midnight and tells somebody they have lost a
 *  run they are still in the middle of — the day is not over, and the figure exists to be
 *  encouraging about a true thing, not discouraging about a premature one. */
function streaksFor(days: ActivityDay[]): { current: number; longest: number } {
  let longest = 0;
  let run = 0;
  for (const day of days) {
    run = day.count > 0 ? run + 1 : 0;
    if (run > longest) longest = run;
  }

  // Walked backwards in place rather than over a reversed copy: `toReversed` is missing in
  // Safari before 16.4 and throws at module scope there, and copying 364 days to read the
  // tail of them is work for nothing.
  let current = 0;
  let i = days.length - 1;
  // Skip today when it is still empty; anything before that ends the run for real.
  if (days[i]?.count === 0) i -= 1;
  while ((days[i]?.count ?? 0) > 0) {
    current += 1;
    i -= 1;
  }

  return { current, longest };
}

/** buildActivityGrid turns a flat series of ledger events into the year the grid draws.
 *
 *  `events` may cover more than the window — the fetch deliberately asks for margin — and
 *  anything outside it is simply not placed. `today` is injected so the arithmetic is
 *  testable without freezing a clock. */
export function buildActivityGrid(events: TimelineEvent[], today: Date = new Date()): ActivityGrid {
  const todayStart = new Date(today.getFullYear(), today.getMonth(), today.getDate());
  const windowStart = addDays(todayStart, -(WINDOW_DAYS - 1));
  // Whole weeks: back to the Monday on or before the window's first day, forward to the
  // Sunday on or after today.
  const first = addDays(windowStart, -mondayIndex(windowStart));
  const last = addDays(todayStart, 6 - mondayIndex(todayStart));

  const byDay = new Map<string, TimelineEvent[]>();
  for (const event of events) {
    const key = dayKey(new Date(event.occurred_at));
    const bucket = byDay.get(key);
    if (bucket) bucket.push(event);
    else byDay.set(key, [event]);
  }
  // Sorted by instant rather than by string: Postgres keeps microseconds and Go trims
  // trailing zeros, so one second can hold both "09:00:00Z" and "09:00:00.482913Z" — and
  // lexically '.' sorts before 'Z', which would put the later one first.
  for (const bucket of byDay.values()) {
    bucket.sort((a, b) => Date.parse(a.occurred_at) - Date.parse(b.occurred_at));
  }

  const todayKey = dayKey(todayStart);
  const windowStartKey = dayKey(windowStart);

  const days: ActivityDay[] = [];
  for (let date = first; date <= last; date = addDays(date, 1)) {
    const key = dayKey(date);
    const inWindow = key >= windowStartKey && key <= todayKey;
    const dayEvents = inWindow ? (byDay.get(key) ?? []) : [];
    days.push({
      key,
      date,
      count: dayEvents.filter(countsAsCandidateAction).length,
      level: 0, // filled below, once the busiest day is known
      events: dayEvents,
      isToday: key === todayKey,
      inWindow,
    });
  }

  const inWindow = days.filter((d) => d.inWindow);
  const busiest = inWindow.reduce((max, d) => (d.count > max ? d.count : max), 0);
  for (const day of days) day.level = levelFor(day.count, busiest);

  const weeks: ActivityWeek[] = [];
  let labelledMonth = -1;
  for (let i = 0; i < days.length; i += 7) {
    const week = days.slice(i, i + 7);
    const weekStart = week[0];
    const weekEnd = week[week.length - 1];
    if (!weekStart || !weekEnd) continue; // unreachable: `days` is built a whole week at a time
    const month = weekEnd.date.getMonth();
    weeks.push({
      key: weekStart.key,
      days: week,
      monthStart: month === labelledMonth ? null : weekEnd.date,
    });
    labelledMonth = month;
  }

  const { current, longest } = streaksFor(inWindow);

  return {
    weeks,
    days,
    total: inWindow.reduce((n, d) => n + d.count, 0),
    currentStreak: current,
    longestStreak: longest,
  };
}

// Pure, $app-free logic behind the mentorship screens. The API calls live in api.ts —
// the one module that knows URLs and wire shapes — and the wire types in types.ts; what
// is here is the arithmetic those screens do on the answer, kept apart so it is testable
// without a browser. Mirrors the split companyFacetModel.ts holds for the company
// catalogue and matchAnalysis.ts for the analysis stream.

import type { Mentor, MentorAvailabilityRule, MentorSession, MentorSlot } from './types';

/** The mentor directory's whole vocabulary, one single-valued filter each.
 *
 *  Single-valued because the endpoint reads them with `query.Get`, not `QueryAll`
 *  (internal/api/handler/mentorship.go) — a second value is not an OR, it is discarded. */
export type MentorFilters = {
  company: string;
  topic: string;
  language: string;
};

/** The filter keys, in the order they are emitted. This list and `knownMentorParams` in
 *  internal/api/handler/mentorship.go are the same vocabulary written twice; a key added
 *  there and forgotten here is simply unreachable from the UI, which is the harmless
 *  direction, and one removed there and left here is forwarded to an endpoint that now
 *  reports it as ignored. */
const MENTOR_FILTER_KEYS = ['company', 'topic', 'language'] as const;

export function emptyMentorFilters(): MentorFilters {
  return { company: '', topic: '', language: '' };
}

/** Serialize to the query the directory endpoint is called with. An unset filter emits
 *  no key at all: `?company=` is not "no company", it is a company whose slug is empty. */
export function mentorFiltersToParams(f: MentorFilters): URLSearchParams {
  const p = new URLSearchParams();
  for (const key of MENTOR_FILTER_KEYS) {
    if (f[key]) p.set(key, f[key]);
  }
  return p;
}

/** The same serialization as a query STRING, for the controls to navigate with.
 *
 *  Components take this rather than building params themselves: a mutable
 *  `URLSearchParams` inside a `.svelte` file is not reactive state, and
 *  `svelte/prefer-svelte-reactivity` is right to say so. */
export function mentorFiltersToQuery(f: MentorFilters): string {
  return mentorFiltersToParams(f).toString();
}

/** Parse from URL params, keeping only what the directory reads.
 *
 *  The whitelist is the point: this URL is hand-edited, shared and crawled, and
 *  forwarding a key the endpoint does not read would widen the answer while the address
 *  bar still claims it was narrowed. The same rule `/companies` holds. */
export function mentorFiltersFromParams(p: URLSearchParams): MentorFilters {
  const f = emptyMentorFilters();
  for (const key of MENTOR_FILTER_KEYS) {
    f[key] = (p.get(key) ?? '').trim();
  }
  return f;
}

/** A company as the filter offers it: the slug the request carries, the name a person
 *  reads. Two spellings of one employer are already collapsed upstream by
 *  `normalize.CompanySlug` and the alias registry, so the slug is the identity here. */
// Not exported: it is read only through `MentorFilterOptions` below, and knip's gate
// covers exports as well as files — an exported name nothing imports is a finding here,
// not a courtesy.
type MentorCompanyOption = { slug: string; name: string };

export type MentorFilterOptions = {
  companies: MentorCompanyOption[];
  topics: string[];
  languages: string[];
};

/** Derive the filter controls' options from a directory listing.
 *
 *  Feed this the UNFILTERED directory. A narrowed list cannot offer the values it just
 *  excluded, so building the controls from what is on screen turns every filter into a
 *  one-way door: pick a topic and the other topics vanish along with the way back.
 *
 *  Sorted rather than left in arrival order — the endpoint promises no order, and options
 *  that reshuffle between reloads move the one a visitor was aiming at. */
export function mentorFilterOptions(mentors: Mentor[]): MentorFilterOptions {
  const companies = new Map<string, string>();
  const topics = new Set<string>();
  const languages = new Set<string>();

  for (const m of mentors) {
    if (m.company_slug) companies.set(m.company_slug, m.company_name || m.company_slug);
    for (const t of m.topics ?? []) topics.add(t);
    for (const l of m.languages ?? []) languages.add(l);
  }

  return {
    companies: [...companies]
      .map(([slug, name]) => ({ slug, name }))
      .sort((a, b) => a.name.localeCompare(b.name)),
    topics: [...topics].sort((a, b) => a.localeCompare(b)),
    languages: [...languages].sort((a, b) => a.localeCompare(b)),
  };
}

// ---- reading a slot ---------------------------------------------------------------
//
// Nothing below constructs a Date, and that is the whole point rather than an economy.
// `local_start` already carries the viewer's zone, resolved server-side against the zone
// database the backend ships. Parsing it into a Date re-interprets it against whatever
// zone this browser happens to be in, and formatting it back gives a different answer
// twice a year and across the date line — Tokyo's 16th at 01:00 is still the 15th in UTC,
// so a slot read that way is filed under the day before the one the mentor offered.
// The label is already correct; read it, do not recompute it.

/** The calendar day a slot belongs to, `YYYY-MM-DD`, in the viewer's zone. */
export function slotLocalDay(s: MentorSlot): string {
  return s.local_start.slice(0, 10);
}

/** The wall clock a person reads, `HH:MM`, in the viewer's zone. */
export function slotLocalTime(s: MentorSlot): string {
  return s.local_start.slice(11, 16);
}

/** Slots bucketed by their local day, days ascending and each day's slots left in the
 *  order the endpoint gave them (it promises ascending starts).
 *
 *  Two slots may share a wall-clock label and differ only in `utc_offset` — the hour the
 *  autumn transition repeats. Both are real hours the mentor stated, so both stay. */
export function groupSlotsByLocalDay(slots: MentorSlot[]): Map<string, MentorSlot[]> {
  const byDay = new Map<string, MentorSlot[]>();
  for (const s of slots) {
    const day = slotLocalDay(s);
    const bucket = byDay.get(day);
    if (bucket) bucket.push(s);
    else byDay.set(day, [s]);
  }
  return new Map([...byDay].sort(([a], [b]) => a.localeCompare(b)));
}

/** Which days the month grid may enable. Derived from the same grouping the day view
 *  reads, so a day can never be clickable in the grid and empty once opened. */
export function daysWithSlots(slots: MentorSlot[]): Set<string> {
  return new Set(groupSlotsByLocalDay(slots).keys());
}

// ---- the calendar grid ------------------------------------------------------------
//
// Also Date-free, for the same reason and one more: a grid built from Date's LOCAL
// accessors is drawn against the browser's zone, so a visitor a few hours either side of
// UTC can be shown a month whose first day sits under the wrong weekday. Using the UTC
// accessors would fix that and still leave the trap one careless edit away. Plain integer
// arithmetic has no zone to get wrong, and it is a dozen lines.

const pad2 = (n: number) => String(n).padStart(2, '0');

/** The `YYYY-MM` a `YYYY-MM-DD` belongs to. */
export function monthOf(day: string): string {
  return day.slice(0, 7);
}

/** Step a `YYYY-MM` by whole months, rolling the year. */
export function addMonths(month: string, delta: number): string {
  const year = Number(month.slice(0, 4));
  const index = Number(month.slice(5, 7)) - 1 + delta;
  return `${year + Math.floor(index / 12)}-${pad2(((index % 12) + 12) % 12 + 1)}`;
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
  const sunday0 =
    (y + Math.floor(y / 4) - Math.floor(y / 100) + Math.floor(y / 400) + offset + day) % 7;
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
function part(parts: Intl.DateTimeFormatPart[], type: string): string {
  return parts.find((p) => p.type === type)?.value ?? '';
}

/** The zone this browser believes it is in, falling back to UTC where `Intl` is absent —
 *  which is the SSR pass, since the server cannot know the viewer's zone at all.
 *
 *  Only ever a REQUEST. What the times are actually in is whatever the slot endpoint
 *  reports back: an unrecognised name is answered in UTC, and a page that assumed its own
 *  guess was honoured cannot tell a correct time from a wrong one. */
export function browserTimezone(): string {
  return typeof Intl === 'undefined' ? 'UTC' : Intl.DateTimeFormat().resolvedOptions().timeZone;
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

// ---- the mentor's own schedule ------------------------------------------------------
//
// Two conventions meet here and they disagree. A stored `weekday` is Go's `time.Weekday`,
// where 0 is SUNDAY; the calendar draws Monday first. Both are correct in their own place,
// so neither is "fixed" — they are mapped, once, below. Getting it backwards shifts a
// mentor's whole week by a day, which reads as though they typed it wrong.

const WEEKDAY_NAMES = [
  'Sunday',
  'Monday',
  'Tuesday',
  'Wednesday',
  'Thursday',
  'Friday',
  'Saturday',
];

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
  const dated = rules
    .filter((r) => r.date != null)
    .sort((a, b) => (a.date ?? '').localeCompare(b.date ?? ''));
  return { weekly, dated };
}

// ---- a booked session ---------------------------------------------------------------

/** An absolute instant read in a named zone: the day it falls on, the wall clock, and the
 *  offset that distinguishes two readings sharing a label.
 *
 *  This one DOES convert, unlike everything in the slot section above, and the difference
 *  is the input rather than a change of mind. A slot arrives with `local_start` already
 *  resolved in the viewer's zone, so converting it again moves it. A booking arrives with
 *  only `starts_at`, an instant ending in `Z`, which is unambiguous and has to be read in
 *  some zone before a person can act on it. */
export function formatInstantIn(
  instant: string,
  timezone: string,
): { day: string; time: string; offset: string } {
  const parts = new Intl.DateTimeFormat('en-US', {
    timeZone: timezone,
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
    timeZoneName: 'longOffset',
  }).formatToParts(new Date(instant));
  return {
    day: `${part(parts, 'year')}-${part(parts, 'month')}-${part(parts, 'day')}`,
    time: `${part(parts, 'hour')}:${part(parts, 'minute')}`,
    // `longOffset` spells it "GMT+09:00"; the sign and the digits are the part worth
    // showing, and UTC comes back as "+00:00" rather than blank so the label never
    // looks like a missing value.
    offset: part(parts, 'timeZoneName').replace('GMT', '') || '+00:00',
  };
}

/** Whether the cancel control should be offered at all.
 *
 *  The endpoint refuses a past or already-cancelled session, so this only decides whether
 *  somebody is shown a button that could return nothing but an error. The boundary is
 *  deliberate: the rule is "before its start", so the start instant itself is too late —
 *  that is the one moment where the control and the endpoint could otherwise disagree. */
export function isCancellable(s: MentorSession, now: Date = new Date()): boolean {
  return s.status === 'confirmed' && Date.parse(s.starts_at) > now.getTime();
}

/** Whether to offer the review form.
 *
 *  Only the SEEKER of a completed session may review, and the wire never states which
 *  party is reading. What it does state is that `seeker_email` reaches the MENTOR alone —
 *  a seeker does not need their own address back — so its ABSENCE is how the reader is
 *  identified. Indirect, and therefore tested rather than assumed.
 *
 *  `completed` already means "confirmed and ended", so a cancelled session can never be
 *  completed and the status does not need repeating here. */
export function canReview(s: MentorSession): boolean {
  return s.completed && !s.seeker_email;
}

/** The query string that results from setting some keys and clearing others.
 *
 *  Copies rather than edits: the caller's params are `page.url.searchParams`, and writing
 *  through that object edits the address bar's own state behind the router's back.
 *
 *  An empty string clears its key, same as null — `?date=` is not a chosen date, and a
 *  cleared control that leaves its key behind makes a link that looks filtered and is not.
 *
 *  It lives here rather than in the components partly because it is worth a test, and
 *  partly because `URLSearchParams` inside a `.svelte` file trips
 *  `svelte/prefer-svelte-reactivity` — the rule is right that a mutable built-in is not
 *  reactive state, and the fix it wants is for this not to be state at all. */
export function withSearchParams(
  current: URLSearchParams,
  next: Record<string, string | null>,
): string {
  const params = new URLSearchParams(current);
  for (const [key, value] of Object.entries(next)) {
    if (value) params.set(key, value);
    else params.delete(key);
  }
  return params.toString();
}

/** The instants to ask the slot endpoint for, to fill one month of the grid.
 *
 *  A day of padding on each side, because the endpoint takes INSTANTS and answers in the
 *  viewer's zone: a window cut exactly on UTC month boundaries loses the month's first
 *  local morning east of UTC and its last local evening west of it. A padded month is at
 *  most 33 days, well inside the endpoint's 62-day ceiling. */
export function slotWindowForMonth(month: string): { from: string; to: string } {
  const previous = addMonths(month, -1);
  return {
    from: `${previous}-${pad2(daysInMonth(previous))}T00:00:00Z`,
    to: `${addMonths(month, 1)}-01T00:00:00Z`,
  };
}

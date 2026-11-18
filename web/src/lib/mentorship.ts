// Pure, $app-free logic behind the mentorship screens. The API calls live in api.ts —
// the one module that knows URLs and wire shapes — and the wire types in types.ts; what
// is here is the arithmetic those screens do on the answer, kept apart so it is testable
// without a browser. Mirrors the split companyFacetModel.ts holds for the company
// catalogue and matchAnalysis.ts for the analysis stream.

import type {
  Mentor,
  MentorProfileInput,
  MentorProfileSuggestions,
  MentorSession,
  MentorSlot,
  OwnMentorProfile,
} from './types';
import { SENIORITY_VALUES } from './generated/contracts';
import { SENIORITY_LABELS, titleCase } from './labels';
import { addMonths, daysInMonth, monthOf, pad2, part } from './mentorshipCalendar';

/** A seniority code as a person reads it — reuses the platform's one seniority label
 *  map and its one title-cased fallback (the same pair `insights.ts`'s own
 *  `seniorityLabel` uses), falling back for a value the map has nothing special to say
 *  about (most of them: only `c_level` needs a label at all). An empty value (a mentor
 *  who left seniority unset) title-cases to '', which is the correct display: this
 *  capability has no "All levels" band the way `/insights` does. */
export function seniorityLabel(value: string): string {
  return SENIORITY_LABELS[value] ?? titleCase(value);
}

/** A mentor's company as a person reads it — the display name if the read joined one, the
 *  slug as a fallback, and "Independent" when neither is set. A mentor is not required to
 *  name a company at all (an independent consultant, or one whose employer isn't in the
 *  catalogue), and a blank space where a company would normally read is easy to mistake
 *  for a data gap rather than a deliberate choice. */
export function companyLabel(companyName: string, companySlug: string): string {
  return companyName || companySlug || 'Independent';
}

/** The mentor directory's whole vocabulary. Five single-valued string filters plus one
 *  flag (`noReviews`) — the flag is kept out of `MENTOR_FILTER_KEYS` below rather than
 *  forced into the same string shape, since "present" is its whole meaning and there is
 *  no value to carry.
 *
 *  The string filters are single-valued because the endpoint reads them with
 *  `query.Get`, not `QueryAll` (internal/api/handler/mentorship.go) — a second value is
 *  not an OR, it is discarded. */
export type MentorFilters = {
  company: string;
  topic: string;
  language: string;
  seniority: string;
  q: string;
  noReviews: boolean;
};

/** The single-valued filter keys, in the order they are emitted. This list (plus
 *  `noReviews`, handled separately below) and `knownMentorParams` in
 *  internal/api/handler/mentorship.go are the same vocabulary written twice; a key added
 *  there and forgotten here is simply unreachable from the UI, which is the harmless
 *  direction, and one removed there and left here is forwarded to an endpoint that now
 *  reports it as ignored. */
const MENTOR_FILTER_KEYS = ['company', 'topic', 'language', 'seniority', 'q'] as const;

export function emptyMentorFilters(): MentorFilters {
  return { company: '', topic: '', language: '', seniority: '', q: '', noReviews: false };
}

/** Serialize to the query the directory endpoint is called with. An unset filter emits
 *  no key at all: `?company=` is not "no company", it is a company whose slug is empty.
 *  `noReviews` follows the same rule in its own shape: false emits nothing, true emits
 *  `no_reviews=1` — there is no "explicitly false" to distinguish from "unset". */
export function mentorFiltersToParams(f: MentorFilters): URLSearchParams {
  const p = new URLSearchParams();
  for (const key of MENTOR_FILTER_KEYS) {
    if (f[key]) p.set(key, f[key]);
  }
  if (f.noReviews) p.set('no_reviews', '1');
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
  f.noReviews = p.get('no_reviews') === '1';
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
  seniorities: string[];
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
  const seniorities = new Set<string>();

  for (const m of mentors) {
    if (m.company_slug) companies.set(m.company_slug, m.company_name || m.company_slug);
    for (const t of m.topics ?? []) topics.add(t);
    for (const l of m.languages ?? []) languages.add(l);
    // Unlike topics/languages, seniority is a single optional field rather than an
    // array — a mentor who left it unset contributes nothing, not an empty-string option.
    if (m.seniority) seniorities.add(m.seniority);
  }

  return {
    companies: [...companies].map(([slug, name]) => ({ slug, name })).sort((a, b) => a.name.localeCompare(b.name)),
    topics: [...topics].sort((a, b) => a.localeCompare(b)),
    languages: [...languages].sort((a, b) => a.localeCompare(b)),
    // Career order (SENIORITY_VALUES), not alphabetical — alpha would scramble
    // "junior, middle, senior, lead" into a meaningless letter sort.
    seniorities: SENIORITY_VALUES.filter((s) => seniorities.has(s)),
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

/** The zone this browser believes it is in, falling back to UTC where `Intl` is absent —
 *  which is the SSR pass, since the server cannot know the viewer's zone at all.
 *
 *  Only ever a REQUEST. What the times are actually in is whatever the slot endpoint
 *  reports back: an unrecognised name is answered in UTC, and a page that assumed its own
 *  guess was honoured cannot tell a correct time from a wrong one. */
export function browserTimezone(): string {
  return typeof Intl === 'undefined' ? 'UTC' : Intl.DateTimeFormat().resolvedOptions().timeZone;
}

/** Seeds a blank create-form with the candidate's best-effort suggestions, one field at
 *  a time. A field the suggestions endpoint has nothing for keeps whatever `base` (the
 *  ordinary blank defaults) already set — the browser timezone included — rather than
 *  being cleared. This is a ONE-TIME starting point: called once against `blank()`
 *  before the mentor has typed anything, never merged into an in-progress or existing
 *  profile's form. */
export function seedFormFromSuggestions(
  base: MentorProfileInput,
  suggestions: MentorProfileSuggestions,
): MentorProfileInput {
  return {
    ...base,
    name: suggestions.name ?? base.name,
    headline: suggestions.headline ?? base.headline,
    bio: suggestions.bio ?? base.bio,
    timezone: suggestions.timezone ?? base.timezone,
    company_slug: suggestions.company_slug ?? base.company_slug,
    topics: suggestions.topics ?? base.topics,
    languages: suggestions.languages ?? base.languages,
  };
}

/** Renders the owner's own profile read back as the whole-object write body every save
 *  of it takes. Read back rather than defaulted, buffer/notice/horizon included, so a
 *  save that only touches one field (a headline correction from the profile form, a
 *  timezone change from the schedule page's session settings) never silently resets
 *  parameters that screen does not show — the same reasoning `blank()`'s hardcoded
 *  defaults exist to protect once a profile is created. */
export function profileInputFromProfile(p: OwnMentorProfile): MentorProfileInput {
  return {
    company_slug: p.company_slug,
    slug: p.slug,
    name: p.name,
    headline: p.headline,
    bio: p.bio,
    topics: p.topics,
    languages: p.languages,
    timezone: p.timezone,
    session_minutes: p.session_minutes,
    buffer_before_minutes: p.buffer_before_minutes,
    buffer_after_minutes: p.buffer_after_minutes,
    notice_minutes: p.notice_minutes,
    horizon_days: p.horizon_days,
    meeting_url: p.meeting_url ?? '',
    show_photo: p.show_photo,
    seniority: p.seniority ?? '',
  };
}

/** An absolute instant read in a named zone: the day it falls on, the wall clock, and the
 *  offset that distinguishes two readings sharing a label.
 *
 *  This one DOES convert, unlike everything in the slot section above, and the difference
 *  is the input rather than a change of mind. A slot arrives with `local_start` already
 *  resolved in the viewer's zone, so converting it again moves it. A booking arrives with
 *  only `starts_at`, an instant ending in `Z`, which is unambiguous and has to be read in
 *  some zone before a person can act on it. */
export function formatInstantIn(instant: string, timezone: string): { day: string; time: string; offset: string } {
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
export function withSearchParams(current: URLSearchParams, next: Record<string, string | null>): string {
  const params = new URLSearchParams(current);
  for (const [key, value] of Object.entries(next)) {
    if (value) params.set(key, value);
    else params.delete(key);
  }
  return params.toString();
}

/** Search forward from today's UTC boundary within the endpoint's 62-day ceiling.
 *  Shared bounds preserve the server cache; the slot engine applies the notice period.
 *  Calendar-month boundaries can miss hours: 30 days from late January can reach March. */
export function openingSlotWindow(now = new Date()): { from: string; to: string } {
  const from = new Date(now);
  from.setUTCHours(0, 0, 0, 0);
  return {
    from: from.toISOString(),
    to: new Date(from.getTime() + 62 * 24 * 60 * 60 * 1000).toISOString(),
  };
}

/** The month a calendar should open on: `current` while it has an open hour, otherwise the
 *  month of the earliest open hour after it, otherwise `current` (nothing is open at all).
 *  Slots before `current` — the padding day — never move it backwards. */
export function firstOpenMonth(slots: MentorSlot[], current: string): string {
  const months = slots.map((s) => monthOf(slotLocalDay(s))).filter((m) => m >= current);
  if (months.length === 0 || months.includes(current)) return current;
  return months.reduce((a, b) => (b < a ? b : a));
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

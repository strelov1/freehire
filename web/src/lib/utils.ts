/** User-facing message for a caught value: the Error's message, else the fallback. */
export function errorMessage(e: unknown, fallback: string): string {
  return e instanceof Error ? e.message : fallback;
}

/** Narrows a possibly-null value, throwing if it is missing instead of silently
 *  trusting it the way a bare `!` would. Chiefly for tests. */
export function must<T>(value: T | null | undefined, what = 'value'): T {
  if (value == null) throw new Error(`expected ${what} to be non-null`);
  return value;
}

/** Exhaustiveness check for a `switch`/`if` over a closed union: put this in the
 *  `default`/final branch so an unhandled member fails to compile instead of
 *  silently falling through. Do not use where the union is meant to stay open
 *  (e.g. forward-compat with server-sent values) — a thrown error there is the
 *  wrong failure mode. */
export function assertNever(x: never): never {
  throw new Error(`Unreachable case: ${JSON.stringify(x)}`);
}

/** The Date behind an RFC3339 string, or null when there is nothing to format —
 *  missing and unparseable are one case here, because every formatter below answers
 *  both with ''. Nothing else in this file needs to tell them apart. */
function parseTs(ts: string | null | undefined): Date | null {
  if (!ts) return null;
  const d = new Date(ts);
  return Number.isNaN(d.getTime()) ? null : d;
}

// Shared so the date a reader sees and the one in the tooltip behind it cannot come
// to disagree about how a month is spelled.
const DATE_PARTS = { year: 'numeric', month: 'short', day: 'numeric' } as const;

/** Format an RFC3339 timestamp as a short local date; '' for null/invalid. */
export function formatDate(ts: string | null | undefined): string {
  const d = parseTs(ts);
  return d ? d.toLocaleDateString(undefined, DATE_PARTS) : '';
}

/** The same instant with the clock time, for a `title` behind a formatDate or
 *  formatDateOrAgo label: the visible line stays short, and a reader who cares about
 *  the hour gets it on hover rather than in a second column. '' for null/invalid. */
export function formatDateTime(ts: string | null | undefined): string {
  const d = parseTs(ts);
  return d ? d.toLocaleString(undefined, { ...DATE_PARTS, hour: '2-digit', minute: '2-digit' }) : '';
}

/** Whether s is a LinkedIn personal-profile URL: an http(s) link on linkedin.com (or a
 *  country/www subdomain) whose path is /in/<handle>. Mirrors the backend's shape check —
 *  the server re-validates on submit, so this is just for inline form feedback. */
export function isLinkedInUrl(s: string): boolean {
  let u: URL;
  try {
    u = new URL(s.trim());
  } catch {
    return false;
  }
  if (u.protocol !== 'http:' && u.protocol !== 'https:') return false;
  const host = u.hostname.toLowerCase();
  if (host !== 'linkedin.com' && !host.endsWith('.linkedin.com')) return false;
  return /^\/in\/[^/]+/.test(u.pathname);
}

/** Compact count formatting: 3354251 → "3.4M", 697191 → "697K", 842 → "842". Full
 *  precision is left to whatever sits behind the label — a chart's tooltip, or the
 *  job's own page. Shared by the activity chart's axis and the job card's view
 *  count, which is why it lives here and not in either of them. */
export function formatCount(n: number): string {
  const abs = Math.abs(n);
  if (abs >= 1e6) return trimZero((n / 1e6).toFixed(1)) + 'M';
  if (abs >= 1e3) return trimZero((n / 1e3).toFixed(abs >= 1e5 ? 0 : 1)) + 'K';
  return String(n);
}

function trimZero(s: string): string {
  return s.replace(/\.0$/, '');
}

const TIME_UNITS: [Intl.RelativeTimeFormatUnit, number][] = [
  ['year', 31536000],
  ['month', 2592000],
  ['week', 604800],
  ['day', 86400],
  ['hour', 3600],
  ['minute', 60],
  ['second', 1],
];

/** How much of the unit gets spelled out. `short` is for a label something else has
 *  already named — an icon, a column heading — where "8 min. ago" says everything
 *  "8 minutes ago" did in half the room. Abbreviating is `Intl`'s job and not ours: a
 *  hand-written "min" is correct in two languages and wrong in the rest. */
export type TimeAgoStyle = 'long' | 'short';

// Built once each: constructing an Intl formatter resolves a locale and loads its
// data, which dwarfs formatting with one — and the feed calls timeAgo per job
// card. The locale is the runtime default, which cannot change within a process
// or a browser session, so one instance per style is safe to share.
const relativeTime: Partial<Record<TimeAgoStyle, Intl.RelativeTimeFormat>> = {};

/** Format an RFC3339 timestamp as a relative "N ago" label (e.g. "13 seconds
 *  ago", "2 days ago"); '' for null/invalid. How recently a job was posted is a
 *  key signal, so the list card shows it relative rather than as a bare date. */
export function timeAgo(ts: string | null | undefined, style: TimeAgoStyle = 'long'): string {
  const d = parseTs(ts);
  if (!d) return '';
  const seconds = Math.round((Date.now() - d.getTime()) / 1000);
  const rtf = (relativeTime[style] ??= new Intl.RelativeTimeFormat(undefined, {
    numeric: 'auto',
    style,
  }));
  for (const [unit, span] of TIME_UNITS) {
    if (Math.abs(seconds) >= span || unit === 'second') {
      return rtf.format(-Math.round(seconds / span), unit);
    }
  }
  return '';
}

// A day. Inside it "20 minutes ago" is the answer a reader wants and a date cannot
// give; past it the relative form starts losing to the date it replaced — "3 days
// ago" is what a reader has to convert back before comparing two postings.
const RECENT_MS = 86400 * 1000;

/** timeAgo while the instant is still within the last day, formatDate once it is
 *  not; '' for null/invalid, like both. A timestamp in the FUTURE takes the date
 *  branch: "in 2 hours" on a posting date is a clock-skew artefact, and reading it
 *  as an age would be worse than reading it as a date.
 *
 *  `style` reaches only the relative branch. Past the day boundary there is nothing to
 *  abbreviate, and a date a reader is comparing against another posting's must not be
 *  shortened out from under them. */
export function formatDateOrAgo(ts: string | null | undefined, style: TimeAgoStyle = 'long'): string {
  const d = parseTs(ts);
  if (!d) return '';
  const age = Date.now() - d.getTime();
  return age >= 0 && age < RECENT_MS ? timeAgo(ts, style) : formatDate(ts);
}

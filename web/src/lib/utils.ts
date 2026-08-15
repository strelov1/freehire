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

/** Format an RFC3339 timestamp as a short local date; '' for null/invalid. */
export function formatDate(ts: string | null | undefined): string {
  if (!ts) return '';
  const d = new Date(ts);
  if (Number.isNaN(d.getTime())) return '';
  return d.toLocaleDateString(undefined, { year: 'numeric', month: 'short', day: 'numeric' });
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

const TIME_UNITS: [Intl.RelativeTimeFormatUnit, number][] = [
  ['year', 31536000],
  ['month', 2592000],
  ['week', 604800],
  ['day', 86400],
  ['hour', 3600],
  ['minute', 60],
  ['second', 1],
];

// Built once, not per call. Constructing an Intl formatter resolves a locale and
// loads its data, which dwarfs the cost of formatting with one — 12x on a
// microbenchmark of the homepage's 32 cards (0.30ms -> 0.03ms per render). That
// is ~1% of a 29ms render, so this is tidiness rather than a fix for anything:
// the profile that surfaced it (timeAgo was second among our own functions) also
// showed the render cost is spread across the page, not concentrated anywhere.
// The locale is `undefined` — the runtime default — and it does not change within
// a process or a browser session, so one instance is safe to reuse.
let relativeTime: Intl.RelativeTimeFormat | undefined;

/** Format an RFC3339 timestamp as a relative "N ago" label (e.g. "13 seconds
 *  ago", "2 days ago"); '' for null/invalid. How recently a job was posted is a
 *  key signal, so the list card shows it relative rather than as a bare date. */
export function timeAgo(ts: string | null | undefined): string {
  if (!ts) return '';
  const d = new Date(ts);
  if (Number.isNaN(d.getTime())) return '';
  const seconds = Math.round((Date.now() - d.getTime()) / 1000);
  const rtf = (relativeTime ??= new Intl.RelativeTimeFormat(undefined, { numeric: 'auto' }));
  for (const [unit, span] of TIME_UNITS) {
    if (Math.abs(seconds) >= span || unit === 'second') {
      return rtf.format(-Math.round(seconds / span), unit);
    }
  }
  return '';
}

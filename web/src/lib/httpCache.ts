// Cache-Control for server-rendered HTML.
//
// The catalogue is enormous and mostly anonymous — company and collection pages
// exist to be found in search — so the same HTML is rendered over and over for
// visitors and crawlers alike, each render costing a search query and a database
// round trip on a box that is also running the ingest. A shared cache in front of
// the app answers those repeats without touching the origin.
//
// The rule that matters is the negative one: a signed-in response carries that
// person's saved jobs, applications and header state. Stored under the plain URL
// by a CDN, it would be served to whoever asked next. That decision lives here, in
// the app, rather than only in a CDN rule — a misconfigured rule is then a missed
// optimisation instead of a data leak.

/** Anonymous, public HTML. `max-age=0` keeps the BROWSER revalidating (someone who
 *  signs in must not be handed their own stale anonymous copy from disk cache),
 *  while `s-maxage` lets a shared cache serve it for five minutes and
 *  `stale-while-revalidate` lets it keep serving during the refresh. */
export const PUBLIC_CACHE = 'public, max-age=0, s-maxage=300, stale-while-revalidate=86400';

/** A page about ONE entity, held a day. Same two guarantees as `PUBLIC_CACHE` —
 *  the browser still revalidates, the edge may still serve while it refreshes —
 *  and only the shared-cache lifetime differs.
 *
 *  The day is not a guess about how fresh a job page can be; it is bounded by how
 *  fresh the catalogue itself is. The fastest thing that takes a posting down is the
 *  ingest sweep, which closes a job unseen for **48 hours**; the liveness probe runs
 *  twice a day and needs two consecutive `expired` reads; the age rule waits 45 days
 *  (docs/agents/job-lifecycle.md). Staying at half that floor keeps the page no
 *  staler than the data behind it, and `stale-while-revalidate=86400` had already
 *  accepted a day of staleness anyway.
 *
 *  What this buys is the revalidation count, not latency. `stale-while-revalidate`
 *  means the edge already answers instantly past expiry, so `s-maxage` only controls
 *  how often a background refresh lands on the origin. That is the whole point here:
 *  the catalogue is long-tail — hundreds of thousands of job and company pages, most
 *  seen a handful of times a day — so entity pages are the overwhelming majority of
 *  URLs and the ones whose misses reach the origin from Helsinki. It was an hour;
 *  measured field TTFB is 1733ms at p75 on phones against a 65ms render, which is
 *  network and origin round trips, not work. */
export const PUBLIC_DETAIL_CACHE = 'public, max-age=0, s-maxage=86400, stale-while-revalidate=86400';

/** Anything tied to a person. `no-store` is deliberate over `no-cache`: nothing
 *  should be written down at all, not even revalidated. */
export const PRIVATE_CACHE = 'private, no-store';

/** A response that failed. Not `private` — there is nothing personal about a 500,
 *  and saying so would be misleading; it simply must not be written down, because
 *  what it describes is a moment rather than a page.
 *
 *  This exists because the opposite was tried by omission. An error page is HTML,
 *  so it collected the same policy as the page it replaced, and Cloudflare stored a
 *  500 under the URL and served it as a HIT for the next hour — `s-maxage=3600` on
 *  an entity page, with `stale-while-revalidate=86400` extending it up to a day.
 *  A blue/green flip that 500s for a few seconds therefore outlives itself by
 *  hours, on pages the origin is already serving correctly again. */
export const NO_CACHE = 'no-store';

/** Route trees that are never shared, whoever asks. An anonymous hit here is a
 *  sign-in screen or a redirect, which is not worth handing to the next visitor. */
const PRIVATE_ROOTS = new Set(['my', 'auth', 'moderation', 'delete-account']);

/** Route families whose SECOND segment names one entity. Only `/<root>/<slug>`
 *  qualifies, never a deeper child: `/jobs/<slug>/discussion` carries comments
 *  people expect to appear promptly, and the index above them (`/jobs`,
 *  `/companies`) is reordered by every ingest run. Both stay on the short
 *  lifetime. */
const DETAIL_ROOTS = new Set(['jobs', 'companies', 'blog']);

/** The Cache-Control an HTML response should carry. */
export function cachePolicy({
  pathname,
  authenticated,
  status = 200,
}: {
  pathname: string;
  authenticated: boolean;
  /** The response's status. Absent means "nothing went wrong" — the parameter was
   *  added after the fact, and a caller that doesn't pass one is describing a page,
   *  not a failure. */
  status?: number;
}): string {
  // The signed-in guard outranks everything, including the error rule: a 500
  // rendered for a signed-in visitor still carries their header state, so it is
  // private first and uncacheable second. NO_CACHE would be the weaker claim.
  if (authenticated) return PRIVATE_CACHE;
  // Only 5xx. A 404 is a stable fact about the URL — closed postings are a routine
  // and permanent outcome here, in large numbers, and letting the edge absorb those
  // repeats is exactly what this cache is for. A 5xx describes a moment.
  if (status >= 500) return NO_CACHE;
  // Compare whole segments: /myths is not under /my.
  const segments = pathname.split('/').filter(Boolean);
  const [root] = segments;
  if (root && PRIVATE_ROOTS.has(root)) return PRIVATE_CACHE;
  if (segments.length === 2 && root && DETAIL_ROOTS.has(root)) return PUBLIC_DETAIL_CACHE;
  return PUBLIC_CACHE;
}

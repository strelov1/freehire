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

/** A page about ONE entity, held twelve times longer. Same two guarantees as
 *  `PUBLIC_CACHE` — the browser still revalidates, the edge may still serve while
 *  it refreshes — and only the shared-cache lifetime differs.
 *
 *  The hour is not a guess about how fresh a job page can be; it is bounded by how
 *  fresh the catalogue itself is. The fastest thing that takes a posting down is the
 *  ingest sweep, which closes a job unseen for **48 hours**; the liveness probe runs
 *  twice a day and needs two consecutive `expired` reads; the age rule waits 45 days
 *  (docs/agents/job-lifecycle.md). Against a floor measured in days, an hour is
 *  noise — and `stale-while-revalidate=86400` already accepted up to a day of it.
 *
 *  What the hour actually buys is the revalidation count. `stale-while-revalidate`
 *  means the edge already answers instantly past expiry, so `s-maxage` was never
 *  controlling latency — it controls how often a background refresh lands on the
 *  origin. Entity pages are the overwhelming majority of URLs, so this is where
 *  twelve-fold matters. */
export const PUBLIC_DETAIL_CACHE = 'public, max-age=0, s-maxage=3600, stale-while-revalidate=86400';

/** Anything tied to a person. `no-store` is deliberate over `no-cache`: nothing
 *  should be written down at all, not even revalidated. */
export const PRIVATE_CACHE = 'private, no-store';

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
}: {
  pathname: string;
  authenticated: boolean;
}): string {
  if (authenticated) return PRIVATE_CACHE;
  // Compare whole segments: /myths is not under /my.
  const segments = pathname.split('/').filter(Boolean);
  const [root] = segments;
  if (root && PRIVATE_ROOTS.has(root)) return PRIVATE_CACHE;
  if (segments.length === 2 && root && DETAIL_ROOTS.has(root)) return PUBLIC_DETAIL_CACHE;
  return PUBLIC_CACHE;
}

// Which URLs at `/` were written when `/` was the job feed.
//
// Extracted from the homepage loader so it can be tested without a SvelteKit runtime:
// this predicate decides whether a visitor's filters survive the move, and the failure
// it guards against is silent.

/** The only params that reach `/` meaning something other than "filter the feed".
 *
 *  Three of them the app itself reads on this route: every guarded page bounces a
 *  signed-out visitor here with `?auth=required&redirect=…`, and a failed OAuth
 *  callback arrives as `?auth_error` (TopBar's afterNavigate). `ref` belongs to the
 *  referral links. Nothing else in web/src reads a query parameter on the homepage. */
const LANDING_PARAMS = new Set(['auth', 'auth_error', 'redirect', 'ref']);

/** Campaign tails, which nothing reads at all — they are for whatever is measuring the
 *  visit. Matched by prefix because the family is open-ended. */
const TRACKING_PREFIXES = ['utm_', 'gclid', 'fbclid', 'mc_'];

/** Was this URL written when `/` was the job feed?
 *
 *  Asked the safe way round: the landing is served only when NOTHING in the URL could
 *  be a filter. The obvious predicate — "does it serialise to feed params?" — is the
 *  dangerous one, because the loader this replaced forwarded `url.searchParams`
 *  VERBATIM to the search API, so `/` honoured every parameter that API accepts, and
 *  several of those (`is_tech`, `salary_max`, `experience_years_min`,
 *  `education_level`, `order`) the browser's filter model does not serialise. Under
 *  that predicate `/?is_tech=true` would have rendered the landing page with the
 *  visitor's filter silently discarded — and silence is the whole problem: they cannot
 *  tell a dropped filter from a catalogue that has nothing.
 *
 *  Inverted, the worst case is a campaign parameter nobody listed sending a visitor to
 *  the feed instead of the landing. That is a different page, not a lost query, and it
 *  is visible the moment it happens. */
export function isLegacyFeedUrl(params: URLSearchParams): boolean {
  for (const key of params.keys()) {
    if (LANDING_PARAMS.has(key)) continue;
    if (TRACKING_PREFIXES.some((prefix) => key.startsWith(prefix))) continue;
    return true;
  }
  return false;
}

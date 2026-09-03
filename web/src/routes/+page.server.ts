import { redirect } from '@sveltejs/kit';
import { serverApi } from '$lib/server/api';
import { filtersFromParams, filtersToParams } from '$lib/facetModel';
import type { PageServerLoad } from './$types';

/** Params that address the feed without filtering it. A bare `/?page=2` or
 *  `/?sort=recent` is a feed URL even though it names no facet, so the redirect below
 *  has to recognise them too. */
const FEED_ONLY_PARAMS = ['page', 'sort'];

/** Was this URL written when `/` was the job feed?
 *
 *  Answered by parsing, not by "does it carry any params at all": `/` is also reached
 *  with `?auth=required&redirect=…` (every guarded page bounces a signed-out visitor
 *  here), with `?ref=` from the referral links, and with the usual utm_* tail.
 *  Redirecting those would drop someone into a job list instead of the sign-in dialog
 *  they were sent for. `filtersToParams(filtersFromParams(…))` keeps only what the feed
 *  actually reads, so everything else leaves the landing page alone. */
function isLegacyFeedUrl(params: URLSearchParams): boolean {
  if (FEED_ONLY_PARAMS.some((p) => params.has(p))) return true;
  return [...filtersToParams(filtersFromParams(params))].length > 0;
}

// The homepage is the landing page: one search box, and under it the shortcuts into
// the catalogue that are worth naming out loud. The feed moved to /jobs.
//
// The page renders without either call — `allSettled`, not `all`. Its job is to put
// the search box on screen, and a stats endpoint being cold or down must cost it its
// chips and its counters, never the box. Both are aggregate, unauthenticated and
// cached server-side, so this stays a cheap SSR on the site's busiest URL.
export const load: PageServerLoad = async ({ url, fetch }) => {
  // 301, query string carried through verbatim, so /?q=go&remote=true still lands on
  // the same filtered feed.
  if (isLegacyFeedUrl(url.searchParams)) redirect(301, `/jobs${url.search}`);

  const api = serverApi(fetch);
  // The same call the search box makes for itself on a listless page — unfiltered,
  // category only. Asking here means the busiest page in the site serves its chips
  // from the SSR it already pays for, and hands the box the distribution instead of
  // making it fetch the identical thing again on first focus.
  const [counts, scale] = await Promise.allSettled([
    api.facetCounts(new URLSearchParams(), { facets: ['category'] }),
    api.catalogScale(),
  ]);

  return {
    counts: counts.status === 'fulfilled' ? counts.value : null,
    scale: scale.status === 'fulfilled' ? scale.value : null,
  };
};

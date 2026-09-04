import { redirect } from '@sveltejs/kit';
import { serverApi } from '$lib/server/api';
import { isLegacyFeedUrl } from '$lib/legacyFeedUrl';
import type { PageServerLoad } from './$types';

// The homepage is the landing page: one search box, and under it the shortcuts into
// the catalogue that are worth naming out loud. The feed moved to /jobs.
//
// The page renders without either call — `allSettled`, not `all`. Its job is to put
// the search box on screen, and a stats endpoint being cold or down must cost it its
// chips and its counters, never the box. Both are aggregate, unauthenticated and
// cached server-side, so this stays a cheap SSR on the site's busiest URL.
export const load: PageServerLoad = async ({ url, fetch }) => {
  // 301, query string carried through verbatim, so /?q=go&work_mode=remote still lands
  // on the same filtered feed.
  if (isLegacyFeedUrl(url.searchParams)) redirect(301, `/jobs${url.search}`);

  const api = serverApi(fetch);
  // Near enough the call the search box makes for itself on a listless page —
  // unfiltered, and `category` is the one distribution its empty dropdown reads.
  // Asking here means the busiest page in the site serves its chips from the SSR it
  // already pays for, and hands the box the distribution instead of making it fetch
  // the same thing again on first focus. `countries` rides along for the second row of
  // chips: a job hunt is a place as often as it is a craft.
  const [counts, scale] = await Promise.allSettled([
    api.facetCounts(new URLSearchParams(), { facets: ['category', 'countries'] }),
    api.catalogScale(),
  ]);

  return {
    counts: counts.status === 'fulfilled' ? counts.value : null,
    scale: scale.status === 'fulfilled' ? scale.value : null,
  };
};

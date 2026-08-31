import { landingCategories } from '$lib/roleLandings';
import { serverApi } from '$lib/server/api';
import type { PageServerLoad } from './$types';

// The hub. Every published category with its open-job count, read off ONE unfiltered
// category distribution — not one call per category. The /collections hub does the
// latter and now fires ~67 backend calls per render; the shape is avoided here rather
// than inherited.
export const load: PageServerLoad = async ({ fetch, setHeaders }) => {
  const counts = await serverApi(fetch).facetCounts(new URLSearchParams(), {
    facets: ['category'],
  });

  const byCategory = counts.facets.category ?? {};
  const categories = landingCategories()
    .map((c) => ({ ...c, openCount: byCategory[c.category] ?? 0 }))
    .filter((c) => c.openCount > 0)
    .sort((a, b) => b.openCount - a.openCount);

  setHeaders({ 'cache-control': 'public, max-age=0, s-maxage=3600' });

  return { categories, total: counts.total };
};

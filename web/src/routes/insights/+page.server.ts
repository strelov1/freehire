import { serverApi } from '$lib/server/api';
import { coveredCategories } from '$lib/insights';
import type { PageServerLoad } from './$types';

// The insights hub: the entry point that links every covered category's three
// pages, so crawlers reach them from one indexable page. Covered set is derived
// from the global roles ranking (the gate). SSR-live + CDN cache window.
export const load: PageServerLoad = async ({ fetch, setHeaders }) => {
  const globalRoles = await serverApi(fetch).insightsRoles({ limit: 200 });
  setHeaders({ 'cache-control': 'public, max-age=0, s-maxage=3600' });
  return { covered: coveredCategories(globalRoles) };
};

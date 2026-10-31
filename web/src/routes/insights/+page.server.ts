import { loadInsightsGate } from '#lib/server/insights.js';
import { serverApi } from '#lib/server/api.js';
import { coveredCategories } from '#lib/insights.js';
import type { PageServerLoad } from './$types';

// The insights hub: the entry point that links every covered category's three
// pages, so crawlers reach them from one indexable page. Covered set is derived
// from the global roles ranking (the gate). SSR-live + CDN cache window.
//
// The role × country grid is a second read in parallel. It is the hub's lead block but
// not its reason to exist, so a failed read drops the grid (null → not rendered)
// rather than failing the page that links every category.
export const load: PageServerLoad = async ({ fetch, setHeaders }) => {
  const [globalRoles, grid] = await Promise.all([
    loadInsightsGate(fetch),
    serverApi(fetch).insightsRoleCountries().catch(() => null),
  ]);
  // A page cached without its grid would stay that way for every visitor for the full
  // hour, so a failed read caches for a minute instead.
  setHeaders({ 'cache-control': `public, max-age=0, s-maxage=${grid ? 3600 : 60}` });
  return { covered: coveredCategories(globalRoles), grid };
};

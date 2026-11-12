import { serverApi } from '#lib/server/api.js';
import { salaryScopeFromParams } from '#lib/insights.js';
import type { PageServerLoad } from './$types';

// Server-render the facet-distribution counts for the current URL filters, so the
// breakdowns are in the initial HTML (indexable, usable without client JS for the
// first paint). The URL query carries the same filter params the drill-down and
// the search API read; the client re-fetches on every filter change. The salary
// bands come from the role × grade × country rollup the same filters select.
export const load: PageServerLoad = async ({ url, fetch }) => {
  const api = serverApi(fetch);
  const sal = salaryScopeFromParams(url.searchParams);
  const [initial, initialSalary] = await Promise.all([
    api.facetCounts(url.searchParams),
    sal ? api.insightsSalary(sal.scope).catch(() => null) : Promise.resolve(null),
  ]);
  return { initial, initialSalary };
};

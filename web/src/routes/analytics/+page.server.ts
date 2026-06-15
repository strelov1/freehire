import { serverApi } from '$lib/server/api';
import type { PageServerLoad } from './$types';

// Server-render the facet-distribution counts for the current URL filters, so the
// breakdowns are in the initial HTML (indexable, usable without client JS for the
// first paint). The URL query carries the same filter params the drill-down and
// the search API read; the client re-fetches on every filter change.
export const load: PageServerLoad = async ({ url, fetch }) => {
  const initial = await serverApi(fetch).facetCounts(url.searchParams);
  return { initial };
};

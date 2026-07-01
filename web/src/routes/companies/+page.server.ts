import { serverApi } from '$lib/server/api';
import { companyFiltersFromParams, companyFiltersToParams } from '$lib/companyFilters';
import type { PageServerLoad } from './$types';

const LIMIT = 20;

// Server-render the first page of companies for the current filters (the ?q search
// plus the sidebar facets), so a shared/deep-linked filtered URL renders filtered
// in the initial HTML. Round-tripping through companyFilters whitelists the params
// to the known facets. The debounced search and "load more" then run client-side.
export const load: PageServerLoad = async ({ url, fetch }) => {
  const facets = companyFiltersToParams(companyFiltersFromParams(url.searchParams));
  const initial = await serverApi(fetch).listCompanies('', LIMIT, 0, facets);
  return { initial };
};

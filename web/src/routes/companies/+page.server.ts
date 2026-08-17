import { serverApi } from '$lib/server/api';
import { companyFiltersFromParams, companyFiltersToParams } from '$lib/companyFilters';
import { PAGE_SIZE, pageOffset, parsePage } from '$lib/pagination';
import type { PageServerLoad } from './$types';

// Server-render the requested page of companies for the current filters (the ?q
// search plus the sidebar facets), so a shared/deep-linked filtered URL renders
// filtered in the initial HTML. Round-tripping through companyFilters whitelists
// the params to the known facets; the debounced search then runs client-side.
//
// `?page=N` addresses the list rather than filtering it, so it is read here and
// never reaches the facets. `parsePage` folds every malformed or out-of-range value
// to 1: this URL is hand-edited and crawled, and a bad one has to mean the first
// page rather than a negative offset. Mirrors the job feed's own `load`.
export const load: PageServerLoad = async ({ url, fetch }) => {
  const facets = companyFiltersToParams(companyFiltersFromParams(url.searchParams));
  const pageNumber = parsePage(url.searchParams);
  const initial = await serverApi(fetch).listCompanies('', PAGE_SIZE, pageOffset(pageNumber), facets);
  return { initial, pageNumber };
};

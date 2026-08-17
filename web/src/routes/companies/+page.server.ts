import { error } from '@sveltejs/kit';
import { serverApi } from '$lib/server/api';
import { companyFiltersFromParams, companyFiltersToParams } from '$lib/companyFilters';
import { PAGE_SIZE, pageExists, pageOffset, parsePage } from '$lib/pagination';
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
  // Same rule the job feed and the collections hold: past the last page the matches
  // fill there is no page, only an empty list under a canonical of its own — the
  // shape Google reads as a soft 404. This listing gained `?page=N` after that rule
  // was written, so it was the one route accepting a page number without checking
  // that the number addressed anything. Page 1 always stands: a filter combination
  // nothing matches renders the list's own empty state, not the site's 404.
  if (!pageExists(pageNumber, initial.total)) error(404, 'Page not found');
  return { initial, pageNumber };
};

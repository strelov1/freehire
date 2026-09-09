import { error } from '@sveltejs/kit';
import { serverApi } from '$lib/server/api';
import { talentFiltersFromParams, talentFiltersToParams } from '$lib/talentFacetModel';
import { PAGE_SIZE, pageExists, pageOffset, parsePage } from '$lib/pagination';
import type { PageServerLoad } from './$types';

// Server-render the requested page of the Talent Network catalogue for the current
// filters, so a shared or crawled URL arrives already filtered in the initial HTML.
// Round-tripping through the filter model whitelists the params to the known facets; the
// client takes over from there (see TalentView for why that hand-off exists at all).
//
// `?page=N` addresses the list rather than filtering it, so it is read here and not
// through the filter model — the same split /companies and /jobs use.
//
// Public read: no cookie forwarded.
export const load: PageServerLoad = async ({ fetch, url }) => {
  const currentPage = parsePage(url.searchParams);

  const params = talentFiltersToParams(talentFiltersFromParams(url.searchParams));
  params.set('limit', String(PAGE_SIZE));
  params.set('offset', String(pageOffset(currentPage)));

  const slice = await serverApi(fetch).listTalent(params.toString());

  // A page past the end is a 404 rather than an empty list: a URL naming page nine of a
  // three-page catalogue is wrong, and rendering it empty would tell a visitor the
  // filters matched nobody when what happened is that they walked off the end.
  if (!pageExists(currentPage, slice.total)) error(404, 'Page not found');

  return { page: slice, currentPage };
};

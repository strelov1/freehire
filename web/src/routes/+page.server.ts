import { serverApi } from '$lib/server/api';
import { FILTERS_TOUCHED_COOKIE, DEFAULT_JOB_FILTERS } from '$lib/filterStorage';
import { defaultFilterTarget, isCrawler } from '$lib/firstVisit';
import { pageOffset, parsePage } from '$lib/pagination';
import type { PageServerLoad } from './$types';

const LIMIT = 20;

// The homepage IS the job feed. Server-render the first page of search results
// for the current URL filters, so the job rows are in the initial HTML. The URL
// query already carries the search params (q, facets, sort) in the shape the
// search API reads; `searchJobs` adds pagination. Filters/sort/"load more" then
// run client-side over the same client.
export const load: PageServerLoad = async ({ url, fetch, cookies, request }) => {
  // First-visit default: a brand-new human on a bare homepage gets the
  // remote-worldwide slice — applied implicitly (search with its params, seed
  // the client's filter state with the same string below) rather than via a
  // redirect, so there's no extra round trip before the page can render.
  // defaultFilterTarget owns which requests are exempt (shared links, returning
  // visitors, crawlers) — a non-null result just means "use the default set";
  // its `/?...` path isn't needed since the address bar never changes.
  const useDefault =
    defaultFilterTarget({
      search: url.search,
      touched: !!cookies.get(FILTERS_TOUCHED_COOKIE),
      crawler: isCrawler(request.headers.get('user-agent')),
    }) !== null;

  const params = new URLSearchParams(
    useDefault ? DEFAULT_JOB_FILTERS : url.searchParams,
  );
  // `page` addresses the feed rather than filtering it; the API would ignore it,
  // and leaving it in would make it part of what the client seeds its filters from.
  params.delete('page');

  // Serve the page the URL asks for, so the <a href> pagination under the feed
  // leads somewhere: each of those links has to render its own rows. Note that a
  // ?page= URL is never the first-visit default case — defaultFilterTarget only
  // fires on a param-less homepage — so the two never contend.
  const pageNumber = parsePage(url.searchParams);
  const initial = await serverApi(fetch).searchJobs(params, LIMIT, pageOffset(pageNumber));
  return { initial, filterParams: params.toString(), pageNumber };
};

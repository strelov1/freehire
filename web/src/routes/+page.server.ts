import { redirect } from '@sveltejs/kit';
import { serverApi } from '$lib/server/api';
import { FILTERS_TOUCHED_COOKIE } from '$lib/filterStorage';
import { defaultFilterTarget, isCrawler } from '$lib/firstVisit';
import type { PageServerLoad } from './$types';

const LIMIT = 20;

// The homepage IS the job feed. Server-render the first page of search results
// for the current URL filters, so the job rows are in the initial HTML. The URL
// query already carries the search params (q, facets, sort) in the shape the
// search API reads; `searchJobs` adds pagination. Filters/sort/"load more" then
// run client-side over the same client.
export const load: PageServerLoad = async ({ url, fetch, cookies, request }) => {
  // First-visit default: redirect a brand-new human on a bare homepage to the
  // remote-worldwide slice before render, so the filtered feed is server-rendered
  // (no flash, no post-hydration store mutation). defaultFilterTarget owns which
  // requests are exempt (shared links, returning visitors, crawlers).
  const target = defaultFilterTarget({
    search: url.search,
    touched: !!cookies.get(FILTERS_TOUCHED_COOKIE),
    crawler: isCrawler(request.headers.get('user-agent')),
  });
  if (target) redirect(302, target);

  const initial = await serverApi(fetch).searchJobs(url.searchParams, LIMIT, 0);
  return { initial };
};

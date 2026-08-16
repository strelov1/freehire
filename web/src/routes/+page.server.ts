import { serverApi } from '$lib/server/api';
import { pageOffset, parsePage } from '$lib/pagination';
import type { PageServerLoad } from './$types';

const LIMIT = 20;

// The homepage IS the job feed. Server-render the first page of search results
// for the current URL filters, so the job rows are in the initial HTML. The URL
// query already carries the search params (q, facets, sort) in the shape the
// search API reads; `searchJobs` adds pagination. Filters/sort/"load more" then
// run client-side over the same client.
export const load: PageServerLoad = async ({ url, fetch }) => {
  const params = new URLSearchParams(url.searchParams);
  // `page` addresses the feed rather than filtering it; the API would ignore it,
  // and leaving it in would make it part of what the client seeds its filters from.
  params.delete('page');

  // Serve the page the URL asks for, so the <a href> pagination under the feed
  // leads somewhere: each of those links has to render its own rows.
  const pageNumber = parsePage(url.searchParams);
  const initial = await serverApi(fetch).searchJobs(params, LIMIT, pageOffset(pageNumber));
  return { initial, filterParams: params.toString(), pageNumber };
};

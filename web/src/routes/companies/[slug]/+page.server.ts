import { error, redirect } from '@sveltejs/kit';
import { ApiError, MovedError } from '$lib/api';
import { pageExists, pageOffset, parsePage } from '$lib/pagination';
import { serverApi } from '$lib/server/api';
import type { PageServerLoad } from './$types';

const LIMIT = 20;

// Server-render the company entity AND its first page of search results. The job
// list is search-backed and scoped to this company (company_slug), so it carries a
// true total (the vacancy count) and supports the same URL filters as /jobs. The
// company entity is fetched separately because search returns only jobs.
//
// Both are awaited, so the rows — and the /jobs/<slug> links on them — are in the
// initial HTML. The list used to stream as an unresolved promise, which arrived as
// a trailing JSON chunk only client-side JS turns into anchors; since crawlers
// discover links by parsing HTML, ~200k company pages advertised none of their
// vacancies, and these pages exist mainly to be found in search. The two calls
// still start together, so this costs the slower of the two rather than their sum
// (measured: +0.64s median TTFB, +1.28s worst).
//
// A 404 (unknown company) becomes a SvelteKit 404; other company failures bubble
// to the 500 page. A failed SEARCH must not take the page down with it — the
// header, About and facts are still worth serving — so it resolves to null and
// CompanyView renders the error state in place of the list.
export const load: PageServerLoad = async ({ params, url, fetch }) => {
  const client = serverApi(fetch);
  const facets = new URLSearchParams(url.searchParams);
  facets.set('company_slug', params.slug);
  // `page` addresses the feed, it is not a search facet.
  facets.delete('page');

  // Serve the page the URL asks for: the <a href> pagination under the feed is
  // what makes a large employer's later postings reachable by link at all.
  const pageNumber = parsePage(url.searchParams);
  // Start the search first so it overlaps the company fetch below.
  const search = client.searchJobs(facets, LIMIT, pageOffset(pageNumber)).catch(() => null);

  let entity;
  try {
    // Only `company` is used (the list comes from `search`), so the returned job
    // is discarded. We can't ask for zero jobs: the API clamps `limit` to >= 1
    // (pageParams), so limit=1 is already the minimal fetch. Trimming this fully
    // needs a backend company-entity-only path — deferred to the latency follow-up.
    entity = await client.getCompany(params.slug, 1, 0);
  } catch (e) {
    // A slug a merge retired: send the visitor — and the crawler holding the old link —
    // to the company that absorbed it, keeping whatever the url had earned in search.
    // The API answers 301 and the client surfaces it rather than following it, because a
    // followed redirect would render the right company under the retired url.
    if (e instanceof MovedError) {
      const query = url.search;
      redirect(301, `/companies/${e.canonicalSlug}${query}`);
    }
    if (e instanceof ApiError && e.status === 404) {
      error(404, 'Company not found');
    }
    throw e;
  }

  const initial = await search;
  // A page past the last one this employer's postings fill is an empty, self-canonical
  // 200 — see the collections loader. A null `initial` is the failed-search path, not
  // an empty company, so it keeps serving the header and facts at whatever page.
  if (initial && !pageExists(pageNumber, initial.total)) error(404, 'Page not found');

  return {
    company: entity.company,
    initial,
    slug: params.slug,
    pageNumber,
    referralAvailable: entity.referral_available,
  };
};

import { error } from '@sveltejs/kit';
import { ApiError } from '$lib/api';
import { serverApi } from '$lib/server/api';
import type { PageServerLoad } from './$types';

const LIMIT = 20;

// Server-render the company entity and stream its first page of search results.
// The job list is search-backed and scoped to this company (company_slug), so it
// carries a true total (the vacancy count) and supports the same URL filters as
// /jobs. The company entity is fetched separately because search returns only jobs.
//
// Only the company entity is awaited — it drives the header and SEO, and is cheap.
// The job search is the slow call, so it is returned as an *unresolved* promise
// that SvelteKit streams: the navigation completes as soon as the company resolves
// and CompanyView renders a skeleton until the jobs land. A 404 (unknown company)
// becomes a SvelteKit 404; other company failures bubble to the 500 page, and a
// failed search surfaces in CompanyView's {:catch}.
export const load: PageServerLoad = async ({ params, url, fetch }) => {
  const client = serverApi(fetch);
  const facets = new URLSearchParams(url.searchParams);
  facets.set('company_slug', params.slug);

  // Start the search now so it runs concurrently with the company fetch, but
  // don't await it — it streams to the client.
  const initial = client.searchJobs(facets, LIMIT, 0);

  try {
    // Only `company` is used (the list comes from `initial`), so the returned job
    // is discarded. We can't ask for zero jobs: the API clamps `limit` to >= 1
    // (pageParams), so limit=1 is already the minimal fetch. Trimming this fully
    // needs a backend company-entity-only path — deferred to the latency follow-up.
    const { company } = await client.getCompany(params.slug, 1, 0);
    return { company, initial, slug: params.slug };
  } catch (e) {
    // The company load failed; mark the abandoned search promise handled so its
    // eventual rejection isn't an unhandled rejection on the server.
    initial.catch(() => {});
    if (e instanceof ApiError && e.status === 404) {
      error(404, 'Company not found');
    }
    throw e;
  }
};

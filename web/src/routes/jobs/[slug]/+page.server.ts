import { error } from '@sveltejs/kit';
import { ApiError } from '$lib/api';
import { serverApi } from '$lib/server/api';
import type { PageServerLoad } from './$types';

// Server-render the job detail: fetch by slug so the article content is in the
// initial HTML. A 404 from the API becomes a SvelteKit 404 page (not a 200
// shell); other failures bubble to the 500 page.
export const load: PageServerLoad = async ({ params, fetch }) => {
  const api = serverApi(fetch);
  // Both fetches key only on the slug and are independent, so run them in parallel
  // — serialising them cost a full API round-trip on every job page. They stay
  // awaited (not streamed) so the "Similar jobs" rows remain in the SSR HTML for
  // internal-link crawlability.
  //
  // Similar jobs are a non-essential discovery aid: a failure (search disabled,
  // no neighbours yet) must not break the page, so it degrades to an empty list.
  const [job, similar, copiesResult] = await Promise.all([
    api.getJob(params.slug).catch((e) => {
      if (e instanceof ApiError && e.status === 404) error(404, 'Job not found');
      throw e;
    }),
    api.getSimilarJobs(params.slug).catch(() => []),
    // A small preview of the other-locations tab (the full list is /jobs/:slug/copies).
    // Non-essential and only meaningful for a mass-posted role, so it degrades to empty.
    api.getJobCopies(params.slug, 10).catch(() => ({ copies: [], total: 0 })),
  ]);
  return { job, similar, copies: copiesResult.copies, copiesTotal: copiesResult.total };
};

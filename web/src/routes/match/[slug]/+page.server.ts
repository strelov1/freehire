import { error } from '@sveltejs/kit';
import { ApiError } from '$lib/api';
import { serverApi } from '$lib/server/api';
import type { PageServerLoad } from './$types';

// Server-render the fit-analysis page: the job (for context/title) plus the caller's
// cached analysis, so a fresh cached result paints in the initial HTML. The cached fit
// needs auth and may not exist — a 401/404/no-cache degrades to null, and the page then
// opens the SSE stream client-side. A missing job is a real 404.
export const load: PageServerLoad = async ({ params, fetch, request }) => {
  // getMatchAnalysis is authenticated, so the session cookie must be forwarded to the internal
  // API — without it the SSR read is a 401, the cached analysis reads as absent, and the
  // page needlessly re-streams a fresh compute every visit.
  const api = serverApi(fetch, request.headers.get('cookie'));
  const job = await api.getJob(params.slug).catch((e) => {
    if (e instanceof ApiError && e.status === 404) error(404, 'Job not found');
    throw e;
  });
  const fit = await api.getMatchAnalysis(params.slug).catch(() => null);
  return { job, fit };
};

import { error } from '@sveltejs/kit';
import { ApiError } from '$lib/api';
import { serverApi } from '$lib/server/api';
import type { PageServerLoad } from './$types';

// Server-render the fit-analysis page: the job (for context/title) plus the caller's
// cached analysis, so a fresh cached result paints in the initial HTML. The cached fit
// needs auth and may not exist — a 401/404/no-cache degrades to null, and the page then
// opens the SSE stream client-side. A missing job is a real 404.
export const load: PageServerLoad = async ({ params, fetch }) => {
  const api = serverApi(fetch);
  const job = await api.getJob(params.slug).catch((e) => {
    if (e instanceof ApiError && e.status === 404) error(404, 'Job not found');
    throw e;
  });
  const fit = await api.getJobFit(params.slug).catch(() => null);
  return { job, fit };
};

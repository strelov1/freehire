import { serverApi } from '$lib/server/api';
import type { PageServerLoad } from './$types';

// Live catalogue scale for the landing stats strip. Both totals are a one-row
// list read (limit=1) — we only want `meta.total`, not the page. `allSettled`
// keeps the marketing homepage rendering even if the API hiccups: a failed leg
// yields `null`, and HomeView falls back to a static "+" figure.
export const load: PageServerLoad = async ({ fetch }) => {
  const api = serverApi(fetch);
  const [jobs, companies] = await Promise.allSettled([api.listJobs(1, 0), api.listCompanies('', 1, 0)]);
  return {
    stats: {
      jobs: jobs.status === 'fulfilled' ? jobs.value.total ?? null : null,
      companies: companies.status === 'fulfilled' ? companies.value.total ?? null : null,
    },
  };
};

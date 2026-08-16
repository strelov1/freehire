import { serverApi } from '$lib/server/api';
import type { PageServerLoad } from './$types';

// Live catalogue scale for the landing stats strip. One call: the backend publishes
// every scale figure as a single snapshot, so the numbers here and the ones on /open
// describe the same measurement rather than two list totals taken at two moments.
//
// The endpoint itself never fails — with no published snapshot it answers with an
// approximate open-job count. The catch covers the transport (the API unreachable), in
// which case HomeView falls back to its static "+" figures.
export const load: PageServerLoad = async ({ fetch }) => {
  try {
    const scale = await serverApi(fetch).catalogScale();
    return {
      stats: {
        jobs: scale.open_jobs,
        // Only the exact snapshot carries a company count; a degraded read reports
        // zero, which is "not measured", not "none". Null instead, so HomeView shows
        // its last-known figure rather than printing "0+ companies".
        companies: scale.exact ? scale.companies : null,
        sources: scale.sources,
      },
    };
  } catch {
    return { stats: { jobs: null, companies: null, sources: null } };
  }
};

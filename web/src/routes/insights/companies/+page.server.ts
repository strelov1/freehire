import { serverApi } from '$lib/server/api';
import type { PageServerLoad } from './$types';

// The company hiring-signal leaderboard: which companies grew or shrank their number
// of open jobs most over the last 30 days. Public, SSR-live with a CDN cache window
// like the sibling insights pages. min_open floors out tiny boards so ingest-artifact
// spikes don't dominate the ranking.
export const load: PageServerLoad = async ({ fetch, setHeaders }) => {
  const api = serverApi(fetch);
  const [ramping, freezing] = await Promise.all([
    api.insightsCompanies({ sort: 'growth', minOpen: 10, limit: 25 }),
    api.insightsCompanies({ sort: '-growth', minOpen: 10, limit: 25 }),
  ]);
  setHeaders({ 'cache-control': 'public, max-age=0, s-maxage=3600' });
  return { ramping, freezing };
};

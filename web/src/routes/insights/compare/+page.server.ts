import { serverApi } from '#lib/server/api.js';
import type { PageServerLoad } from './$types';

export const load: PageServerLoad = async ({ fetch, setHeaders }) => {
  const points = await serverApi(fetch)
    .insightsRoleDemand()
    .catch(() => []);
  setHeaders({ 'cache-control': `public, max-age=0, s-maxage=${points.length ? 3600 : 60}` });
  return { points };
};

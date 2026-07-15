import { serverApi } from '$lib/server/api';
import type { PageServerLoad } from './$types';

// The public /status page pulls the ingest-fleet health live from our own public
// API. Best-effort: if the read fails, the page renders an "unavailable" state
// rather than erroring the whole route.

export const load: PageServerLoad = async ({ fetch, setHeaders }) => {
  const api = serverApi(fetch);

  // The rollup moves slowly (crons run hourly-plus); let the CDN/browser hold it briefly.
  setHeaders({ 'cache-control': 'public, max-age=60' });

  try {
    return { status: await api.ingestStatus() };
  } catch {
    return { status: null };
  }
};

import { serverApi } from '$lib/server/api';
import type { EmbedProgress } from '$lib/types';
import type { PageServerLoad } from './$types';

// The first paint of /semantic-search, from the same document the page's stream sends.
// Best-effort like /status: a failed read renders every figure as "measuring…"
// rather than erroring the route, and the stream fills them in once it connects.
const UNMEASURED: EmbedProgress = {
  eligible: null,
  covered: null,
  snapshot_covered: null,
  queued: null,
  rate_per_minute: null,
  eta_seconds: null,
  snapshot_at: null,
  measured_at: null,
};

export const load: PageServerLoad = async ({ fetch, setHeaders }) => {
  // The figures move every 15 seconds and the stream takes over after the first
  // paint, so a short shared cache costs a reader nothing.
  setHeaders({ 'cache-control': 'public, max-age=15' });
  try {
    return { progress: await serverApi(fetch).embedProgress() };
  } catch {
    return { progress: UNMEASURED };
  }
};

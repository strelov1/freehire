import { serverApi } from '$lib/server/api';
import type { PageServerLoad } from './$types';

export const load: PageServerLoad = async ({ fetch }) => {
  const api = serverApi(fetch);
  // An unreachable feed is reported as a failure, not as an empty one. Catching into
  // `threads: []` renders "No discussions yet." — which is a measurement, and with a
  // handful of threads live an outage is indistinguishable from the truth. Same rule
  // the catalogue figures follow: a zero must not reach a page as if it were counted.
  try {
    const { threads, nextCursor } = await api.listRecentThreads();
    return { threads, nextCursor, failed: false };
  } catch {
    return { threads: [], nextCursor: undefined, failed: true };
  }
};

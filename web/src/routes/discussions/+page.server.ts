import { serverApi } from '$lib/server/api';
import type { PageServerLoad } from './$types';

export const load: PageServerLoad = async ({ fetch }) => {
  const api = serverApi(fetch);
  // An unreachable feed renders the empty state rather than an error page: the
  // section is a browsing surface, and nothing here is the reader's own data.
  const { threads, nextCursor } = await api
    .listRecentThreads()
    .catch(() => ({ threads: [], nextCursor: undefined }));
  return { threads, nextCursor };
};

import { serverApi } from '$lib/server/api';
import type { PageServerLoad } from './$types';

export const load: PageServerLoad = async ({ params, fetch }) => {
  const api = serverApi(fetch);
  const { threads, nextCursor } = await api
    .listThreads('job', params.slug)
    .catch(() => ({ threads: [], nextCursor: undefined }));
  return { slug: params.slug, threads, nextCursor };
};

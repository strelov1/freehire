import { serverApi } from '$lib/server/api';
import type { PageServerLoad } from './$types';

// First page of a company's discussion threads, server-rendered so the list is in
// the initial HTML. A list failure degrades to empty rather than breaking the page.
export const load: PageServerLoad = async ({ params, fetch }) => {
  const api = serverApi(fetch);
  const { threads, nextCursor } = await api
    .listThreads('company', params.slug)
    .catch(() => ({ threads: [], nextCursor: undefined }));
  return { slug: params.slug, threads, nextCursor };
};

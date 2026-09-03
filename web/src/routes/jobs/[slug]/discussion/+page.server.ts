import { serverApi } from '$lib/server/api';
import { loadDiscussionSubject } from '$lib/server/discussionSubject';
import type { PageServerLoad } from './$types';

// First page of a vacancy's discussion threads, server-rendered so the list is in the
// initial HTML. The vacancy itself rides along for the header: the list can be empty
// and the header must still name what the page is about, so it cannot be read off the
// threads. Both degrade rather than break — neither is the reader's own data.
export const load: PageServerLoad = async ({ params, fetch }) => {
  const api = serverApi(fetch);
  const [{ threads, nextCursor }, subject] = await Promise.all([
    api.listThreads('job', params.slug).catch(() => ({ threads: [], nextCursor: undefined })),
    loadDiscussionSubject(fetch, 'job', params.slug),
  ]);
  return { slug: params.slug, threads, nextCursor, subject };
};

import { serverApi } from '$lib/server/api';
import { loadDiscussionSubject } from '$lib/server/discussionSubject';
import type { PageServerLoad } from './$types';

// First page of a company's discussion threads, server-rendered so the list is in the
// initial HTML. The company itself rides along for the header, which until now printed
// the raw slug where the company's name belongs. A list failure degrades to empty and
// an unreadable company to null, rather than breaking the page.
export const load: PageServerLoad = async ({ params, fetch }) => {
  const api = serverApi(fetch);
  const [{ threads, nextCursor }, subject] = await Promise.all([
    api.listThreads('company', params.slug).catch(() => ({ threads: [], nextCursor: undefined })),
    loadDiscussionSubject(fetch, 'company', params.slug),
  ]);
  return { slug: params.slug, threads, nextCursor, subject };
};

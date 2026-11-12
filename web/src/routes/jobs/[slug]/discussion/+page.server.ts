import { redirect } from '@sveltejs/kit';
import { resolve } from '$app/paths';
import { serverApi } from '#lib/server/api.js';
import { loadDiscussionIndex } from '#lib/server/community.js';
import type { PageServerLoad } from './$types';

// The job's discussion is a tab on the job page now; this address stays for the links
// already out there (and a thread's "Back to discussion"). It hands over only when the
// job answers to this exact slug: topics are stored under the slug they were started on,
// and the tab lists them by the job's CURRENT slug, so a renamed or removed job keeps its
// discussion here.
export const load: PageServerLoad = async ({ params, fetch }) => {
  const job = await serverApi(fetch)
    .getJob(params.slug)
    .catch(() => null);
  if (job?.public_slug === params.slug) {
    redirect(301, `${resolve('/jobs/[slug]', { slug: params.slug })}?tab=discussion`);
  }
  return loadDiscussionIndex(fetch, 'job', params.slug);
};

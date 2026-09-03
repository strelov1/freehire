import { loadDiscussionIndex } from '$lib/server/community';
import type { PageServerLoad } from './$types';

export const load: PageServerLoad = async ({ params, fetch }) =>
  loadDiscussionIndex(fetch, 'company', params.slug);

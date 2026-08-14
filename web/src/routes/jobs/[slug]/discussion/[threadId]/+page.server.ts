import { loadThread } from '$lib/server/community';
import type { PageServerLoad } from './$types';

export const load: PageServerLoad = async ({ params, fetch }) => loadThread(fetch, 'job', params);

import { loadThread } from '#lib/server/community.js';
import type { PageServerLoad } from './$types';

export const load: PageServerLoad = async ({ params, fetch }) => loadThread(fetch, 'job', params);

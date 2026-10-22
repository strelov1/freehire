import { error } from '@sveltejs/kit';
import { ApiError } from '#lib/api.js';
import { serverApi } from '#lib/server/api.js';
import { rethrowUpstream } from '#lib/server/upstream.js';
import type { PageServerLoad } from './$types';

// One session, readable by its two parties only. A caller who is neither gets the same
// 404 as one naming a session that does not exist — the endpoint answers that way on
// purpose, so this route cannot be used to confirm a booking is real, and the page must
// not soften it into "you don't have access to this".
export const load: PageServerLoad = async ({ params, fetch, request }) => {
  const api = serverApi(fetch, request.headers.get('cookie'));
  try {
    return { session: await api.getMySession(params.id) };
  } catch (e) {
    if (e instanceof ApiError && e.status === 404) {
      error(404, 'Session not found');
    }
    rethrowUpstream(e);
  }
};

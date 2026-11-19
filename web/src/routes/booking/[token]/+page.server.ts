import { error } from '@sveltejs/kit';
import { ApiError } from '#lib/api.js';
import { serverApi } from '#lib/server/api.js';
import { rethrowUpstream } from '#lib/server/upstream.js';
import type { PageServerLoad } from './$types';

// Only checks the link resolves, so a dead one is a real 404 page. The slots themselves are
// read in the browser, in the viewer's zone, for the reason mentors/[slug] gives.
export const load: PageServerLoad = async ({ params, fetch }) => {
  try {
    await serverApi(fetch).getBookingSlots(params.token, 'UTC');
  } catch (e) {
    if (e instanceof ApiError && e.status === 404) {
      error(404, 'This booking link is not available');
    }
    rethrowUpstream(e);
  }
  return { token: params.token };
};

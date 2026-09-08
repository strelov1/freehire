import { error } from '@sveltejs/kit';
import { ApiError } from '$lib/api';
import { serverApi } from '$lib/server/api';
import { rethrowUpstream } from '$lib/server/upstream';
import type { PageServerLoad } from './$types';

// One member's public card, by their minted catalogue handle.
//
// A member who has left, one whose CV extract has gone stale, a handle nobody holds, and
// a string that could not be a handle all answer the same 404 from the backend
// (internal/api/handler/talent_catalog.go). This load does not try to tell them apart —
// doing so is what would turn the route into a way of asking whether an account exists.
// Public read: no cookie forwarded.
export const load: PageServerLoad = async ({ params, fetch }) => {
  try {
    return { member: await serverApi(fetch).getTalentCard(params.handle) };
  } catch (e) {
    if (e instanceof ApiError && e.status === 404) error(404, 'Profile not found');
    rethrowUpstream(e);
  }
};

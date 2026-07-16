import { redirect } from '@sveltejs/kit';
import { serverApi } from '$lib/server/api';
import type { PageServerLoad } from './$types';

// /my/tracking/[slug] renders the tracking board with the application's drawer open
// (deep-linkable — the inbox links here, and a refresh/share reopens the same card).
// The board itself is for any signed-in user; the Emails tab inside the drawer
// self-gates to moderators. Guard auth like /my/tracking and pass the slug through.
// The board is server-fetched (same as /my/tracking) so opening a deep link paints
// the board + drawer in one round trip instead of a client fetch on mount.
export const load: PageServerLoad = async ({ parent, params, url, fetch, request }) => {
  const { user } = await parent();
  if (!user) {
    redirect(302, `/?auth=required&redirect=${encodeURIComponent(url.pathname)}`);
  }
  // A transient API failure shouldn't 500 the deep link — leave board undefined and
  // let JobBoard fall back to its client fetch (which renders the friendly error state).
  try {
    const board = await serverApi(fetch, request.headers.get('cookie')).listMyJobs('board', 500, 0);
    return { slug: params.slug, board: board.items };
  } catch {
    return { slug: params.slug };
  }
};

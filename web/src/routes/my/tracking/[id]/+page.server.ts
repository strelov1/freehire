import { redirect } from '@sveltejs/kit';
import { loadBoard } from '$lib/server/tracking';
import type { PageServerLoad } from './$types';

// /my/tracking/[id] renders the tracking board with the application's drawer open
// (deep-linkable — the inbox links here, and a refresh/share reopens the same card).
// The board itself is for any signed-in user; the Emails tab inside the drawer
// self-gates to moderators. Guard auth like /my/tracking and pass the row id through.
// The board is server-fetched (same as /my/tracking) so opening a deep link paints
// the board + drawer in one round trip instead of a client fetch on mount.
export const load: PageServerLoad = async ({ parent, params, url, fetch, request }) => {
  const { user } = await parent();
  if (!user) {
    const target = url.pathname + url.search;
    redirect(302, `/?auth=required&redirect=${encodeURIComponent(target)}`);
  }
  return { id: params.id, board: await loadBoard(fetch, request.headers.get('cookie')) };
};

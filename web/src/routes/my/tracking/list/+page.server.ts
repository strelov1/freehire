import { redirect } from '@sveltejs/kit';
import { signinUrl } from '$lib/signin';
import { loadBoard } from '$lib/server/tracking';
import type { PageServerLoad } from './$types';

// /my/tracking/list is the board's rows read as a list. Same guard and same server
// fetch as /my/tracking — one load, so the two views cannot show different data.
export const load: PageServerLoad = async ({ parent, url, fetch, request }) => {
  const { user } = await parent();
  if (!user) {
    redirect(302, signinUrl({ returnTo: url.pathname + url.search, cancelTo: '/', mode: 'login' }));
  }
  return { board: await loadBoard(fetch, request.headers.get('cookie')) };
};

import { error } from '@sveltejs/kit';
import { ApiError } from '$lib/api';
import { serverApi } from '$lib/server/api';
import type { PageServerLoad } from './$types';

const LIMIT = 20;

// A public board is a shared saved search: load it by slug, then server-render the first
// page of the jobs matching its stored query (so the rows are in the initial HTML for
// sharing/reload). An unknown or unshared slug is a 404 → the app's +error page renders
// the not-found state. Public read: no cookie forwarded.
export const load: PageServerLoad = async ({ params, fetch }) => {
  const client = serverApi(fetch);

  let board;
  try {
    board = await client.getBoard(params.slug);
  } catch (e) {
    if (e instanceof ApiError && e.status === 404) throw error(404, 'Board not found');
    throw e;
  }

  const initial = await client.searchJobs(new URLSearchParams(board.query), LIMIT, 0);
  return { board, initial };
};

import { error } from '@sveltejs/kit';
import { ApiError } from '$lib/api';
import { serverApi } from '$lib/server/api';
import { rethrowUpstream } from '$lib/server/upstream';
import type { PageServerLoad } from './$types';

// A public job list: load it by slug and render its jobs directly — unlike a shared
// board (a live query), the public read already returns the materialized set, so
// there is no second call to re-run a query. An unknown or unshared slug is a 404 →
// the app's +error page renders the not-found state. Public read: no cookie forwarded.
export const load: PageServerLoad = async ({ params, fetch }) => {
  const client = serverApi(fetch);

  try {
    return { list: await client.getPublicJobList(params.slug) };
  } catch (e) {
    if (e instanceof ApiError && e.status === 404) throw error(404, 'List not found');
    rethrowUpstream(e);
  }
};

import { serverApi } from '$lib/server/api';
import type { PageServerLoad } from './$types';

// The sessions the caller BOOKED. The profile belongs to the layout, so this pane asks for
// one thing rather than for everything the section might need.
//
// The upcoming/past split is the SERVER's, made against one clock for the whole list.
// Recomputing it here would reopen the gap where a session starting mid-read lands in both
// halves or in neither.
export const load: PageServerLoad = async ({ fetch, request }) => {
  const api = serverApi(fetch, request.headers.get('cookie'));
  return { sessions: await api.listMySessions() };
};

import { serverApi } from '$lib/server/api';
import type { PageServerLoad } from './$types';

// The seeker's own sessions. Authenticated, so the session cookie is forwarded explicitly:
// with an absolute API base URL `event.fetch` does not carry it (see $lib/server/api).
//
// The split into upcoming and past is the SERVER's, made against one clock for the whole
// list — recomputing it here would reopen the gap where a session starting mid-read lands
// in both halves or in neither.
export const load: PageServerLoad = async ({ fetch, request }) => {
  const api = serverApi(fetch, request.headers.get('cookie'));
  return { sessions: await api.listMySessions() };
};

import { serverApi } from '$lib/server/api';
import type { PageServerLoad } from './$types';

// The mentor's own weekly rows and dated exceptions, in one list — the endpoint returns
// both kinds together and `splitAvailability` separates them for display.
export const load: PageServerLoad = async ({ fetch, request }) => {
  const api = serverApi(fetch, request.headers.get('cookie'));
  return { availability: await api.myMentorAvailability() };
};

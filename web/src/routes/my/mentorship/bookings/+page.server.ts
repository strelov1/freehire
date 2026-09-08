import { serverApi } from '$lib/server/api';
import type { PageServerLoad } from './$types';

// The sessions booked WITH the caller, as a mentor. Carries each seeker's address — the
// mentor is meeting this person, so their identity is not a leak.
export const load: PageServerLoad = async ({ fetch, request }) => {
  const api = serverApi(fetch, request.headers.get('cookie'));
  return { bookings: await api.myMentorBookings() };
};

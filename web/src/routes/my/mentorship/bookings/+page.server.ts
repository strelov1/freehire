import { redirect } from '@sveltejs/kit';
import { serverApi } from '$lib/server/api';
import type { PageServerLoad } from './$types';

// The sessions booked WITH the caller, as a mentor. Carries each seeker's address — the
// mentor is meeting this person, so their identity is not a leak.
//
// Mentor-only, and gated here rather than by hiding the tab: see the schedule pane's own
// note. A hidden tab is not a closed door, and this one answered 500 to anybody who
// walked through it.
export const load: PageServerLoad = async ({ fetch, request, parent }) => {
  const { profile } = await parent();
  if (!profile) redirect(303, '/my/mentorship');

  const api = serverApi(fetch, request.headers.get('cookie'));
  return { bookings: await api.myMentorBookings() };
};

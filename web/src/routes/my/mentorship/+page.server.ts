import { serverApi } from '$lib/server/api';
import type { PageServerLoad } from './$types';

// One section for both sides of the marketplace: the sessions the caller BOOKED, and —
// when they are a mentor — their profile, schedule and the sessions booked WITH them.
//
// Authenticated, so the session cookie is forwarded explicitly: with an absolute API base
// URL `event.fetch` does not carry it (see $lib/server/api).
//
// The upcoming/past split is the SERVER's, made against one clock for the whole list.
// Recomputing it here would reopen the gap where a session starting mid-read lands in both
// halves or in neither.
//
// A caller who is not a mentor gets `profile: null` — that is the ordinary answer, not a
// failure, so `myMentorProfile` turns the 404 into null rather than leaving every caller
// to catch it. The mentor-only reads are then skipped entirely rather than fetched and
// discarded.
export const load: PageServerLoad = async ({ fetch, request }) => {
  const api = serverApi(fetch, request.headers.get('cookie'));

  const [sessions, profile] = await Promise.all([api.listMySessions(), api.myMentorProfile()]);
  if (!profile) {
    return { sessions, profile: null, availability: [], bookings: null };
  }

  const [availability, bookings] = await Promise.all([
    api.myMentorAvailability(),
    api.myMentorBookings(),
  ]);
  return { sessions, profile, availability, bookings };
};

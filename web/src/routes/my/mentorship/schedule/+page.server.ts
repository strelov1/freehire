import { requireMentorProfile } from '$lib/server/mentorshipGate';
import { serverApi } from '$lib/server/api';
import type { PageServerLoad } from './$types';

// Mentor-only, and gated HERE rather than by hiding the tab. Hiding it is what a visitor
// sees; this is what happens when one arrives anyway — by typing the address, by following
// a link somebody pasted, or by keeping a bookmark from before they withdrew. Without the
// gate `myMentorAvailability` throws for an account with no profile and the pane answers
// 500, which is the ugliest possible way to say "this is not for you".
//
// A redirect rather than a 404: they ARE allowed in this section, just not on this pane,
// and the index is the part of it that is theirs.
export const load: PageServerLoad = async ({ fetch, request, parent }) => {
  // The layout has already read it; this costs no second call.
  requireMentorProfile((await parent()).profile);

  const api = serverApi(fetch, request.headers.get('cookie'));
  return { availability: await api.myMentorAvailability() };
};

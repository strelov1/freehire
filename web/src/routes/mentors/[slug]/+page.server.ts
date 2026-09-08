import { error } from '@sveltejs/kit';
import { ApiError } from '$lib/api';
import { requireMentorshipAccess } from '$lib/server/mentorshipGate';
import { serverApi } from '$lib/server/api';
import { rethrowUpstream } from '$lib/server/upstream';
import type { PageServerLoad } from './$types';

// The PROFILE is server-rendered; the SLOTS are not, and that split is deliberate.
//
// Every slot is expressed in the viewer's zone, and the server does not know it — the
// browser resolves it, and the endpoint echoes back the zone it actually used. Rendering
// slots here would mean rendering them in UTC and then swapping every visible time after
// hydration, which shows a seeker one set of hours and then silently replaces it with
// another. Times that change under you are worse than times that arrive a moment late.
//
// The profile itself carries no zone-dependent text, so it renders on the server as
// usual: the name, headline, company and topics are what search reads, and the page
// exists partly to be found.
//
// A profile that is pending, rejected, paused or withdrawn answers as though it does not
// exist, so the 404 below covers all of them — deliberately, since telling a visitor that
// a mentor exists but is hidden leaks the moderation queue.
export const load: PageServerLoad = async ({ params, fetch , parent }) => {
  // Beta-gated, and a 404 rather than a 403: while the marketplace is unreleased the
  // honest answer is that this page is not there, and a 403 would advertise it to
  // every crawler that found the URL.
  requireMentorshipAccess((await parent()).user);

  try {
    return { mentor: await serverApi(fetch).getMentor(params.slug) };
  } catch (e) {
    if (e instanceof ApiError && e.status === 404) {
      error(404, 'Mentor not found');
    }
    rethrowUpstream(e);
  }
};

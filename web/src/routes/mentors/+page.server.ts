import { requireMentorshipAccess } from '$lib/server/mentorshipGate';
import { serverApi } from '$lib/server/api';
import { mentorFilterOptions, mentorFiltersFromParams, mentorFiltersToParams } from '$lib/mentorship';
import type { PageServerLoad } from './$types';

// Server-render the directory for the current filters, so a shared or deep-linked
// filtered URL renders filtered in the initial HTML rather than after hydration.
//
// Round-tripping through mentorFilters whitelists the query to the three params the
// endpoint reads (`knownMentorParams` in internal/api/handler/mentorship.go). A param it
// does not read would widen the answer while the address bar still claimed it narrowed it.
//
// TWO reads, and only when something is actually filtered. The filter controls are built
// from the UNFILTERED directory: a narrowed list cannot offer the values it just excluded,
// so deriving the options from the visible rows makes every filter a one-way door. The
// unfiltered read doubles as the list whenever no filter is set, which is the common case.
//
// Filtering itself stays on the endpoint rather than being redone here. The publication
// predicate — approved, unpaused, not withdrawn — lives in one place on purpose, and a
// second copy in the browser is the drift the mentor-profile spec warns about.
export const load: PageServerLoad = async ({ url, fetch , parent }) => {
  // Beta-gated, and a 404 rather than a 403: while the marketplace is unreleased the
  // honest answer is that this page is not there, and a 403 would advertise it to
  // every crawler that found the URL.
  requireMentorshipAccess((await parent()).user);

  const filters = mentorFiltersFromParams(url.searchParams);
  const params = mentorFiltersToParams(filters);
  const api = serverApi(fetch);

  const all = await api.listMentors();
  const mentors = params.toString() ? await api.listMentors(params) : all;

  return { mentors, options: mentorFilterOptions(all), filters };
};

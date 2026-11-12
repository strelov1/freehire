import type { Http } from '../api-core';
import type {
  Mentor,
  MentorAvailabilityRule,
  MentorCalendar,
  MentorProfileInput,
  MentorProfileSuggestions,
  MentorSession,
  MentorSessions,
  MentorSlot,
  MentorSlots,
  OwnMentorProfile,
  PendingMentorProfile,
} from '../types';
import type { components } from '#lib/generated/api.js';
import { ApiError, jsonBody } from '../api-core';

export function mentorsApi({ call, request, requestData }: Http) {
  /** Population-ranked city-name search over the embedded GeoNames dictionary,
   *  backing the profile's base-city and relocation-cities autocomplete. `country`
   *  narrows to one ISO 3166-1 alpha-2 code; each result carries its own raw code
   *  too (unrelated cities can share a name), not a pre-composed label. */
  /** The public mentor directory: approved, unpaused profiles, narrowed by the params
   *  `mentorFiltersToParams` produced.
   *
   *  There is no offset and no `total` — the endpoint answers `{data, meta:{count}}`, and
   *  the supply here is hand-onboarded rather than crawled, so `limit` bounds the page and
   *  nothing paginates it. */
  async function listMentors(filters?: URLSearchParams, limit?: number): Promise<Mentor[]> {
    const params = new URLSearchParams(filters);
    if (limit != null) params.set('limit', String(limit));
    const q = params.toString();
    return requestData<Mentor[]>(`/api/v1/mentors${q ? `?${q}` : ''}`);
  }

  /** One published mentor. Answers as though the profile did not exist when it is
   *  pending, rejected, paused or withdrawn — so a 404 here means "not bookable", not
   *  necessarily "never existed". */
  async function getMentor(slug: string): Promise<Mentor> {
    return requestData<Mentor>(`/api/v1/mentors/${slug}`);
  }

  /** The bookable hours in a window, expressed in the viewer's zone.
   *
   *  `from`/`to` are RFC 3339 INSTANTS, not dates — the endpoint parses them that way and
   *  bounds the span at 62 days. The answer's `timezone` is the zone actually used and may
   *  not be the one asked for: an unrecognised name falls back to UTC, and a caller that
   *  assumes otherwise cannot tell a correct time from a wrong one. */
  async function getMentorSlots(
    slug: string,
    from: string,
    to: string,
    timezone: string,
    signal?: AbortSignal,
  ): Promise<MentorSlots> {
    const params = new URLSearchParams({ from, to });
    if (timezone) params.set('timezone', timezone);
    const res = await request<{ data: MentorSlot[]; meta: { timezone: string } }>(
      `/api/v1/mentors/${slug}/slots?${params}`,
      { signal },
    );
    return { slots: res.data, timezone: res.meta.timezone };
  }

  /** Take one of a mentor's offered hours.
   *
   *  `starts_at` is the absolute instant, never the wall clock: it is what the server
   *  re-derives the slot from. `timezone` is recorded on the booking so the confirmation
   *  can be written in the zone the seeker actually booked in.
   *
   *  Refused with a NAMED reason when the hour is no longer offerable — the mentor paused,
   *  the availability moved, the notice period elapsed, or somebody else took it. The race
   *  and the stale tab are deliberately indistinguishable here. */
  async function bookMentorSession(
    slug: string,
    body: { starts_at: string; timezone: string; note?: string; job_id?: number },
  ): Promise<MentorSession> {
    return requestData<MentorSession>(
      `/api/v1/mentors/${slug}/bookings`,
      jsonBody('POST', body satisfies components['schemas']['MentorshipProfileBody']),
    );
  }

  /** The caller's own sessions, split by the server against one clock. */
  async function listMySessions(): Promise<MentorSessions> {
    return requestData<MentorSessions>('/api/v1/me/mentorship/sessions');
  }

  /** One session. Readable by its two parties only; anybody else gets the 404 a session
   *  that does not exist gets, so the route cannot confirm one is real. */
  async function getMySession(id: string): Promise<MentorSession> {
    return requestData<MentorSession>(`/api/v1/me/mentorship/sessions/${id}`);
  }

  /** The seeker's rating of a completed session.
   *
   *  PUT, not POST: there is one review per session and a second submission REPLACES the
   *  first. The mentor's review count does not move on an edit. */
  async function reviewMySession(id: string, rating: number, comment: string): Promise<void> {
    await call(`/api/v1/me/mentorship/sessions/${id}/review`, jsonBody('PUT', { rating, comment }));
  }

  /** Cancel, as either party, before the start. Frees the hour and tells the other side. */
  async function cancelMySession(id: string, reason: string): Promise<MentorSession> {
    return requestData<MentorSession>(`/api/v1/me/mentorship/sessions/${id}/cancel`, jsonBody('POST', { reason }));
  }

  // --- The mentor's own cabinet ----------------------------------------------

  /** The caller's mentor profile, or null when they have none. A 404 here is the ordinary
   *  answer for "not a mentor", not a failure, so it is turned into null rather than left
   *  for every caller to catch. */
  async function myMentorProfile(): Promise<OwnMentorProfile | null> {
    try {
      return await requestData<OwnMentorProfile>('/api/v1/me/mentorship/profile');
    } catch (e) {
      if (e instanceof ApiError && e.status === 404) return null;
      throw e;
    }
  }

  /** Best-effort prefill for the create form, from the candidate's résumé, user
   *  profile, account and experience bank — never from the mentor profile itself. Only
   *  the create form calls it, and only before a profile exists; a failure here must
   *  not block rendering the (then blank) form. */
  async function mentorProfileSuggestions(): Promise<MentorProfileSuggestions> {
    return requestData<MentorProfileSuggestions>('/api/v1/me/mentorship/profile/suggestions');
  }

  async function createMentorProfile(body: MentorProfileInput): Promise<OwnMentorProfile> {
    return requestData<OwnMentorProfile>('/api/v1/me/mentorship/profile', jsonBody('POST', body));
  }

  async function updateMentorProfile(body: MentorProfileInput): Promise<OwnMentorProfile> {
    return requestData<OwnMentorProfile>(
      '/api/v1/me/mentorship/profile',
      jsonBody('PUT', body satisfies components['schemas']['MentorshipProfileBody']),
    );
  }

  /** Leave the directory without moderation, keeping confirmed bookings standing. */
  async function pauseMentorProfile(paused: boolean): Promise<OwnMentorProfile> {
    return requestData<OwnMentorProfile>('/api/v1/me/mentorship/profile/pause', jsonBody('POST', { paused }));
  }

  /** Withdraw. Every confirmed future booking is cancelled and each seeker told; past
   *  bookings are retained as history. The row is MARKED, not deleted — bookings and
   *  reviews cascade off it, so a delete would erase every session that ever happened.
   *  Idempotent: withdrawing an already-withdrawn profile succeeds. */
  async function withdrawMentorProfile(): Promise<void> {
    await call('/api/v1/me/mentorship/profile', { method: 'DELETE' });
  }

  /** Resubmit a withdrawn profile for moderation — back to pending, with no special
   *  treatment for having been a mentor before. Refused (409) for a profile that was
   *  never withdrawn. */
  async function reactivateMentorProfile(): Promise<OwnMentorProfile> {
    return requestData<OwnMentorProfile>('/api/v1/me/mentorship/profile/reactivate', { method: 'POST' });
  }

  async function myMentorAvailability(): Promise<MentorAvailabilityRule[]> {
    return requestData<MentorAvailabilityRule[]>('/api/v1/me/mentorship/availability');
  }

  /** The caller's own resolved calendar for one month — booked, busy, free and closed
   *  time — read-only. `month` is `YYYY-MM`; omitted, the server defaults to the current
   *  month. */
  async function myMentorCalendar(month?: string): Promise<MentorCalendar> {
    const params = month ? `?month=${encodeURIComponent(month)}` : '';
    const res = await request<{ data: MentorCalendar['intervals']; meta: { timezone: string } }>(
      `/api/v1/me/mentorship/availability/calendar${params}`,
    );
    return { intervals: res.data, timezone: res.meta.timezone };
  }

  /** Swap the WHOLE recurring week in one call.
   *
   *  Whole-week and not per-row on purpose: a schedule is a shape a mentor reasons about
   *  all at once, and the replacement is one transaction — a half-applied edit leaves them
   *  bookable at hours they just removed. Dated overrides are untouched. */
  async function replaceWeeklyAvailability(rules: { weekday: number; start: string; end: string }[]): Promise<void> {
    // 204 No Content, so there is nothing to unwrap — `requestData` here would parse an
    // empty body, throw, and report a write that actually succeeded as a failure. Callers
    // that need the stored rows (their server-assigned ids, which the delete route takes)
    // re-read `myMentorAvailability` afterwards.
    await call('/api/v1/me/mentorship/availability/weekly', jsonBody('PUT', { rules }));
  }

  /** Add a dated exception. An equal start and end CLOSES that date, beating every other
   *  rule on it — that is how "I am away on the 16th" is one insert rather than a rewrite
   *  of the weekly schedule. */
  async function addAvailabilityOverride(date: string, start: string, end: string): Promise<MentorAvailabilityRule> {
    return requestData<MentorAvailabilityRule>(
      '/api/v1/me/mentorship/availability/overrides',
      jsonBody('POST', { date, start, end }),
    );
  }

  async function deleteAvailabilityRule(id: number): Promise<void> {
    await call(`/api/v1/me/mentorship/availability/${id}`, { method: 'DELETE' });
  }

  /** The sessions booked WITH the caller, as a mentor. Carries each seeker's address —
   *  the mentor is meeting this person, so their identity is not a leak. */
  async function myMentorBookings(): Promise<MentorSessions> {
    return requestData<MentorSessions>('/api/v1/me/mentorship/bookings');
  }

  // --- Mentor moderation (behind the moderator gate) -------------------------

  /** The pending queue, oldest first. */
  async function listPendingMentorProfiles(): Promise<PendingMentorProfile[]> {
    return requestData<PendingMentorProfile[]>('/api/v1/mentorship/profiles');
  }

  /** Approve or reject one profile. Approval is what puts it in the public directory;
   *  nothing else infers it, including an approved referral offer for the same company. */
  async function decideMentorProfile(id: number, status: 'approved' | 'rejected'): Promise<PendingMentorProfile> {
    return requestData<PendingMentorProfile>(`/api/v1/mentorship/profiles/${id}/decide`, jsonBody('POST', { status }));
  }

  return {
    listMentors,
    getMentor,
    getMentorSlots,
    bookMentorSession,
    listMySessions,
    getMySession,
    reviewMySession,
    cancelMySession,
    myMentorProfile,
    mentorProfileSuggestions,
    createMentorProfile,
    updateMentorProfile,
    pauseMentorProfile,
    withdrawMentorProfile,
    reactivateMentorProfile,
    myMentorAvailability,
    myMentorCalendar,
    replaceWeeklyAvailability,
    addAvailabilityOverride,
    deleteAvailabilityRule,
    myMentorBookings,
    listPendingMentorProfiles,
    decideMentorProfile,
  };
}

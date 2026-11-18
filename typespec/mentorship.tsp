import "@typespec/http";
import "@typespec/openapi";

using Http;
using TypeSpec.OpenAPI;

namespace Freehire;

// The mentorship marketplace: the public directory and its slots, the seeker's bookings,
// the mentor's own cabinet, and the moderation queue. The two photo routes (images) stay
// on Fiber. The schedule rows and slots reuse the models buddies.tsp declares.

/** A mentor as the PUBLIC sees them: the directory card and the profile read are one shape. */
model MentorshipMentor {
  slug: string;
  name: string;
  company_slug: string;
  company_name: string;
  headline: string;
  bio: string;
  topics: string[];
  languages: string[];
  timezone: string;
  session_minutes: int64;
  rating_count: int64;
  rating_avg: float64;
  show_photo: boolean;

  /** Absent when the mentor left it unset. */
  seniority?: string;
}

/** The moderation status of a profile. `withdrawn` is a mentor who has left; the row survives it. */
enum MentorshipProfileStatus {
  pending,
  approved,
  rejected,
  withdrawn,
}

/** A mentor as a moderator sees them: the public shape plus the moderation state, never the meeting link. */
model MentorshipModeratedMentor {
  ...MentorshipMentor;
  status: MentorshipProfileStatus;
  paused: boolean;
  created_at: utcDateTime;
}

/** A mentor as their owner sees them: the moderated shape plus the room and the session parameters. */
model MentorshipOwnMentor {
  ...MentorshipModeratedMentor;

  /** Absent while the mentor has not set one. */
  meeting_url?: string;

  buffer_before_minutes: int64;
  buffer_after_minutes: int64;
  notice_minutes: int64;
  horizon_days: int64;
}

/** A pending profile in the moderation queue. */
model MentorshipPendingMentor {
  ...MentorshipModeratedMentor;
  id: int64;
  has_approved_referral_offer: boolean;
}

/** `ignored_params` lists the query params the directory did not read. */
model MentorshipDirectoryMeta {
  count: int64;
  ignored_params?: string[];
}

model MentorshipDirectoryPage {
  data: MentorshipMentor[];
  meta: MentorshipDirectoryMeta;
}

model MentorshipCountMeta {
  count: int64;
}

model MentorshipTimezoneMeta {
  timezone: string;
  count: int64;
}

model MentorshipSlotsPage {
  data: BuddyMockSlot[];
  meta: MentorshipTimezoneMeta;
}

/** A booked session is `confirmed` or `cancelled`. */
enum MentorshipBookingStatus {
  confirmed,
  cancelled,
}

/** One session as a party to it sees it; seeker_email only for the mentor. */
model MentorshipBooking {
  id: string;
  mentor_slug: string;
  headline: string;
  starts_at: utcDateTime;
  ends_at: utcDateTime;
  status: MentorshipBookingStatus;
  note: string;
  meeting_url: string;
  seeker_email?: string;
  completed: boolean;
}

model MentorshipSessions {
  upcoming: MentorshipBooking[];
  past: MentorshipBooking[];
}

model MentorshipSessionsPage {
  data: MentorshipSessions;
  meta: MentorshipCountMeta;
}

model MentorshipReview {
  booking_id: string;
  rating: int64;
  comment: string;
}

/** `booked`: a confirmed session; `busy`: a synced interval from the mentor's calendar; `free`: offered to seekers; `closed`: outside the stated availability. */
enum MentorshipCalendarStatus {
  booked,
  busy,
  free,
  closed,
}

model MentorshipCalendarInterval {
  starts_at: utcDateTime;
  ends_at: utcDateTime;
  status: MentorshipCalendarStatus;
}

model MentorshipCalendarPage {
  data: MentorshipCalendarInterval[];
  meta: MentorshipTimezoneMeta;
}

model MentorshipRulesPage {
  data: BuddyMockRuleOut[];
  meta: MentorshipCountMeta;
}

model MentorshipBookingBody {
  /** RFC 3339. */
  starts_at?: string;

  timezone?: string;
  note?: string;
  job_id?: int64;
}

model MentorshipCancelBody {
  reason?: string;
}

model MentorshipReviewBody {
  rating?: int64;
  comment?: string;
}

model MentorshipProfileBody {
  company_slug?: string;
  slug?: string;
  name?: string;
  headline?: string;
  bio?: string;
  topics?: string[] | null;
  languages?: string[] | null;
  timezone?: string;
  session_minutes?: int64;
  buffer_before_minutes?: int64;
  buffer_after_minutes?: int64;
  notice_minutes?: int64;
  horizon_days?: int64;
  meeting_url?: string;
  show_photo?: boolean;
  seniority?: string;
}

model MentorshipPauseBody {
  paused?: boolean;
}

model MentorshipDecideBody {
  status?: string;
}

/** The public directory. */
@useAuth(NoAuth)
@tag("Mentorship")
@route("/mentors")
@get
op listMentors(
  @query company?: string,
  @query topic?: string,
  @query language?: string,
  @query seniority?: string,
  @query q?: string,
  @query no_reviews?: string,
  @query limit?: string,
): MentorshipDirectoryPage | Problem;

@useAuth(NoAuth)
@tag("Mentorship")
@route("/mentors/{slug}")
@get
op getMentor(@path slug: string): Data<MentorshipMentor> | Problem;

/** The bookable hours in a window, in the viewer's zone. */
@useAuth(NoAuth)
@tag("Mentorship")
@route("/mentors/{slug}/slots")
@get
op getMentorSlots(
  @path slug: string,
  @query from?: string,
  @query to?: string,
  @query timezone?: string,
): MentorshipSlotsPage | Problem;

@useAuth(SessionOrKey)
@tag("Mentorship")
@route("/mentors/{slug}/bookings")
@post
op bookSession(@path slug: string, @body body: MentorshipBookingBody): {
  @statusCode _: 201;
  @body body: Data<MentorshipBooking>;
} | Problem;

@useAuth(SessionOrKey)
@tag("Mentorship")
@route("/me/mentorship/sessions")
@get
op listMySessions(@query limit?: string): MentorshipSessionsPage | Problem;

@useAuth(SessionOrKey)
@tag("Mentorship")
@route("/me/mentorship/sessions/{id}")
@get
op getSession(@path id: string): Data<MentorshipBooking> | Problem;

@useAuth(SessionOrKey)
@tag("Mentorship")
@route("/me/mentorship/sessions/{id}/cancel")
@post
op cancelSession(@path id: string, @body body?: MentorshipCancelBody): Data<MentorshipBooking> | Problem;

@useAuth(SessionOrKey)
@tag("Mentorship")
@route("/me/mentorship/sessions/{id}/review")
@put
op reviewSession(@path id: string, @body body: MentorshipReviewBody): Data<MentorshipReview> | Problem;

/** The owner's own profile whatever its status; null when there is none. */
@useAuth(SessionOrKey)
@tag("Mentorship")
@route("/me/mentorship/profile")
@get
op getMyMentorProfile(): Data<MentorshipOwnMentor | null> | Problem;

@useAuth(SessionOrKey)
@tag("Mentorship")
@route("/me/mentorship/profile")
@post
op submitMentorProfile(@body body: MentorshipProfileBody): {
  @statusCode _: 201;
  @body body: Data<MentorshipOwnMentor>;
} | Problem;

@useAuth(SessionOrKey)
@tag("Mentorship")
@route("/me/mentorship/profile")
@put
op updateMentorProfile(@body body: MentorshipProfileBody): Data<MentorshipOwnMentor> | Problem;

@useAuth(SessionOrKey)
@tag("Mentorship")
@route("/me/mentorship/profile")
@delete
op withdrawMentorProfile(): NoContentResponse | Problem;

@useAuth(SessionOrKey)
@tag("Mentorship")
@route("/me/mentorship/profile/pause")
@post
op pauseMentorProfile(@body body: MentorshipPauseBody): Data<MentorshipOwnMentor> | Problem;

@useAuth(SessionOrKey)
@tag("Mentorship")
@route("/me/mentorship/profile/reactivate")
@post
op reactivateMentorProfile(): Data<MentorshipOwnMentor> | Problem;

/** The mentor's whole schedule, weekly rows and dated overrides together. */
@useAuth(SessionOrKey)
@tag("Mentorship")
@route("/me/mentorship/availability")
@get
op getMyAvailability(): MentorshipRulesPage | Problem;

/** The owner's resolved calendar for one month (`month=YYYY-MM`, default the current one). */
@useAuth(SessionOrKey)
@tag("Mentorship")
@route("/me/mentorship/availability/calendar")
@get
op getMyCalendar(@query month?: string): MentorshipCalendarPage | Problem;

@useAuth(SessionOrKey)
@tag("Mentorship")
@route("/me/mentorship/availability/weekly")
@put
op replaceWeeklyAvailability(@body body: BuddyMockWeekly): NoContentResponse | Problem;

@useAuth(SessionOrKey)
@tag("Mentorship")
@route("/me/mentorship/availability/overrides")
@post
op addAvailabilityOverride(@body body: BuddyMockRule): {
  @statusCode _: 201;
  @body body: Data<BuddyMockRuleOut>;
} | Problem;

@useAuth(SessionOrKey)
@tag("Mentorship")
@route("/me/mentorship/availability/{id}")
@delete
op deleteAvailabilityRule(@path id: string): NoContentResponse | Problem;

/** Who is coming: the mentor's own sessions, each with the seeker's address. */
@useAuth(SessionOrKey)
@tag("Mentorship")
@route("/me/mentorship/bookings")
@get
op listMentorBookings(@query limit?: string): MentorshipSessionsPage | Problem;

/** The moderation queue. Moderators only. */
@useAuth(SessionOrKey)
@extension("x-audience", "moderator")
@tag("Mentorship")
@route("/mentorship/profiles")
@get
op listPendingMentorProfiles(): {
  @body body: {
    data: MentorshipPendingMentor[];
    meta: MentorshipCountMeta;
  };
} | Problem;

@useAuth(SessionOrKey)
@extension("x-audience", "moderator")
@tag("Mentorship")
@route("/mentorship/profiles/{id}/decide")
@post
op decideMentorProfile(@path id: string, @body body: MentorshipDecideBody): Data<MentorshipModeratedMentor> | Problem;

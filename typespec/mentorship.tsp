import "@typespec/http";

using Http;

namespace Freehire;

// The mentorship marketplace: the public directory and its slots, the seeker's bookings,
// the mentor's own cabinet, and the moderation queue. The two photo routes (images) stay
// on Fiber. The schedule rows and slots reuse the models buddies.tsp declares.

/**
 * A mentor as the public, the owner and a moderator see them. The owner and moderator
 * views add status, paused and created_at; the owner alone gets meeting_url and the
 * session parameters.
 */
model MentorshipMentor {
  slug: string;
  name: string;
  company_slug: string;
  company_name: string;
  headline: string;
  bio: string;
  topics: string[] | null;
  languages: string[] | null;
  timezone: string;
  session_minutes: int64;
  rating_count: int64;
  rating_avg: float64;
  show_photo: boolean;
  seniority?: string;
  status?: string;
  paused?: boolean;
  meeting_url?: string;
  created_at?: utcDateTime;
  buffer_before_minutes?: int64;
  buffer_after_minutes?: int64;
  notice_minutes?: int64;
  horizon_days?: int64;
}

/** A pending profile in the moderation queue. */
model MentorshipPendingMentor {
  ...MentorshipMentor;
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

/** One session as a party to it sees it; seeker_email only for the mentor. */
model MentorshipBooking {
  id: string;
  mentor_slug: string;
  headline: string;
  starts_at: utcDateTime;
  ends_at: utcDateTime;
  status: string;
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

model MentorshipCalendarInterval {
  starts_at: utcDateTime;
  ends_at: utcDateTime;

  /** One of booked, busy, free or closed. */
  status: string;
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

@tag("Mentorship")
@route("/mentors/{slug}")
@get
op getMentor(@path slug: string): Data<MentorshipMentor> | Problem;

/** The bookable hours in a window, in the viewer's zone. */
@tag("Mentorship")
@route("/mentors/{slug}/slots")
@get
op getMentorSlots(
  @path slug: string,
  @query from?: string,
  @query to?: string,
  @query timezone?: string,
): MentorshipSlotsPage | Problem;

@tag("Mentorship")
@route("/mentors/{slug}/bookings")
@post
op bookSession(@path slug: string, @body body: MentorshipBookingBody): {
  @statusCode _: 201;
  @body body: Data<MentorshipBooking>;
} | Problem;

@tag("Mentorship")
@route("/me/mentorship/sessions")
@get
op listMySessions(@query limit?: string): MentorshipSessionsPage | Problem;

@tag("Mentorship")
@route("/me/mentorship/sessions/{id}")
@get
op getSession(@path id: string): Data<MentorshipBooking> | Problem;

@tag("Mentorship")
@route("/me/mentorship/sessions/{id}/cancel")
@post
op cancelSession(@path id: string, @body body?: MentorshipCancelBody): Data<MentorshipBooking> | Problem;

@tag("Mentorship")
@route("/me/mentorship/sessions/{id}/review")
@put
op reviewSession(@path id: string, @body body: MentorshipReviewBody): Data<MentorshipReview> | Problem;

/** The owner's own profile whatever its status; null when there is none. */
@tag("Mentorship")
@route("/me/mentorship/profile")
@get
op getMyMentorProfile(): Data<MentorshipMentor | null> | Problem;

@tag("Mentorship")
@route("/me/mentorship/profile")
@post
op submitMentorProfile(@body body: MentorshipProfileBody): {
  @statusCode _: 201;
  @body body: Data<MentorshipMentor>;
} | Problem;

@tag("Mentorship")
@route("/me/mentorship/profile")
@put
op updateMentorProfile(@body body: MentorshipProfileBody): Data<MentorshipMentor> | Problem;

@tag("Mentorship")
@route("/me/mentorship/profile")
@delete
op withdrawMentorProfile(): NoContentResponse | Problem;

@tag("Mentorship")
@route("/me/mentorship/profile/pause")
@post
op pauseMentorProfile(@body body: MentorshipPauseBody): Data<MentorshipMentor> | Problem;

@tag("Mentorship")
@route("/me/mentorship/profile/reactivate")
@post
op reactivateMentorProfile(): Data<MentorshipMentor> | Problem;

/** The mentor's whole schedule, weekly rows and dated overrides together. */
@tag("Mentorship")
@route("/me/mentorship/availability")
@get
op getMyAvailability(): MentorshipRulesPage | Problem;

/** The owner's resolved calendar for one month (`month=YYYY-MM`, default the current one). */
@tag("Mentorship")
@route("/me/mentorship/availability/calendar")
@get
op getMyCalendar(@query month?: string): MentorshipCalendarPage | Problem;

@tag("Mentorship")
@route("/me/mentorship/availability/weekly")
@put
op replaceWeeklyAvailability(@body body: BuddyMockWeekly): NoContentResponse | Problem;

@tag("Mentorship")
@route("/me/mentorship/availability/overrides")
@post
op addAvailabilityOverride(@body body: BuddyMockRule): {
  @statusCode _: 201;
  @body body: Data<BuddyMockRuleOut>;
} | Problem;

@tag("Mentorship")
@route("/me/mentorship/availability/{id}")
@delete
op deleteAvailabilityRule(@path id: string): NoContentResponse | Problem;

/** Who is coming: the mentor's own sessions, each with the seeker's address. */
@tag("Mentorship")
@route("/me/mentorship/bookings")
@get
op listMentorBookings(@query limit?: string): MentorshipSessionsPage | Problem;

/** The moderation queue. Moderators only. */
@tag("Mentorship")
@route("/mentorship/profiles")
@get
op listPendingMentorProfiles(): {
  @body body: {
    data: MentorshipPendingMentor[];
    meta: MentorshipCountMeta;
  };
} | Problem;

@tag("Mentorship")
@route("/mentorship/profiles/{id}/decide")
@post
op decideMentorProfile(@path id: string, @body body: MentorshipDecideBody): Data<MentorshipMentor> | Problem;

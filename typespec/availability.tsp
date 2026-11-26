import "@typespec/http";
import "@typespec/openapi";

using Http;
using TypeSpec.OpenAPI;

namespace Freehire;

// One availability calendar per user, read by mentorship, buddy mocks and booking links. The
// rule shapes are BuddyMockRule and BuddyMockRuleOut (buddies.tsp); the older mentorship and
// buddy availability routes read and write the same rows.

model AvailabilityBookingOut {
  /** Meeting length of a booking link: 15, 30, 45 or 60. */
  duration_min: int64;

  /** Shortest notice before a booking-link meeting, in hours (0 to 168). */
  notice_hours: int64;
}

model AvailabilityOut {
  /** Weekly rules and dated overrides together. */
  rules: BuddyMockRuleOut[];

  /** The zone the hours are stated in; null until settings were saved. */
  timezone: string | null;

  /** False while the user has neither hours nor settings, so booking links still use their default week (Mon-Fri 10-18, 30 min, 12 h). */
  saved: boolean;

  booking: AvailabilityBookingOut;
}

model AvailabilityWeeklyBody {
  rules?: BuddyMockRule[];

  /** IANA zone the hours are stated in; stored when present, 422 if invalid. */
  timezone?: string;
}

model AvailabilityRuleBody {
  /** 0 (Sunday) through 6; null for a dated override. */
  weekday?: int64 | null;

  /** YYYY-MM-DD; null for a weekly rule. */
  date?: string | null;

  /** HH:MM. */
  start?: string;

  /** HH:MM; 24:00 means until midnight. */
  end?: string;

  /** IANA zone the hours are stated in; stored when present, 422 if invalid. */
  timezone?: string;
}

model AvailabilityBookingBody {
  /** IANA zone the hours are stated in. */
  timezone: string;

  duration_min: int64;
  notice_hours: int64;
}

/** The caller's availability: hours, zone and booking-link settings. */
@useAuth(SessionOrKey)
@summary("Get my availability.")
@tag("Availability")
@route("/me/availability")
@get
op getAvailability(): Data<AvailabilityOut> | Problem;

/** Swap the recurring week; dated overrides stay. */
@useAuth(SessionOrKey)
@summary("Replace my weekly hours.")
@tag("Availability")
@route("/me/availability/weekly")
@put
op replaceAvailabilityWeekly(@body body: AvailabilityWeeklyBody): NoContentResponse | Problem;

/** Add one dated override: a narrowed day, an extra day, or (equal start and end) a closure. */
@useAuth(SessionOrKey)
@summary("Add a dated override.")
@tag("Availability")
@route("/me/availability/overrides")
@post
op addAvailabilityDated(@body body: AvailabilityRuleBody): {
  @statusCode _: 201;
  @body body: Data<BuddyMockRuleOut>;
} | Problem;

/** Remove one of the caller's own rules. */
@useAuth(SessionOrKey)
@summary("Delete one availability rule.")
@tag("Availability")
@route("/me/availability/{id}")
@delete
op deleteAvailabilityEntry(@path id: string): NoContentResponse | Problem;

/** Save the zone, meeting length and notice of booking links. Pro or Ultra only (402); a bad duration or notice is 422. */
@useAuth(SessionOrKey)
@summary("Save my booking-link settings.")
@tag("Availability")
@route("/me/availability/booking")
@put
op saveAvailabilityBooking(@body body: AvailabilityBookingBody): Data<AvailabilityBookingOut> | Problem;

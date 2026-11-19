import "@typespec/http";
import "@typespec/openapi";

using Http;
using TypeSpec.OpenAPI;

namespace Freehire;

// A candidate's booking link: one unguessable public page per application where a recruiter,
// with no account, books a 30-minute slot. The slots reuse the models buddies.tsp and
// mentorship.tsp declare. The public pair never names the candidate, their address or the
// application.

model BookingLinkEnableBody {
  /** The candidate's IANA zone, sent by the browser; the default hours are wall-clock times in it. */
  timezone: string;
}

model BookingLinkOut {
  /** The link's public page. The same URL on every call. */
  url: string;
}

model BookingLinkBookBody {
  /** RFC 3339; must be one of the offered slots. */
  starts_at: string;

  /** The booker's name, shown to the candidate; 1-200 characters, no control characters. */
  name: string;

  /** The booker's bare email address, shown to the candidate. */
  email: string;
}

model BookingLinkBooking {
  starts_at: utcDateTime;
  ends_at: utcDateTime;
}

/** Turn on the caller's booking link for one of their applications. Pro or Ultra only (402). */
@summary("Turn on an application's booking link.")
@opExample(#{ parameters: #{ id: 42, body: #{ timezone: "Europe/Berlin" } } })
@tag("Booking links")
@route("/me/applications/{id}/booking-link")
@post
op enableBookingLink(@path id: int64, @body body: BookingLinkEnableBody): {
  @statusCode _: 201;
  @body body: Data<BookingLinkOut>;
} | Problem;

/** A link's free slots from now to its horizon, in the viewer's zone (UTC when unknown; `meta.timezone` says which). */
@useAuth(NoAuth)
@summary("List a booking link's free slots.")
@tag("Booking links")
@route("/booking/{token}/slots")
@get
op getBookingLinkSlots(@path token: string, @query tz?: string): MentorshipSlotsPage | Problem;

/** Book one of a link's slots. A taken or no longer offered slot is 409 `slot_unavailable`. */
@useAuth(NoAuth)
@summary("Book a slot through a booking link.")
@opExample(#{
  parameters: #{
    token: "<token>",
    body: #{ starts_at: "2026-12-01T09:00:00Z", name: "Jane Recruiter", email: "jane@example.com" },
  },
})
@tag("Booking links")
@route("/booking/{token}/bookings")
@post
op bookBookingLinkSlot(@path token: string, @body body: BookingLinkBookBody): {
  @statusCode _: 201;
  @body body: Data<BookingLinkBooking>;
} | Problem;

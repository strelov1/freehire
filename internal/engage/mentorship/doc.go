// Package mentorship is the mentorship marketplace: a named, moderated insider at a
// company in the catalogue publishes when they are free, and a seeker books half an hour
// of it.
//
// Unlike internal/engage/referral, which keeps the insider unnamed, a mentor is chosen and
// so has a face. The two share the company key and nothing else.
//
// Three things are true here and nowhere else in the repository:
//
// A stored availability time carries no zone and no date. It is resolved through the
// mentor's own IANA zone, per date, and only then converted to UTC; converting once and
// repeating weekly shifts every slot by an hour for half the year.
//
// Slots are never stored: they are a pure function of the schedule, session parameters,
// busy set and current instant, so DST and boundary cases test without a database.
//
// Two confirmed bookings for one mentor cannot overlap; a Postgres EXCLUDE constraint
// guarantees it. The re-check here only gives an ordinary refusal a reason.
//
// This package takes no money; payment is an empty seam.
package mentorship

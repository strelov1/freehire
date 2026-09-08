// Package mentorship is the mentorship marketplace: a named, moderated insider at a
// company in the catalogue publishes when they are free, and a seeker books half an hour
// of it.
//
// It is the anonymous referral's opposite number and deliberately so. internal/engage/referral
// keeps the insider unnamed until they choose to reach out, because a referral is a favour
// asked of a stranger; a mentor is chosen, so a mentor has a face. The two share the
// company key and nothing else.
//
// Three things are true here and nowhere else in the repository:
//
// A stored availability time carries no zone and no date. It is resolved through the
// mentor's own IANA zone, per date, and only then converted to UTC. The other order —
// convert once, repeat weekly — moves every slot by an hour for half the year, silently,
// and no mentor could diagnose it.
//
// Slots are never stored. They are a pure function of the schedule, the session
// parameters, the busy set and the current instant, which is what makes the
// daylight-saving and boundary cases testable without a database.
//
// Two confirmed bookings for one mentor cannot overlap, and the guarantee is a Postgres
// EXCLUDE constraint rather than a re-check in this package. The re-check here exists to
// give an ordinary refusal a reason — a paused mentor, an elapsed notice period, a
// withdrawn day — not to win a race it cannot win.
//
// What this package does NOT do: it takes no money, and it reads no calendar. Both are
// named seams (see the change's design.md), and both are empty.
package mentorship

// Package slots is the availability engine shared by mentorship sessions and buddy mock
// interviews: an owner's weekly rules and dated overrides, stated in wall-clock time in
// their own IANA zone, expanded per date, with busy time subtracted, cut into sessions
// and filtered by the notice period and the horizon.
//
// It is a pure function of its inputs — no database, no clock, no owner id — which is what
// makes the daylight-saving and boundary cases testable without Postgres, and what lets a
// listed window and the re-derivation a booking runs agree. It moved here unchanged from
// internal/engage/mentorship, whose AGENTS.md still states the rules a change here keeps.
package slots

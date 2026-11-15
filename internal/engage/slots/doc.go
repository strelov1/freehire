// Package slots is the availability engine shared by mentorship sessions and buddy mock
// interviews: an owner's weekly rules and dated overrides, stated in wall-clock time in
// their own IANA zone, expanded per date, with busy time subtracted, cut into sessions
// and filtered by the notice period and the horizon.
//
// It is a pure function of its inputs (no database, no clock, no owner id), so DST and
// boundary cases test without Postgres and a listed window agrees with the booking's
// re-derivation. internal/engage/mentorship's AGENTS.md states the rules a change here keeps.
package slots

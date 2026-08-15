// Package isoweek has one function: the Monday (ISO 8601 week start) of a given
// time, at midnight UTC. Shared by insights_skill_history's writer
// (cmd/rollup-stats) and its reader (internal/handler), which both need to
// agree on what "this week" means.
package isoweek

import "time"

// Start returns the Monday (ISO 8601 week start) of t's week, at midnight UTC.
func Start(t time.Time) time.Time {
	day := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
	weekday := int(day.Weekday())
	if weekday == 0 { // Sunday
		weekday = 7
	}
	return day.AddDate(0, 0, -(weekday - 1))
}

// Package deliverywindow decides whether "now" is the right moment to send a
// notification, for an account's own local time. It is a leaf package shared by
// internal/engage/notify, reminder and nudge, so the timing math lives in one place.
package deliverywindow

import "time"

// resolveLocation maps tz to a *time.Location, falling back to UTC for an empty or
// unrecognized zone: a bad timezone must never block delivery.
func resolveLocation(tz string) *time.Location {
	loc, err := time.LoadLocation(tz)
	if tz == "" || err != nil {
		return time.UTC
	}
	return loc
}

// timeOfDay returns how far past local midnight t falls.
func timeOfDay(t time.Time) time.Duration {
	return time.Duration(t.Hour())*time.Hour +
		time.Duration(t.Minute())*time.Minute +
		time.Duration(t.Second())*time.Second
}

// InQuietHours reports whether `now`, interpreted in tz, falls inside the
// half-open window [start, end). Both start and end must be set for the
// window to be active; either nil means quiet hours are off. A window where
// start is later than end (e.g. 22:00-08:00) wraps across midnight.
func InQuietHours(now time.Time, tz string, start, end *time.Duration) bool {
	if start == nil || end == nil {
		return false
	}
	tod := timeOfDay(now.In(resolveLocation(tz)))
	if *start <= *end {
		return tod >= *start && tod < *end
	}
	return tod >= *start || tod < *end
}

// DigestDue reports whether a daily-frequency digest should fire now: local
// time has reached digestTime, and lastSentAt (if any) was on an earlier local
// calendar day. A nil digestTime is never due. Fires on the first pass after digestTime
// each local day, whatever the polling cadence.
func DigestDue(now time.Time, tz string, digestTime *time.Duration, lastSentAt *time.Time) bool {
	if digestTime == nil {
		return false
	}
	loc := resolveLocation(tz)
	local := now.In(loc)
	if timeOfDay(local) < *digestTime {
		return false
	}
	if lastSentAt == nil {
		return true
	}
	sentLocal := lastSentAt.In(loc)
	sentDay := time.Date(sentLocal.Year(), sentLocal.Month(), sentLocal.Day(), 0, 0, 0, 0, loc)
	today := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)
	return sentDay.Before(today)
}

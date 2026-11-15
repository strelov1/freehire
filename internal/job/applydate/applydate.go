package applydate

import (
	"errors"
	"time"
)

// MaxAgeDays is how old a stated apply date may be; older ones say nothing about whether a
// posting is live now.
const MaxAgeDays = 365

// ErrOutOfRange is the sentinel behind every rejection here; match it, not the message.
var ErrOutOfRange = errors.New("apply date out of range")

// rangeError carries the reason alone as its message and matches ErrOutOfRange through Is.
// Callers put the text in a 400 body, so it must read as an instruction to the person.
type rangeError struct{ reason string }

func (e *rangeError) Error() string { return e.reason }

func (e *rangeError) Is(target error) bool { return target == ErrOutOfRange }

// noonHour is the hour of the stated day an application is stored at. Midnight UTC would
// render as the previous date for every reader west of Greenwich.
//
// Bound the DAY, not the stored instant: with a noon stamp "I applied today" is in the future
// for the whole UTC morning. Validate the day first, then place it with Instant.
const noonHour = 12

// Instant places a stated day at the hour applications are stored at.
func Instant(day time.Time) time.Time {
	y, m, d := day.Date()
	return time.Date(y, m, d, noonHour, 0, 0, 0, time.UTC)
}

// Validate bounds a stated apply date to the window in which it can mean anything: not the
// future, and not so old it describes a different hiring round. It takes the DAY, never the
// instant Instant derives from it. The ghost report and the tracker's dated apply share it so
// they agree on which dates are believable.
func Validate(appliedOn, now time.Time) error {
	if appliedOn.After(now) {
		return &rangeError{reason: "the apply date is in the future"}
	}
	if now.Sub(appliedOn) > MaxAgeDays*24*time.Hour {
		return &rangeError{reason: "the apply date is more than a year ago"}
	}
	return nil
}

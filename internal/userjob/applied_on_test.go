package userjob

import (
	"strings"
	"testing"
	"time"
)

// The window a stated apply date must fall in. It lives here rather than in either caller
// because two of them now state such a date — the ghost report and the tracker's own dated
// apply — and two copies of "which dates are believable" would answer differently the first
// time one of them was tuned.
func TestValidateAppliedOn(t *testing.T) {
	now := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name    string
		day     time.Time
		wantErr bool
	}{
		{"today", now, false},
		{"yesterday", now.AddDate(0, 0, -1), false},
		{"a year ago exactly", now.AddDate(0, 0, -365), false},
		{"tomorrow", now.AddDate(0, 0, 1), true},
		{"older than a year", now.AddDate(0, 0, -366), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateAppliedOn(tc.day, now)
			if tc.wantErr && err == nil {
				t.Error("err = nil, want an error")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("err = %v, want nil", err)
			}
		})
	}
}

// The message names which bound was crossed, because it reaches the caller as the body of a
// 400 and "invalid date" tells them nothing about how to fix it.
func TestValidateAppliedOnSaysWhich(t *testing.T) {
	now := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
	if err := ValidateAppliedOn(now.AddDate(0, 0, 1), now); err == nil ||
		!strings.Contains(err.Error(), "future") {
		t.Errorf("err = %v, want it to mention the future", err)
	}
	if err := ValidateAppliedOn(now.AddDate(0, 0, -400), now); err == nil ||
		!strings.Contains(err.Error(), "year") {
		t.Errorf("err = %v, want it to mention the year bound", err)
	}
}

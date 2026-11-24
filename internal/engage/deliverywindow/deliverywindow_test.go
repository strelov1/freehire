package deliverywindow

import (
	"testing"
	"time"
)

func dur(hh, mm int) *time.Duration {
	d := time.Duration(hh)*time.Hour + time.Duration(mm)*time.Minute
	return &d
}

func TestInQuietHours(t *testing.T) {
	// A fixed UTC instant; each case supplies its own timezone to interpret it in.
	now := time.Date(2026, 8, 14, 23, 30, 0, 0, time.UTC) // 23:30 UTC

	cases := []struct {
		name       string
		tz         string
		start, end *time.Duration
		want       bool
	}{
		{"off when start/end unset", "UTC", nil, nil, false},
		{"off when only start set", "UTC", dur(22, 0), nil, false},
		{"same-day window, inside", "UTC", dur(22, 0), dur(23, 59), true},
		{"same-day window, outside", "UTC", dur(1, 0), dur(2, 0), false},
		{"overnight window, inside after start", "UTC", dur(22, 0), dur(8, 0), true},
		{"overnight window, inside before end", "UTC", dur(22, 0), dur(8, 0), true},
		{"overnight window, outside", "UTC", dur(9, 0), dur(21, 0), false},
		{"invalid timezone falls back to UTC", "Not/AZone", dur(22, 0), dur(23, 59), true},
		{"empty timezone falls back to UTC", "", dur(22, 0), dur(23, 59), true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := InQuietHours(now, c.tz, c.start, c.end); got != c.want {
				t.Errorf("InQuietHours(%v, %q, %v, %v) = %v, want %v", now, c.tz, c.start, c.end, got, c.want)
			}
		})
	}
}

func TestInQuietHoursOvernightBothSidesOfMidnight(t *testing.T) {
	start, end := dur(22, 0), dur(8, 0)

	beforeMidnight := time.Date(2026, 8, 14, 23, 0, 0, 0, time.UTC) // 23:00, after start
	if !InQuietHours(beforeMidnight, "UTC", start, end) {
		t.Error("expected 23:00 to be inside the 22:00-08:00 window")
	}

	afterMidnight := time.Date(2026, 8, 15, 3, 0, 0, 0, time.UTC) // 03:00, before end
	if !InQuietHours(afterMidnight, "UTC", start, end) {
		t.Error("expected 03:00 to be inside the 22:00-08:00 window")
	}

	midday := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	if InQuietHours(midday, "UTC", start, end) {
		t.Error("expected 12:00 to be outside the 22:00-08:00 window")
	}
}

func TestDigestDue(t *testing.T) {
	nineAM := dur(9, 0)

	t.Run("not yet due before the time boundary", func(t *testing.T) {
		now := time.Date(2026, 8, 14, 8, 59, 0, 0, time.UTC)
		if DigestDue(now, "UTC", nineAM, nil) {
			t.Error("expected not due before 09:00")
		}
	})

	t.Run("due at/after the time boundary with no prior send", func(t *testing.T) {
		now := time.Date(2026, 8, 14, 9, 0, 0, 0, time.UTC)
		if !DigestDue(now, "UTC", nineAM, nil) {
			t.Error("expected due at 09:00 with no prior send")
		}
	})

	t.Run("not due again same local day", func(t *testing.T) {
		now := time.Date(2026, 8, 14, 18, 0, 0, 0, time.UTC)
		lastSent := time.Date(2026, 8, 14, 9, 5, 0, 0, time.UTC)
		if DigestDue(now, "UTC", nineAM, &lastSent) {
			t.Error("expected not due again the same local day")
		}
	})

	t.Run("due again on the next local day", func(t *testing.T) {
		now := time.Date(2026, 8, 15, 9, 5, 0, 0, time.UTC)
		lastSent := time.Date(2026, 8, 14, 9, 5, 0, 0, time.UTC)
		if !DigestDue(now, "UTC", nineAM, &lastSent) {
			t.Error("expected due again on the next local day")
		}
	})

	t.Run("nil digest time is never due", func(t *testing.T) {
		now := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
		if DigestDue(now, "UTC", nil, nil) {
			t.Error("expected never due with no digest time configured")
		}
	})
}

package sources

import (
	"testing"
	"time"
)

func TestParseRFC3339(t *testing.T) {
	utc := func(y int, mo time.Month, d, h, mi int) *time.Time {
		x := time.Date(y, mo, d, h, mi, 0, 0, time.UTC)
		return &x
	}
	cases := []struct {
		name string
		in   string
		want *time.Time
	}{
		{"zulu with fraction", "2026-04-15T00:00:00.000Z", utc(2026, 4, 15, 0, 0)},
		{"colon offset", "2026-03-01T12:00:00-04:00", utc(2026, 3, 1, 16, 0)},             // → 16:00 UTC
		{"numeric offset no colon", "2026-06-30T15:42:00+0000", utc(2026, 6, 30, 15, 42)}, // iCIMS careers-home
		{"numeric offset with fraction", "2026-06-30T15:42:00.000+0000", utc(2026, 6, 30, 15, 42)},
		{"numeric offset nonzero", "2026-03-01T12:00:00-0400", utc(2026, 3, 1, 16, 0)},
		{"empty", "", nil},
		{"garbage", "not-a-date", nil},
	}
	for _, c := range cases {
		got := parseRFC3339(c.in)
		switch {
		case c.want == nil && got != nil:
			t.Errorf("%s: parseRFC3339(%q) = %v, want nil", c.name, c.in, got)
		case c.want != nil && got == nil:
			t.Errorf("%s: parseRFC3339(%q) = nil, want %v", c.name, c.in, c.want)
		case c.want != nil && got != nil && !got.Equal(*c.want):
			t.Errorf("%s: parseRFC3339(%q) = %v, want %v (UTC)", c.name, c.in, got.UTC(), c.want)
		}
	}
}

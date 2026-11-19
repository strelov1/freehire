package kit

import (
	"testing"
	"time"
)

// Every date helper funnels through NotFuture and answers nil for what it cannot read.
func TestDateParsers(t *testing.T) {
	want := time.Date(2026, 6, 30, 15, 42, 0, 0, time.UTC)
	day := time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		name string
		got  *time.Time
		want *time.Time
	}{
		{"rfc3339", ParseRFC3339("2026-06-30T15:42:00Z"), &want},
		{"rfc3339 without the offset colon", ParseRFC3339("2026-06-30T15:42:00+0000"), &want},
		{"rfc3339 garbage", ParseRFC3339("yesterday"), nil},
		{"rfc3339-or-date takes a timestamp", ParseRFC3339OrDate("2026-06-30T15:42:00Z"), &want},
		{"rfc3339-or-date takes a bare date", ParseRFC3339OrDate("2026-06-30"), &day},
		{"date", ParseDate("2026-06-30"), &day},
		{"space time", ParseSpaceTime("2026-06-30 15:42:00 UTC"), &want},
		{"pubDate with offset", ParsePubDate("Tue, 30 Jun 2026 15:42:00 +0000"), &want},
		{"pubDate with zone name", ParsePubDate("Tue, 30 Jun 2026 15:42:00 UTC"), &want},
		{"epoch millis", ParseEpochMillis(want.UnixMilli()), &want},
		{"epoch millis zero", ParseEpochMillis(0), nil},
		{"epoch seconds", ParseEpochSeconds(want.Unix()), &want},
		{"epoch seconds zero", ParseEpochSeconds(0), nil},
		{"future", ParseDate(time.Now().AddDate(1, 0, 0).Format("2006-01-02")), nil},
	}
	for _, tc := range cases {
		switch {
		case tc.want == nil && tc.got != nil:
			t.Errorf("%s: got %v, want nil", tc.name, tc.got)
		case tc.want != nil && (tc.got == nil || !tc.got.Equal(*tc.want)):
			t.Errorf("%s: got %v, want %v", tc.name, tc.got, tc.want)
		}
	}
}

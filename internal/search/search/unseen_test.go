package search

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/platform/db"
)

func seenAt(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t, Valid: true} }

func TestUnseenWindowsUnseen(t *testing.T) {
	now := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	windows := UnseenWindows{"greenhouse": 14 * 24 * time.Hour, "builtin": 30 * 24 * time.Hour}

	cases := []struct {
		name string
		job  db.Job
		want bool
	}{
		{"seen yesterday", db.Job{Source: "greenhouse", LastSeenAt: seenAt(now.Add(-24 * time.Hour))}, false},
		{"exactly at the window", db.Job{Source: "greenhouse", LastSeenAt: seenAt(now.Add(-14 * 24 * time.Hour))}, false},
		{"past the window", db.Job{Source: "greenhouse", LastSeenAt: seenAt(now.Add(-15 * 24 * time.Hour))}, true},
		{"inside a wider window of its own", db.Job{Source: "builtin", LastSeenAt: seenAt(now.Add(-20 * 24 * time.Hour))}, false},
		{"past a wider window of its own", db.Job{Source: "builtin", LastSeenAt: seenAt(now.Add(-31 * 24 * time.Hour))}, true},
		// A source with no window is one nothing re-crawls (telegram, an employer's own
		// vacancy) or one whose unseen reading is not evidence: never withheld for age.
		{"source with no window", db.Job{Source: "telegram", LastSeenAt: seenAt(now.Add(-90 * 24 * time.Hour))}, false},
		{"never stamped", db.Job{Source: "greenhouse"}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := windows.Unseen(c.job, now); got != c.want {
				t.Errorf("Unseen() = %v, want %v", got, c.want)
			}
		})
	}
}

func TestWithheld(t *testing.T) {
	now := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	windows := UnseenWindows{"greenhouse": UnseenWindow}
	showable := db.Job{Source: "greenhouse", Category: "backend", Description: "<p>Build things.</p>", LastSeenAt: seenAt(now)}

	if Withheld(showable, now, windows) {
		t.Fatalf("a categorised, described, freshly seen job was withheld")
	}

	stale := showable
	stale.LastSeenAt = seenAt(now.Add(-UnseenWindow - time.Hour))
	if !Withheld(stale, now, windows) {
		t.Errorf("a job its source stopped showing %v ago was not withheld", UnseenWindow+time.Hour)
	}

	bodiless := showable
	bodiless.Description = ""
	if !Withheld(bodiless, now, windows) {
		t.Errorf("a job with no body was not withheld")
	}

	uncategorised := showable
	uncategorised.Category = ""
	uncategorised.Title = "Shelf Stocker"
	if !Withheld(uncategorised, now, windows) {
		t.Errorf("a job with no category was not withheld")
	}

	if Withheld(stale, now, nil) {
		t.Errorf("no windows must mean no unseen rule")
	}
}

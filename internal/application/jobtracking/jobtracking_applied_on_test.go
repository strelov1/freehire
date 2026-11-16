package jobtracking_test

import (
	"errors"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/application/appevent"
	"github.com/strelov1/freehire/internal/application/jobtracking"
	"github.com/strelov1/freehire/internal/job/applydate"
)

// The candidate stating the day beats anything we inferred, so this path overwrites a date
// already recorded — the opposite of MarkAppliedAt, whose date is read off employer mail and is
// only an upper bound. The two therefore reach different repository methods.
func TestMarkAppliedOn_OverwritesADateAlreadyRecorded(t *testing.T) {
	sent := time.Date(2026, 7, 3, 12, 0, 0, 0, time.UTC)
	now := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
	repo := newRepo()
	repo.appliedResult = jobtracking.Interaction{JobID: jobID, AppliedAt: tPtr(sent)}
	svc := jobtracking.New(repo)

	if _, err := svc.MarkAppliedOn(ctx(), userID, slug, sent, now, appevent.SourceUser); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !repo.redatedAt.Equal(sent) {
		t.Errorf("repository re-dated to %v, want the stated %v", repo.redatedAt, sent)
	}
}

// The window is checked here rather than at the HTTP door, so the CLI and the in-app assistant —
// which call this service directly and never pass through Fiber — are bounded by the same rule.
func TestMarkAppliedOn_RefusesADateOutsideTheWindow(t *testing.T) {
	now := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
	cases := map[string]time.Time{
		"tomorrow":          now.AddDate(0, 0, 1),
		"older than a year": now.AddDate(0, 0, -400),
	}
	for name, day := range cases {
		t.Run(name, func(t *testing.T) {
			repo := newRepo()
			svc := jobtracking.New(repo)

			_, err := svc.MarkAppliedOn(ctx(), userID, slug, day, now, appevent.SourceUser)
			if !errors.Is(err, applydate.ErrOutOfRange) {
				t.Errorf("err = %v, want ErrAppliedOnOutOfRange", err)
			}
			if !repo.redatedAt.IsZero() {
				t.Error("an unbelievable date reached the repository")
			}
		})
	}
}

// Stating today must work at every hour of the day. The day is stored at noon UTC, so validating
// that derived instant rather than the day itself refuses "today" for the whole UTC morning —
// and, for a caller east of UTC+12, refuses their today permanently. The window bounds the day a
// person names; the hour the row happens to be stored at is not part of it.
func TestMarkAppliedOn_AcceptsTodayAtAnyHour(t *testing.T) {
	day := time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC)
	for _, hour := range []int{0, 6, 11, 12, 23} {
		now := time.Date(2026, 8, 2, hour, 0, 0, 0, time.UTC)
		repo := newRepo()
		repo.appliedResult = jobtracking.Interaction{JobID: jobID, AppliedAt: tPtr(day)}
		svc := jobtracking.New(repo)

		if _, err := svc.MarkAppliedOn(ctx(), userID, slug, day, now, appevent.SourceUser); err != nil {
			t.Errorf("now=%02d:00 UTC: %v", hour, err)
		}
	}
}

// The day reaches storage as noon UTC. Midnight would render as the previous date for every
// reader west of Greenwich, so the card would show a day earlier than the one they typed.
func TestMarkAppliedOn_StoresTheDayAtNoon(t *testing.T) {
	day := time.Date(2026, 7, 27, 0, 0, 0, 0, time.UTC)
	now := time.Date(2026, 8, 2, 9, 0, 0, 0, time.UTC)
	repo := newRepo()
	svc := jobtracking.New(repo)

	if _, err := svc.MarkAppliedOn(ctx(), userID, slug, day, now, appevent.SourceUser); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
	if !repo.redatedAt.Equal(want) {
		t.Errorf("repository received %v, want %v", repo.redatedAt, want)
	}
}

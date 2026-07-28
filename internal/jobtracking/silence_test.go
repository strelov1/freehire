package jobtracking_test

import (
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/jobtracking"
	"github.com/strelov1/freehire/internal/userjob"
)

// tracked builds a tracked row with the silence inputs the repository supplies.
func tracked(stage string, appliedAt, lastActivity *time.Time, pending bool) jobtracking.TrackedJob {
	return jobtracking.TrackedJob{
		Interaction: jobtracking.Interaction{
			AppliedAt: appliedAt,
			Stage:     &stage,
		},
		LastActivityAt:       lastActivity,
		HasPendingSuggestion: pending,
	}
}

// A row that is not an application is not waiting on anyone, so it derives no
// silence at all — the UI must be able to tell "nothing owed" from "owed and
// fine", which a zero-day active state would blur.
func TestSilenceNilForNonApplications(t *testing.T) {
	now := time.Now()
	ago := now.Add(-40 * 24 * time.Hour)

	if got := tracked("", nil, nil, false).Silence(now); got != nil {
		t.Errorf("saved-only row derived %+v, want nil", got)
	}
	// Applied but with a settled stage: still nothing to report.
	for _, stage := range []string{"rejected", "accepted", "withdrawn"} {
		if got := tracked(stage, &ago, &ago, false).Silence(now); got != nil {
			t.Errorf("stage %q derived %+v, want nil", stage, got)
		}
	}
}

// Days silent counts whole elapsed days from the last activity, and the state is
// the stage's verdict on that number.
func TestSilenceCountsWholeDays(t *testing.T) {
	now := time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC)
	applied := now.Add(-30 * 24 * time.Hour)

	cases := []struct {
		name      string
		stage     string
		last      time.Time
		wantDays  int
		wantState string
	}{
		{"fresh application", "applied", now.Add(-10 * 24 * time.Hour), 10, userjob.SilenceActive},
		{"part of a day does not count", "applied", now.Add(-10*24*time.Hour - 13*time.Hour), 10, userjob.SilenceActive},
		{"exactly at the threshold", "applied", now.Add(-21 * 24 * time.Hour), 21, userjob.SilenceActive},
		{"past the threshold", "applied", now.Add(-22 * 24 * time.Hour), 22, userjob.SilenceSilent},
		{"same wait, later stage", "interview", now.Add(-13 * 24 * time.Hour), 13, userjob.SilenceSilent},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := tracked(c.stage, &applied, &c.last, false).Silence(now)
			if got == nil {
				t.Fatal("derived nil, want a silence")
			}
			if got.DaysSilent != c.wantDays {
				t.Errorf("DaysSilent = %d, want %d", got.DaysSilent, c.wantDays)
			}
			if got.State != c.wantState {
				t.Errorf("State = %q, want %q", got.State, c.wantState)
			}
			if !got.LastActivityAt.Equal(c.last) {
				t.Errorf("LastActivityAt = %v, want %v", got.LastActivityAt, c.last)
			}
		})
	}
}

// Mail awaiting confirmation contradicts the claim that nobody replied, so the
// verdict softens into a question — but only where there was a verdict to soften.
func TestSilencePendingSuggestion(t *testing.T) {
	now := time.Now()
	applied := now.Add(-30 * 24 * time.Hour)

	past := now.Add(-22 * 24 * time.Hour)
	if got := tracked("applied", &applied, &past, true).Silence(now); got == nil || got.State != userjob.SilenceUnconfirmed {
		t.Errorf("past the threshold with a pending suggestion = %+v, want %q", got, userjob.SilenceUnconfirmed)
	}
	inside := now.Add(-3 * 24 * time.Hour)
	if got := tracked("applied", &applied, &inside, true).Silence(now); got == nil || got.State != userjob.SilenceActive {
		t.Errorf("inside the threshold with a pending suggestion = %+v, want %q", got, userjob.SilenceActive)
	}
}

// An application whose last activity the repository could not supply falls back
// to its apply date rather than deriving nothing — the SQL already guarantees
// this, and the domain must not reintroduce a hole the query closed.
func TestSilenceFallsBackToAppliedAt(t *testing.T) {
	now := time.Now()
	applied := now.Add(-25 * 24 * time.Hour)

	got := tracked("applied", &applied, nil, false).Silence(now)
	if got == nil {
		t.Fatal("derived nil for an application with no last activity, want a silence from applied_at")
	}
	if got.DaysSilent != 25 {
		t.Errorf("DaysSilent = %d, want 25", got.DaysSilent)
	}
	if got.State != userjob.SilenceSilent {
		t.Errorf("State = %q, want %q", got.State, userjob.SilenceSilent)
	}
}

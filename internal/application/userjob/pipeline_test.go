package userjob

import (
	"slices"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/job/silence"
)

// TestEveryStageIsRankedOrTerminal is the direction the old cross-package test lacked. It checked
// that every stage mailclassify named was a real stage; nothing checked the reverse, so a stage
// added to Stages was silently unranked — rank 0, below `applied`, absent from the terminal set.
// Any forward signal would then advance an application BACKWARD out of it.
//
// Exactly one of the two, never both and never neither: a stage is either a position an
// application passes through or an outcome it ends at.
func TestEveryStageIsRankedOrTerminal(t *testing.T) {
	for _, stage := range Stages {
		_, ranked := activeRank[stage]
		isTerminal := terminalStages[stage]
		switch {
		case ranked && isTerminal:
			t.Errorf("stage %q is both ranked and terminal — an application cannot both pass "+
				"through it and end at it", stage)
		case !ranked && !isTerminal:
			t.Errorf("stage %q is neither ranked nor terminal. Add it to activeRank (a position "+
				"the pipeline passes through) or to terminalStages (a settled outcome). Left out "+
				"of both it ranks 0, below `applied`, so any forward signal advances an "+
				"application backward out of it.", stage)
		}
	}

	// And nothing ranked or terminal may be missing from the vocabulary itself.
	for stage := range activeRank {
		if !ValidStage(stage) {
			t.Errorf("activeRank has %q, which is not in Stages", stage)
		}
	}
	for stage := range terminalStages {
		if !ValidStage(stage) {
			t.Errorf("terminalStages has %q, which is not in Stages", stage)
		}
	}
}

// The silence thresholds answer the same question the ranks do — is this application still in
// flight — so a stage that accrues silence is exactly a stage with a rank. Adding an active
// stage without a threshold makes it tolerate silence forever; adding a threshold without a rank
// makes it unreachable.
func TestSilenceThresholdsCoverExactlyTheActiveStages(t *testing.T) {
	// The ladder lives in internal/job/silence, below this package, because the ghost signal
	// needs it too. The invariant tying it to the stage pipeline stays here, where
	// activeRank is: a stage that advances but tolerates no measured silence would never
	// report as silent, and a threshold for a stage that does not exist is dead weight.
	covered := silence.Stages()
	for stage := range activeRank {
		if !slices.Contains(covered, stage) {
			t.Errorf("active stage %q has no silence threshold, so it never reports as silent", stage)
		}
	}
	for _, stage := range covered {
		if _, ok := activeRank[stage]; !ok {
			t.Errorf("silence.Stages has %q, which is not an active stage", stage)
		}
	}
}

func TestForward(t *testing.T) {
	tests := []struct {
		name            string
		current, target string
		want            bool
	}{
		{"seeds from empty", "", "applied", true},
		{"advances one step", "applied", "screening", true},
		{"advances several steps", "applied", "offer", true},
		{"refuses backward", "interview", "screening", false},
		{"refuses sideways", "interview", "interview", false},
		{"never resurrects a rejection", "rejected", "interview", false},
		{"never resurrects an acceptance", "accepted", "offer", false},
		{"never advances INTO a terminal outcome", "interview", "rejected", false},
		// A reply that arrives after the candidate gave up must not reopen the application:
		// they concluded no answer was coming, and a late classification is not a reversal of
		// that. The rule costs nothing to extend — it follows from `expired` being terminal.
		{"never resurrects an expired application", "expired", "responded", false},
		{"never advances INTO expired", "applied", "expired", false},
		{"refuses an unknown target", "applied", "definitely-not-a-stage", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Forward(tt.current, tt.target); got != tt.want {
				t.Errorf("Forward(%q, %q) = %v, want %v", tt.current, tt.target, got, tt.want)
			}
		})
	}
}

// DaysSilent is whole days, floored at zero. It was three separate copies — the tracking board,
// the follow-up gate and the ghost signal — each with its own spelling of the same arithmetic,
// held together by comments naming one another. The ladder above it was already shared, so a
// day's disagreement between the badge and the offer was the one thing left that could split
// them.
func TestDaysSilent(t *testing.T) {
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		last time.Time
		want int
	}{
		{"same instant", now, 0},
		{"a part-day does not count", now.Add(-23 * time.Hour), 0},
		{"exactly one day", now.Add(-24 * time.Hour), 1},
		{"a day and a half is one day", now.Add(-36 * time.Hour), 1},
		{"three weeks", now.AddDate(0, 0, -21), 21},
		// Clock skew, or a stamp a moment in the future, must not report negative silence.
		{"the future is not negative silence", now.Add(2 * time.Hour), 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := silence.Days(now, tt.last); got != tt.want {
				t.Errorf("silence.Days(%v) = %d, want %d", tt.last, got, tt.want)
			}
		})
	}
}

func TestEmployerMay(t *testing.T) {
	cases := []struct {
		name       string
		current    string
		byEmployer bool
		target     string
		want       bool
	}{
		{"advances an applied candidate to interview", "applied", false, "interview", true},
		{"seeds a record with no stage", "", false, "screening", true},
		{"rejects an active application", "interview", false, "rejected", true},
		{"announces a hire", "offer", false, "accepted", true},
		{"never moves the candidate's own stage backward", "interview", false, "screening", false},
		{"never reopens a withdrawal", "withdrawn", false, "interview", false},
		{"never overrides an outcome the candidate recorded", "rejected", false, "offer", false},
		{"corrects its own rejection", "rejected", true, "interview", true},
		{"moves its own stage backward", "interview", true, "screening", true},
		{"no-op on the same stage", "interview", true, "interview", false},
		{"refuses a stage outside the vocabulary", "applied", false, "hired", false},
	}
	for _, c := range cases {
		if got := EmployerMay(c.current, c.byEmployer, c.target); got != c.want {
			t.Errorf("%s: EmployerMay(%q, %v, %q) = %v, want %v", c.name, c.current, c.byEmployer, c.target, got, c.want)
		}
	}
}

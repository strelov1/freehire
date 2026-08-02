package userjob

import "testing"

// Every active stage has a threshold, terminal stages have none, and an unset
// stage is judged as `applied` — an application with no stage recorded is still
// an application.
func TestSilenceThresholdDays(t *testing.T) {
	cases := []struct {
		stage    string
		wantDays int
		wantOK   bool
	}{
		{"applied", 21, true},
		{"screening", 18, true},
		{"responded", 15, true},
		{"interview", 12, true},
		{"offer", 5, true},
		{"", 21, true}, // unset reads as applied
		{"accepted", 0, false},
		{"rejected", 0, false},
		{"withdrawn", 0, false},
		{"banana", 0, false},
	}
	for _, c := range cases {
		days, ok := SilenceThresholdDays(c.stage)
		if ok != c.wantOK || days != c.wantDays {
			t.Errorf("SilenceThresholdDays(%q) = (%d, %v), want (%d, %v)",
				c.stage, days, ok, c.wantDays, c.wantOK)
		}
	}
}

// The ladder must never invert: an application that has advanced is waiting on a
// conversation already underway, and tolerating *more* silence there than at the
// application stage would invert the whole point of the feature.
func TestSilenceThresholdsGrowStricter(t *testing.T) {
	ordered := []string{"applied", "screening", "responded", "interview", "offer"}
	prev := 1 << 30
	for _, stage := range ordered {
		days, ok := SilenceThresholdDays(stage)
		if !ok {
			t.Fatalf("stage %q has no threshold", stage)
		}
		if days >= prev {
			t.Errorf("threshold for %q is %d, want less than the previous stage's %d",
				stage, days, prev)
		}
		prev = days
	}
}

// A threshold is the last tolerated day, not the first offending one: at exactly
// the threshold the application is still active.
func TestSilenceStateForBoundary(t *testing.T) {
	cases := []struct {
		name  string
		stage string
		days  int
		want  string
	}{
		{"well inside", "applied", 10, SilenceActive},
		{"exactly at the threshold", "applied", 21, SilenceActive},
		{"one day past", "applied", 22, SilenceSilent},
		{"interview well past", "interview", 18, SilenceSilent},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := SilenceStateFor(c.stage, c.days, false); got != c.want {
				t.Errorf("SilenceStateFor(%q, %d, false) = %q, want %q",
					c.stage, c.days, got, c.want)
			}
		})
	}
}

// The same elapsed silence means different things at different stages — the
// reason the ladder exists at all.
func TestSilenceStateForIsStageAware(t *testing.T) {
	const days = 13
	if got := SilenceStateFor("applied", days, false); got != SilenceActive {
		t.Errorf("applied at %d days = %q, want %q", days, got, SilenceActive)
	}
	if got := SilenceStateFor("interview", days, false); got != SilenceSilent {
		t.Errorf("interview at %d days = %q, want %q", days, got, SilenceSilent)
	}
}

// A settled application is not waiting on anyone, so it reports no state at all
// rather than a reassuring "active" — the caller must be able to tell "not
// waiting" from "waiting and fine".
func TestSilenceStateForTerminalStages(t *testing.T) {
	for _, stage := range []string{"accepted", "rejected", "withdrawn"} {
		if got := SilenceStateFor(stage, 365, false); got != "" {
			t.Errorf("SilenceStateFor(%q, 365, false) = %q, want the empty state", stage, got)
		}
		if got := SilenceStateFor(stage, 365, true); got != "" {
			t.Errorf("SilenceStateFor(%q, 365, true) = %q, want the empty state", stage, got)
		}
	}
}

// A pending suggestion only ever softens a silence claim into a question. It
// must not raise one: mail awaiting confirmation on an application that is
// answering promptly is not a problem to report.
func TestSilenceStateForPendingSuggestion(t *testing.T) {
	if got := SilenceStateFor("applied", 22, true); got != SilenceUnconfirmed {
		t.Errorf("past threshold with a pending suggestion = %q, want %q", got, SilenceUnconfirmed)
	}
	if got := SilenceStateFor("applied", 10, true); got != SilenceActive {
		t.Errorf("inside threshold with a pending suggestion = %q, want %q", got, SilenceActive)
	}
}

// An expired application waits on nobody, so it reports no silence state at all — not `active`,
// which would read as "waiting and fine". This is what stops the new stage from becoming a
// second name for silence: the moment the candidate records the outcome, the clock goes quiet.
func TestExpiredReportsNoSilenceState(t *testing.T) {
	for _, days := range []int{0, 21, 400} {
		if got := SilenceStateFor("expired", days, false); got != "" {
			t.Errorf("SilenceStateFor(\"expired\", %d, false) = %q, want \"\"", days, got)
		}
	}
}

package userjob

import "testing"

func TestStagesOrder(t *testing.T) {
	want := []string{"applied", "screening", "responded", "interview", "offer", "accepted", "rejected", "withdrawn", "expired"}
	if len(Stages) != len(want) {
		t.Fatalf("len(Stages) = %d, want %d", len(Stages), len(want))
	}
	for i, s := range want {
		if Stages[i] != s {
			t.Errorf("Stages[%d] = %q, want %q", i, Stages[i], s)
		}
	}
}

func TestValidStage(t *testing.T) {
	for _, s := range Stages {
		if !ValidStage(s) {
			t.Errorf("ValidStage(%q) = false, want true", s)
		}
	}
	if ValidStage("bogus") {
		t.Error("ValidStage(\"bogus\") = true, want false")
	}
	if ValidStage("") {
		t.Error("ValidStage(\"\") = true, want false")
	}
}

// `expired` is the outcome for an application nobody answered — the employer never replied, or
// the posting went away. It is settled, so it takes no rank and no silence threshold, and it
// shows in `closed` beside the other three endings under its own label.
//
// The stage exists because the vocabulary had no word for the most common ending: `rejected`
// says the employer decided, `withdrawn` says the candidate did, and neither is true of silence.
func TestExpiredIsASettledOutcomeInClosed(t *testing.T) {
	if !ValidStage("expired") {
		t.Fatal("ValidStage(\"expired\") = false, want true")
	}
	if !IsTerminal("expired") {
		t.Error("IsTerminal(\"expired\") = false: an application does not pass through it")
	}
	if got := GroupOf("expired"); got != "closed" {
		t.Errorf("GroupOf(\"expired\") = %q, want \"closed\"", got)
	}
	if got := Label("expired"); got != "Expired" {
		t.Errorf("Label(\"expired\") = %q, want \"Expired\"", got)
	}
	if _, ok := SilenceThresholdDays("expired"); ok {
		t.Error("SilenceThresholdDays(\"expired\") reports a threshold: a settled application waits on nobody")
	}
}

package mailclassify

import (
	"testing"

	"github.com/strelov1/freehire/internal/userjob"
)

// TestStageTargetsAreValidStages guards the coupling: every stage this package
// maps a signal to must be a real user_jobs stage, so the vocabulary can't drift
// out from under the tracking pipeline.
func TestStageTargetsAreValidStages(t *testing.T) {
	for sig, stage := range signalStage {
		if !userjob.ValidStage(stage) {
			t.Errorf("signal %q maps to invalid stage %q", sig, stage)
		}
	}
	for stage := range stageOrder {
		if !userjob.ValidStage(stage) {
			t.Errorf("stageOrder has invalid stage %q", stage)
		}
	}
}

func TestSanitizeCoercesOutOfVocabulary(t *testing.T) {
	got := Classification{Signal: "definitely-not-a-signal", Confidence: 0.7}.Sanitize()
	if got.Signal != SignalOther {
		t.Fatalf("out-of-vocabulary signal = %q, want %q", got.Signal, SignalOther)
	}
}

func TestSanitizeKeepsKnownSignal(t *testing.T) {
	got := Classification{Signal: SignalInterviewInvitation, Confidence: 0.9}.Sanitize()
	if got.Signal != SignalInterviewInvitation {
		t.Fatalf("known signal = %q, want it preserved", got.Signal)
	}
}

func TestSanitizeClampsConfidence(t *testing.T) {
	cases := []struct{ in, want float64 }{
		{1.5, 1.0}, {-0.3, 0.0}, {0.5, 0.5},
	}
	for _, c := range cases {
		got := Classification{Signal: SignalOffer, Confidence: c.in}.Sanitize()
		if got.Confidence != c.want {
			t.Fatalf("clamp(%v) = %v, want %v", c.in, got.Confidence, c.want)
		}
	}
}

func TestAdvanceStage(t *testing.T) {
	cases := []struct {
		name      string
		current   string
		signal    StatusSignal
		wantStage string
		wantOK    bool
	}{
		{"forward from applied to interview", "applied", SignalInterviewInvitation, "interview", true},
		{"forward from empty stage", "", SignalInterviewInvitation, "interview", true},
		{"backward acknowledgement after interview is ignored", "interview", SignalAcknowledgement, "", false},
		{"backward interview after offer is ignored", "offer", SignalInterviewInvitation, "", false},
		{"rejection never auto-advances", "screening", SignalRejection, "", false},
		{"other never advances", "applied", SignalOther, "", false},
		{"offer advances from interview", "interview", SignalOffer, "offer", true},
		{"terminal rejected is never resurrected", "rejected", SignalAcknowledgement, "", false},
		{"terminal accepted is never moved", "accepted", SignalOffer, "", false},
		{"terminal withdrawn is never moved", "withdrawn", SignalInterviewInvitation, "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			stage, ok := AdvanceStage(c.current, c.signal)
			if stage != c.wantStage || ok != c.wantOK {
				t.Fatalf("AdvanceStage(%q, %q) = (%q, %v), want (%q, %v)",
					c.current, c.signal, stage, ok, c.wantStage, c.wantOK)
			}
		})
	}
}

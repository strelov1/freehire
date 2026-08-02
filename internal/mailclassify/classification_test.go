package mailclassify

import (
	"testing"

	"github.com/strelov1/freehire/internal/userjob"
)

// TestStageTargetsAreValidStages guards what remains of the coupling: every stage this package
// maps a signal to must be a real user_jobs stage, so the mail vocabulary cannot drift out from
// under the tracking pipeline.
//
// The other direction — every tracking stage is ranked or terminal — now lives in internal/userjob
// with the rule itself, which is the point of the move. It used to be missing entirely, and that
// gap is what let a stage inserted into Stages rank below `applied`.
func TestStageTargetsAreValidStages(t *testing.T) {
	for sig, imp := range signalStage {
		if !userjob.ValidStage(imp.Stage) {
			t.Errorf("signal %q maps to invalid stage %q", sig, imp.Stage)
		}
		// A terminal stage may be IMPLIED — a rejection email plainly means `rejected`, and
		// saying so is how the reader learns why nothing moved. It may never be ADVANCED to:
		// deciding an application is settled stays the candidate's, made by pressing a button.
		if imp.Advances && userjob.IsTerminal(imp.Stage) {
			t.Errorf("signal %q auto-advances to the terminal stage %q; deciding an application "+
				"is settled is never an inference from mail", sig, imp.Stage)
		}
	}
}

// Everything the reader is told about a signal comes from this one table, so a signal missing
// from it renders as a bare label with nothing said about the stage. `other` is the deliberate
// exception: it means "we could not tell", which implies no stage at all.
func TestStageForCoversEverySignal(t *testing.T) {
	for _, s := range SignalValues {
		sig := StatusSignal(s)
		stage, advances := StageFor(sig)
		switch sig {
		case SignalOther, SignalInfoRequest, SignalIncompleteApplication:
			if stage != "" || advances {
				t.Errorf("StageFor(%q) = (%q, %v), want no implied stage", sig, stage, advances)
			}
		default:
			if stage == "" {
				t.Errorf("StageFor(%q) implies no stage; the reader would see a label and no "+
					"explanation of what it means for the application", sig)
			}
		}
	}
}

// The suggestion exists because this is true: a rejection says what it says, and still moves
// nothing by itself.
func TestARejectionImpliesRejectedButNeverAdvances(t *testing.T) {
	stage, advances := StageFor(SignalRejection)
	if stage != "rejected" {
		t.Errorf("StageFor(rejection) stage = %q, want %q", stage, "rejected")
	}
	if advances {
		t.Error("StageFor(rejection) advances = true, want false")
	}
	if got, ok := AdvanceStage("screening", SignalRejection); ok || got != "" {
		t.Errorf("AdvanceStage(screening, rejection) = (%q, %v), want (\"\", false)", got, ok)
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

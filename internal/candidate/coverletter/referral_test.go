package coverletter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/strelov1/freehire/internal/candidate/experience"
	"github.com/strelov1/freehire/internal/platform/llm"
)

type referralTracer struct{ calls int }

func (t *referralTracer) Observe(llm.Generation)         { t.calls++ }
func (t *referralTracer) Shutdown(context.Context) error { return nil }

func TestReferralDraftUsesPublishableEvidenceAndDoesNotTraceProfile(t *testing.T) {
	kept := manualAtom("I cut build time by 30 percent")
	inferred := experience.Atom{ID: uuid.New(), Claim: "MODEL-INVENTED-CERTIFICATION", Provenance: experience.ProvenanceAgentInferred}
	body := "Hello Sam, I noticed your work at Acme. I am applying for the engineering role, and I cut build time by 30 percent in a previous project. If you feel comfortable, would you consider referring me? No pressure either way."
	model := &scriptedModel{responses: []string{
		fmt.Sprintf(`{"body":%q,"cited_atom_ids":[%q]}`, body, kept.ID.String()),
	}}
	trace := &referralTracer{}
	in := testInput([]experience.Atom{kept, inferred})
	in.Referral = &Referral{Channel: "linkedin", Profile: "Sam, engineer at Acme. PUBLIC-RECIPIENT-PROFILE"}
	draft, err := NewAnalyzer(llm.NewWithModel(model, llm.WithTracer(trace, "test"))).Draft(context.Background(), in)
	if err != nil || draft == nil {
		t.Fatalf("Draft = (%v, %v)", draft, err)
	}
	if trace.calls != 0 {
		t.Fatal("request-only contact was traced")
	}
	if strings.Contains(model.sent(), inferred.Claim) {
		t.Fatal("model-inferred achievement reached the prompt")
	}
	if !strings.Contains(model.sent(), "PUBLIC-RECIPIENT-PROFILE") {
		t.Fatal("the profile never reached the drafting chain")
	}
	if len(draft.Cited) != 1 || draft.Cited[0] != kept.ID {
		t.Fatalf("cited = %v", draft.Cited)
	}
	if model.calls != 1 {
		t.Fatalf("model calls = %d, want one", model.calls)
	}
}

// A model citation cannot authenticate an atom that was not among the ranked offer.
func TestReferralDraftRejectsUncitedAndUnrankedEvidence(t *testing.T) {
	atoms := []experience.Atom{
		manualAtom("first"), manualAtom("second"), manualAtom("third"),
		manualAtom("fourth"), manualAtom("fifth"),
	}
	for _, tt := range []struct {
		name  string
		cited []string
	}{
		{name: "no citation"},
		{name: "outside top four", cited: []string{atoms[4].ID.String()}},
		{name: "mixed valid and unknown", cited: []string{atoms[0].ID.String(), atoms[4].ID.String()}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cited, err := json.Marshal(tt.cited)
			if err != nil {
				t.Fatal(err)
			}
			response := fmt.Sprintf(`{"body":%q,"cited_atom_ids":%s}`, "Hello Sam. I am interested in the role and would appreciate a referral if you feel comfortable. Thank you for considering it.", cited)
			model := &scriptedModel{responses: []string{response}}
			in := testInput(atoms)
			in.Referral = &Referral{Channel: "linkedin", Profile: "Sam at Acme"}
			got, err := NewAnalyzer(llm.NewWithModel(model)).Draft(context.Background(), in)
			if !errors.Is(err, errInvalidReferralDraft) || got != nil {
				t.Fatalf("Draft = (%v, %v), want an invalid draft", got, err)
			}
			if strings.Contains(model.sent(), atoms[4].Claim) {
				t.Fatal("fifth achievement reached the model")
			}
			if model.calls != 1 {
				t.Fatalf("model calls = %d, want one", model.calls)
			}
		})
	}
}

func TestReferralInputRejectsUnknownChannelAndUnboundedProfile(t *testing.T) {
	for _, r := range []Referral{
		{Channel: "sms", Profile: "Sam"},
		{Channel: "email", Profile: "   "},
		{Channel: "linkedin", Profile: strings.Repeat("x", MaxReferralProfileRunes+1)},
	} {
		if r.Valid() {
			t.Fatalf("unexpected valid referral: channel=%q length=%d", r.Channel, len(r.Profile))
		}
	}
}

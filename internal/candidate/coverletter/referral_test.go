package coverletter

import (
	"context"
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
		fmt.Sprintf(`{"selected":[%q]}`, kept.ID.String()),
		fmt.Sprintf(`{"body":%q}`, body),
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

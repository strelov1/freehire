package coverletter

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/strelov1/freehire/internal/candidate/experience"
	"github.com/strelov1/freehire/internal/platform/llm"
)

// The spec authorises exactly ONE degradation at stage 3: an answer that cannot be PARSED. A
// gateway failure is a different thing, and R9 says a failing model returns no letter at all —
// otherwise a network blip silently skips the only stage that cuts unsupported claims, and the
// result is stored as though it had been audited.
func TestGatewayFailureDuringTheAuditFailsTheDraft(t *testing.T) {
	a := manualAtom("shipped it")
	model := &scriptedModel{responses: []string{
		`{"selected":["` + a.ID.String() + `"]}`,
		`{"body":"DRAFTED ` + longBody(400) + `"}`,
	}} // no third response: the audit call errors

	got, err := NewAnalyzer(llm.NewWithModel(model)).Draft(context.Background(), testInput([]experience.Atom{a}))

	if err == nil {
		t.Fatal("err = nil; a failing gateway must not pass an un-audited letter off as audited")
	}
	if got != nil {
		t.Errorf("letter = %v, want nil — nothing may be stored from a chain that did not finish", got)
	}
}

// An answer that arrives and cannot be read is the degradation the spec does allow: the draft
// stands, because a third stage may improve a letter and never destroy it.
func TestUnparseableAuditStillKeepsTheDraft(t *testing.T) {
	a := manualAtom("shipped it")
	model := &scriptedModel{responses: []string{
		`{"selected":["` + a.ID.String() + `"]}`,
		`{"body":"DRAFTED ` + longBody(400) + `"}`,
		`not json at all`,
	}}

	got, err := NewAnalyzer(llm.NewWithModel(model)).Draft(context.Background(), testInput([]experience.Atom{a}))
	if err != nil {
		t.Fatalf("Draft: %v", err)
	}
	if !strings.HasPrefix(got.Body, "DRAFTED") {
		t.Errorf("body = %q, want the un-audited draft", got.Body[:min(20, len(got.Body))])
	}
}

// The gate is one function on one path, so BOTH entry points inherit it by construction — but
// "by construction" is a claim, and this executes it. The handler-level parity tests can only
// read source text; this runs the thing they describe.
func TestEveryCallerOfDraftInheritsTheGate(t *testing.T) {
	inferred := experience.Atom{ID: uuid.New(), Claim: "LEAKEDCLAIM", Provenance: experience.ProvenanceAgentInferred}
	kept := manualAtom("KEPTCLAIM")

	// Two callers, differing only in the order they hand the bank over — which is all the two
	// entry points differ by, since both pass it unfiltered.
	for name, atoms := range map[string][]experience.Atom{
		"inferred first": {inferred, kept},
		"inferred last":  {kept, inferred},
	} {
		body := longBody(400)
		model := &scriptedModel{responses: []string{
			`{"selected":["` + kept.ID.String() + `"]}`,
			`{"body":"` + body + `"}`,
			`{"body":"` + body + `","cited_atom_ids":["` + kept.ID.String() + `"]}`,
		}}

		got, err := NewAnalyzer(llm.NewWithModel(model)).Draft(context.Background(), testInput(atoms))
		if err != nil {
			t.Fatalf("%s: Draft: %v", name, err)
		}
		if strings.Contains(model.sent(), "LEAKEDCLAIM") {
			t.Errorf("%s: an agent_inferred claim reached the model", name)
		}
		for _, id := range got.Cited {
			if id == inferred.ID {
				t.Errorf("%s: an agent_inferred atom was cited", name)
			}
		}
	}
}

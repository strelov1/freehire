package coverletter

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/strelov1/freehire/internal/candidate/experience"
	"github.com/strelov1/freehire/internal/platform/llm"
)

// The audit is asked to cut every sentence the achievements do not support. It cannot make
// that cut against evidence it was never shown — the prompt would be asking it to check a
// claim against a list that is not there.
func TestAuditSeesTheEvidenceItIsAskedToCheckAgainst(t *testing.T) {
	a := manualAtom("SUPPORTINGCLAIMTEXT")
	model := &scriptedModel{responses: []string{
		`{"selected":["` + a.ID.String() + `"]}`,
		`{"body":"` + longBody(400) + `"}`,
		`{"body":"` + longBody(400) + `"}`,
	}}

	if _, err := NewAnalyzer(llm.NewWithModel(model)).Draft(context.Background(), testInput([]experience.Atom{a})); err != nil {
		t.Fatalf("Draft: %v", err)
	}
	// The third call is the audit. Its prompts must carry the claim, not just the letter.
	audit := model.prompts[len(model.prompts)-2:]
	if !strings.Contains(strings.Join(audit, "\n"), "SUPPORTINGCLAIMTEXT") {
		t.Error("the audit turn carried no achievements; it cannot enforce the support rule it is given")
	}
}

// After the audit cuts a sentence, the atom behind it is no longer evidence for anything the
// letter says. Showing it beside the letter claims support the letter no longer has — the
// mirror of the invented-id case Sanitize already guards.
func TestCitationsFollowTheAuditNotTheSelection(t *testing.T) {
	kept, dropped := manualAtom("kept"), manualAtom("dropped")
	model := &scriptedModel{responses: []string{
		`{"selected":["` + kept.ID.String() + `","` + dropped.ID.String() + `"]}`,
		`{"body":"` + longBody(400) + `"}`,
		`{"body":"` + longBody(400) + `","cited_atom_ids":["` + kept.ID.String() + `"]}`,
	}}

	got, err := NewAnalyzer(llm.NewWithModel(model)).Draft(context.Background(), testInput([]experience.Atom{kept, dropped}))
	if err != nil {
		t.Fatalf("Draft: %v", err)
	}
	if len(got.Cited) != 1 || got.Cited[0] != kept.ID {
		t.Errorf("Cited = %v, want only %v — the audit dropped the other one", got.Cited, kept.ID)
	}
}

// An audit that names nothing is not evidence that nothing is cited; it just did not say.
// Stage 1's selection is a real answer, so it stands.
func TestCitationsFallBackToTheSelectionWhenTheAuditNamesNone(t *testing.T) {
	a := manualAtom("kept")
	model := &scriptedModel{responses: []string{
		`{"selected":["` + a.ID.String() + `"]}`,
		`{"body":"` + longBody(400) + `"}`,
		`{"body":"` + longBody(400) + `"}`,
	}}

	got, err := NewAnalyzer(llm.NewWithModel(model)).Draft(context.Background(), testInput([]experience.Atom{a}))
	if err != nil {
		t.Fatalf("Draft: %v", err)
	}
	if len(got.Cited) != 1 || got.Cited[0] != a.ID {
		t.Errorf("Cited = %v, want stage 1's selection", got.Cited)
	}
}

// When neither stage named anything, an empty citation list is the honest answer. Listing
// whatever happened to be offered would assert support nobody claimed.
func TestNoCitationsWhenNeitherStageNamedAny(t *testing.T) {
	a, b := manualAtom("one"), manualAtom("two")
	model := &scriptedModel{responses: []string{
		`{"selected":[]}`,
		`{"body":"` + longBody(400) + `"}`,
		`{"body":"` + longBody(400) + `"}`,
	}}

	got, err := NewAnalyzer(llm.NewWithModel(model)).Draft(context.Background(), testInput([]experience.Atom{a, b}))
	if err != nil {
		t.Fatalf("Draft: %v", err)
	}
	if len(got.Cited) != 0 {
		t.Errorf("Cited = %v, want none — no stage claimed to use any of them", got.Cited)
	}
}

// Stage 1 has no test at all otherwise: task 3.4 asked for one and it was marked done
// without one existing.
func TestSelectPicksTheAtomAnsweringTheRequirement(t *testing.T) {
	wanted, other := manualAtom("kafka pipeline"), manualAtom("unrelated")
	model := &scriptedModel{responses: []string{
		`{"selected":["` + wanted.ID.String() + `"]}`,
		`{"body":"` + longBody(400) + `"}`,
		`{"body":"` + longBody(400) + `","cited_atom_ids":["` + wanted.ID.String() + `"]}`,
	}}

	got, err := NewAnalyzer(llm.NewWithModel(model)).Draft(context.Background(), testInput([]experience.Atom{wanted, other}))
	if err != nil {
		t.Fatalf("Draft: %v", err)
	}
	if len(got.Cited) != 1 || got.Cited[0] != wanted.ID {
		t.Errorf("Cited = %v, want the selected atom %v", got.Cited, wanted.ID)
	}
}

// A model that emits junk in a key the prompt never asked for must not take the whole draft
// down with it. Sanitize overwrites the citations anyway.
func TestDraftSurvivesJunkInAKeyItNeverAskedFor(t *testing.T) {
	a := manualAtom("shipped it")
	model := &scriptedModel{responses: []string{
		`{"selected":["` + a.ID.String() + `"]}`,
		`{"body":"` + longBody(400) + `","cited_atom_ids":{"nonsense":1}}`,
		`{"body":"` + longBody(400) + `"}`,
	}}

	got, err := NewAnalyzer(llm.NewWithModel(model)).Draft(context.Background(), testInput([]experience.Atom{a}))
	if err != nil {
		t.Fatalf("Draft: %v — a malformed key the prompt never asked for must not fail the stage", err)
	}
	if got == nil || got.Body == "" {
		t.Fatal("no letter came back")
	}
}

// A partially-filled Bounds is not the zero value, so a zero-sentinel would leave the
// ceilings at 0 and clip every body to "" — an empty letter, persisted.
func TestValidateFillsMissingBoundsFieldByField(t *testing.T) {
	got := Bounds{MaxCited: 3}.OrDefault()

	d := DefaultBounds()
	if got.MaxCited != 3 {
		t.Errorf("MaxCited = %d, want the caller's 3", got.MaxCited)
	}
	if got.StandardCeiling != d.StandardCeiling || got.ShortCeiling != d.ShortCeiling || got.Floor != d.Floor {
		t.Errorf("unset fields = %+v, want the defaults filled in per field", got)
	}
}

func TestValidateRejectsANonPositiveField(t *testing.T) {
	got := Bounds{MaxCited: -1, Floor: 0}.OrDefault()

	d := DefaultBounds()
	if got.MaxCited != d.MaxCited || got.Floor != d.Floor {
		t.Errorf("got %+v, want a non-positive field replaced by its default", got)
	}
}

// MaxCited: 0 would disable the cap entirely, because len(kept) == 0 is never true after the
// first append. OrDefault is what stops that reaching Sanitize.
func TestZeroMaxCitedCannotDisableTheCap(t *testing.T) {
	b := Bounds{}.OrDefault()
	offered := make([]uuid.UUID, b.MaxCited+3)
	for i := range offered {
		offered[i] = uuid.New()
	}
	l := Letter{Body: "ok", Cited: append([]uuid.UUID(nil), offered...)}

	l.Sanitize(BandStandard, b, offered)

	if len(l.Cited) != b.MaxCited {
		t.Errorf("len(Cited) = %d, want the cap %d", len(l.Cited), b.MaxCited)
	}
}

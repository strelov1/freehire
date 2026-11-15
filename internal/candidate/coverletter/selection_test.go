package coverletter

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/strelov1/freehire/internal/candidate/experience"
	"github.com/strelov1/freehire/internal/platform/llm"
)

func TestSelectionResolvesOnlyConfirmedChosenFacts(t *testing.T) {
	a := experience.Atom{ID: uuid.New(), Claim: "Built worker", Provenance: experience.ProvenanceManual}
	b := experience.Atom{ID: uuid.New(), Claim: "Other work", Provenance: experience.ProvenanceManual}
	got, err := ResolveSelection([]uuid.UUID{a.ID}, []experience.Atom{b, a})
	if err != nil || len(got) != 1 || got[0].ID != a.ID {
		t.Fatalf("resolved %v: %v", got, err)
	}
	for _, ids := range [][]uuid.UUID{{}, {uuid.New()}, {a.ID, a.ID}, {uuid.Nil}} {
		if _, err := ResolveSelection(ids, []experience.Atom{a}); !errors.Is(err, ErrInvalidSelection) {
			t.Fatalf("accepted %v: %v", ids, err)
		}
	}
	a.Provenance = experience.ProvenanceAgentInferred
	if _, err := ResolveSelection([]uuid.UUID{a.ID}, []experience.Atom{a}); !errors.Is(err, ErrInvalidSelection) {
		t.Fatalf("inference accepted: %v", err)
	}
}

func TestEvidenceFingerprintTracksContentsNotOrder(t *testing.T) {
	a := experience.Atom{ID: uuid.New(), Claim: "One", Provenance: experience.ProvenanceManual}
	b := experience.Atom{ID: uuid.New(), Claim: "Two", Provenance: experience.ProvenanceCVImport}
	before := EvidenceFingerprint([]experience.Atom{a, b})
	if before == "" || before != EvidenceFingerprint([]experience.Atom{b, a}) {
		t.Fatal("fingerprint depends on ordering")
	}
	a.Context = "Changed detail"
	if before == EvidenceFingerprint([]experience.Atom{a, b}) {
		t.Fatal("changed context went undetected")
	}
	a.Context = ""
	a.Provenance = experience.ProvenanceAgentInferred
	if before == EvidenceFingerprint([]experience.Atom{a, b}) {
		t.Fatal("changed provenance went undetected")
	}
}

func TestStoredEvidenceTracksSelectionAndClaimChanges(t *testing.T) {
	a := experience.Atom{ID: uuid.New(), Claim: "One", Provenance: experience.ProvenanceManual}
	selection := &Selection{Revision: 1, AtomIDs: []uuid.UUID{a.ID}}
	stored := Stored{EvidenceRevision: 1, EvidenceFingerprint: EvidenceFingerprint([]experience.Atom{a})}
	if stored.EvidenceChanged(selection, []experience.Atom{a}) {
		t.Fatal("unchanged evidence marked changed")
	}
	a.Claim = "Edited"
	if !stored.EvidenceChanged(selection, []experience.Atom{a}) {
		t.Fatal("edit went undetected")
	}
	selection.Revision = 2
	if !stored.EvidenceChanged(selection, nil) {
		t.Fatal("revision ignored")
	}
	if (Stored{}).EvidenceChanged(selection, nil) {
		t.Fatal("legacy freshness asserted")
	}
}

func TestAutomaticDraftBecomesChangedAfterFirstExplicitSelection(t *testing.T) {
	a := experience.Atom{ID: uuid.New(), Claim: "One", Provenance: experience.ProvenanceManual}
	stored := Stored{EvidenceFingerprint: EvidenceFingerprint([]experience.Atom{a})}
	if !stored.EvidenceChanged(&Selection{Revision: 1, AtomIDs: []uuid.UUID{a.ID}}, []experience.Atom{a}) {
		t.Fatal("first explicit selection did not mark automatic draft changed")
	}
}

func TestDraftConsidersEveryAcceptedSelectionFact(t *testing.T) {
	var bank []experience.Atom
	var ids []uuid.UUID
	for i := range MaxSelectedAtoms {
		atom := manualAtom(fmt.Sprintf("CONFIRMEDFACT%03d", i))
		bank = append(bank, atom)
		ids = append(ids, atom.ID)
	}
	atoms, err := ResolveSelection(ids, bank)
	if err != nil {
		t.Fatal(err)
	}
	body := longBody(400)
	model := &scriptedModel{responses: []string{
		`{"selected":[]}`, `{"body":"` + body + `"}`, `{"body":"` + body + `"}`,
	}}
	if _, err := NewAnalyzer(llm.NewWithModel(model)).Draft(context.Background(), testInput(atoms)); err != nil {
		t.Fatal(err)
	}
	for _, atom := range atoms {
		if !strings.Contains(model.sent(), atom.Claim) {
			t.Errorf("accepted fact never reached model: %s", atom.Claim)
		}
	}
}

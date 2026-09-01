package coverletter

import (
	"testing"

	"github.com/google/uuid"
	"github.com/strelov1/freehire/internal/candidate/experience"
)

func atom(p experience.Provenance) experience.Atom {
	return experience.Atom{ID: uuid.New(), Claim: "cut p95 latency from 800ms to 120ms", Provenance: p}
}

func TestPublishableKeepsWhatTheCandidateAsserted(t *testing.T) {
	for _, p := range []experience.Provenance{
		experience.ProvenanceManual,
		experience.ProvenanceCVImport,
		experience.ProvenanceStatedInChat,
	} {
		if got := Publishable([]experience.Atom{atom(p)}); len(got) != 1 {
			t.Errorf("provenance %q: kept %d atoms, want 1 — the candidate asserted this", p, len(got))
		}
	}
}

// The one rule this feature sells. A model's own reading may be banked and searched, but it
// may not be handed to a stage that will write it into prose the candidate signs.
func TestPublishableWithholdsWhatTheModelInferred(t *testing.T) {
	if got := Publishable([]experience.Atom{atom(experience.ProvenanceAgentInferred)}); len(got) != 0 {
		t.Errorf("kept %d atoms, want 0 — agent_inferred may not reach a letter", len(got))
	}
}

// Fails closed, the same way experience's own gate does: a provenance nobody recognises is a
// new entry point that forgot to name itself, not a licence.
func TestPublishableWithholdsAnUnrecognisedProvenance(t *testing.T) {
	for _, p := range []experience.Provenance{"", "trust_me", "AGENT_INFERRED"} {
		if got := Publishable([]experience.Atom{atom(p)}); len(got) != 0 {
			t.Errorf("provenance %q: kept %d atoms, want 0 — an unknown provenance is never publishable", p, len(got))
		}
	}
}

func TestPublishablePreservesOrderAndDropsOnlyTheUnpublishable(t *testing.T) {
	keep1, drop, keep2 := atom(experience.ProvenanceManual), atom(experience.ProvenanceAgentInferred), atom(experience.ProvenanceCVImport)

	got := Publishable([]experience.Atom{keep1, drop, keep2})

	if len(got) != 2 || got[0].ID != keep1.ID || got[1].ID != keep2.ID {
		t.Errorf("got %d atoms in an unexpected order; want keep1 then keep2 with the inferred one dropped", len(got))
	}
}

func TestPublishableReturnsEmptyNotNilForAnEmptyBank(t *testing.T) {
	if got := Publishable(nil); got == nil {
		t.Error("Publishable(nil) is nil; want an empty slice so callers can range without a guard")
	}
}

// IDs is what the chain hands the model and what Sanitize filters citations against, so the
// two must be derived from the same filtered set and never from the raw bank.
func TestIDsReportsTheAtomsInOrder(t *testing.T) {
	a, b := atom(experience.ProvenanceManual), atom(experience.ProvenanceManual)

	got := IDs([]experience.Atom{a, b})

	if len(got) != 2 || got[0] != a.ID || got[1] != b.ID {
		t.Errorf("IDs = %v, want [%v %v]", got, a.ID, b.ID)
	}
}

package coverletter

import (
	"context"
	"errors"
	"testing"

	"github.com/strelov1/freehire/internal/candidate/experience"
	"github.com/strelov1/freehire/internal/candidate/matchanalysis"
)

type fakeRetriever struct {
	byText map[string][]experience.Match
	all    []experience.Atom
	err    error
	texts  []string
}

func (f *fakeRetriever) Retrieve(_ context.Context, _ int64, q experience.Query, _ int) ([]experience.Match, error) {
	f.texts = append(f.texts, q.Text)
	if f.err != nil {
		return nil, f.err
	}
	return f.byText[q.Text], nil
}

func (f *fakeRetriever) ListAtoms(context.Context, int64) ([]experience.Atom, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.all, nil
}

func match(a experience.Atom, score float64) experience.Match {
	return experience.Match{Atom: a, Score: score}
}

func reqs(texts ...string) []matchanalysis.Requirement {
	out := make([]matchanalysis.Requirement, 0, len(texts))
	for _, t := range texts {
		out = append(out, matchanalysis.Requirement{Text: t, Status: "missing-have"})
	}
	return out
}

func TestGatherRetrievesPerRequirement(t *testing.T) {
	a, b := manualAtom("kafka pipeline"), manualAtom("led five people")
	r := &fakeRetriever{byText: map[string][]experience.Match{
		"Kafka":      {match(a, 3)},
		"Leadership": {match(b, 2)},
	}}

	got, err := Gather(context.Background(), r, 1, reqs("Kafka", "Leadership"))
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d atoms, want 2", len(got))
	}
	if len(r.texts) != 2 {
		t.Errorf("retrieved %d times, want one per requirement", len(r.texts))
	}
}

// Two requirements answered by the same achievement is the common case, not an edge one:
// "Kafka" and "event-driven architecture" are one bullet. A duplicate would spend the
// offered-atom budget twice on the same evidence.
func TestGatherDeduplicatesAnAtomThatAnswersTwoRequirements(t *testing.T) {
	a := manualAtom("kafka pipeline")
	r := &fakeRetriever{byText: map[string][]experience.Match{
		"Kafka":        {match(a, 3)},
		"Event-driven": {match(a, 2)},
	}}

	got, err := Gather(context.Background(), r, 1, reqs("Kafka", "Event-driven"))
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("got %d atoms, want 1 — the same atom answered both", len(got))
	}
}

func TestGatherOrdersByBestScore(t *testing.T) {
	weak, strong := manualAtom("weak"), manualAtom("strong")
	r := &fakeRetriever{byText: map[string][]experience.Match{
		"A": {match(weak, 1)},
		"B": {match(strong, 9)},
	}}

	got, err := Gather(context.Background(), r, 1, reqs("A", "B"))
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}
	if len(got) != 2 || got[0].Claim != "strong" {
		t.Errorf("first atom is %q, want the highest-scoring one", got[0].Claim)
	}
}

// A vacancy whose requirements retrieve nothing must still get a letter: the candidate has
// evidence, it just does not line up requirement-by-requirement. Falling back to the bank is
// what keeps the letter honest AND possible.
func TestGatherFallsBackToTheBankWhenNothingMatchesARequirement(t *testing.T) {
	a := manualAtom("something unrelated")
	r := &fakeRetriever{byText: map[string][]experience.Match{}, all: []experience.Atom{a}}

	got, err := Gather(context.Background(), r, 1, reqs("Rust"))
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}
	if len(got) != 1 || got[0].ID != a.ID {
		t.Errorf("got %v, want the bank as a fallback", got)
	}
}

// With no requirements at all there is nothing to retrieve against, so the bank IS the answer
// rather than an empty result.
func TestGatherReadsTheBankWhenThereAreNoRequirements(t *testing.T) {
	a := manualAtom("anything")
	r := &fakeRetriever{all: []experience.Atom{a}}

	got, err := Gather(context.Background(), r, 1, nil)
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("got %d atoms, want the bank", len(got))
	}
}

func TestGatherPropagatesAFailure(t *testing.T) {
	r := &fakeRetriever{err: errors.New("db down")}
	if _, err := Gather(context.Background(), r, 1, reqs("Kafka")); err == nil {
		t.Fatal("err = nil, want the underlying failure")
	}
}

func TestGatherReturnsEmptyNotNilForAnEmptyBank(t *testing.T) {
	got, err := Gather(context.Background(), &fakeRetriever{}, 1, nil)
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}
	if got == nil {
		t.Error("got nil, want an empty slice")
	}
}

// The spec assigns the letter one job: answer what the CV undersells. A genuine gap is a
// requirement the candidate does not meet, so retrieving evidence for it can only surface
// something adjacent that a drafting stage might then stretch into a claim.
func TestGatherIgnoresGenuineGaps(t *testing.T) {
	a := manualAtom("kafka pipeline")
	r := &fakeRetriever{byText: map[string][]experience.Match{"Rust": {match(a, 3)}}, all: []experience.Atom{a}}

	all := []matchanalysis.Requirement{{Text: "Rust", Status: "missing-gap"}}
	if _, err := Gather(context.Background(), r, 1, all); err != nil {
		t.Fatalf("Gather: %v", err)
	}
	for _, seen := range r.texts {
		if seen == "Rust" {
			t.Error("retrieved evidence for a genuine gap; the letter must claim no experience with it")
		}
	}
}

func TestGatherRetrievesOnlyForWhatTheCVUndersells(t *testing.T) {
	have, gap := manualAtom("kafka"), manualAtom("rust")
	r := &fakeRetriever{byText: map[string][]experience.Match{
		"Kafka": {match(have, 3)},
		"Rust":  {match(gap, 3)},
	}}

	mixed := []matchanalysis.Requirement{
		{Text: "Kafka", Status: "missing-have"},
		{Text: "Rust", Status: "missing-gap"},
	}
	got, err := Gather(context.Background(), r, 1, mixed)
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}
	if len(got) != 1 || got[0].ID != have.ID {
		t.Errorf("got %v, want only the atom answering the missing-have requirement", got)
	}
}

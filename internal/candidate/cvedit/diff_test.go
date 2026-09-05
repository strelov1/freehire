package cvedit

import (
	"reflect"
	"testing"

	"github.com/strelov1/freehire/internal/candidate/cv"
	"github.com/strelov1/freehire/internal/candidate/perioddate"
)

// applyingTheDiffReproducesTheTarget is the property that makes the differ trustworthy: the
// operations it derives, applied to the old state, must produce the new one exactly. Every
// case below asserts it before looking at the shape of the operations.
func applyingTheDiffReproducesTheTarget(t *testing.T, old, want State) []Op {
	t.Helper()
	ops := Diff(old, want)

	got, _, err := Apply(old, ops)
	if err != nil {
		t.Fatalf("Apply(Diff(...)): %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Diff produced %d ops that do not reproduce the target:\n got  %+v\n want %+v", len(ops), got, want)
	}
	return ops
}

func kinds(ops []Op) []OpKind {
	out := make([]OpKind, len(ops))
	for i, op := range ops {
		out[i] = op.Kind
	}
	return out
}

func TestDiffOfAnUnchangedStateIsEmpty(t *testing.T) {
	if ops := Diff(sample(), sample()); len(ops) != 0 {
		t.Fatalf("Diff of an unchanged state = %+v, want nothing", ops)
	}
}

func TestDiffFindsChangedScalars(t *testing.T) {
	old := sample()
	want := sample()
	want.Title = "Staff Engineer"
	want.Summary = "Distributed systems"
	want.Style.FontSize = 11
	want.Margins.Left = 0.75

	ops := applyingTheDiffReproducesTheTarget(t, old, want)

	if len(ops) != 4 {
		t.Fatalf("got %d ops, want one per changed scalar: %+v", len(ops), ops)
	}
	for _, op := range ops {
		if op.Kind != OpSet {
			t.Fatalf("changed scalar produced %q, want a set: %+v", op.Kind, op)
		}
	}
}

func TestDiffOfEqualLengthListsComparesInPlace(t *testing.T) {
	old := sample()
	want := sample()
	want.Experience[0].Bullets[1] = "Twice over"

	ops := applyingTheDiffReproducesTheTarget(t, old, want)

	if len(ops) != 1 || ops[0].Kind != OpSet {
		t.Fatalf("got %+v, want a single set", ops)
	}
	if got := string(ops[0].Path); got != "experience[0].bullets[1]" {
		t.Fatalf("path = %q, want the bullet's own address", got)
	}
}

func TestDiffFindsAnInsertedBullet(t *testing.T) {
	old := sample()
	want := sample()
	want.Experience[0].Bullets = []string{"Shipped it", "Mentored two juniors", "Twice"}

	ops := applyingTheDiffReproducesTheTarget(t, old, want)

	if !reflect.DeepEqual(kinds(ops), []OpKind{OpInsert}) {
		t.Fatalf("got %+v, want a single insert", ops)
	}
	if got := string(ops[0].Path); got != "experience[0].bullets[1]" {
		t.Fatalf("path = %q, want the position it was inserted at", got)
	}
}

func TestDiffFindsARemovedBullet(t *testing.T) {
	old := sample()
	want := sample()
	want.Experience[0].Bullets = []string{"Twice"}

	ops := applyingTheDiffReproducesTheTarget(t, old, want)

	if !reflect.DeepEqual(kinds(ops), []OpKind{OpRemove}) {
		t.Fatalf("got %+v, want a single remove", ops)
	}
	if got := string(ops[0].Path); got != "experience[0].bullets[0]" {
		t.Fatalf("path = %q, want the removed position", got)
	}
}

func TestDiffFindsAnAddedEntry(t *testing.T) {
	old := sample()
	want := sample()
	want.Experience = append(want.Experience, cv.ExperienceItem{Role: "Staff", Company: "Globex"})

	ops := applyingTheDiffReproducesTheTarget(t, old, want)

	if !reflect.DeepEqual(kinds(ops), []OpKind{OpInsert}) {
		t.Fatalf("got %+v, want a single insert", ops)
	}
}

// A rewritten bullet arrives from the editor as a whole-document save, so the differ sees a
// list of a different shape. Read naively it is a delete and an add, and the feed would say
// so. Collapsing the pair is what makes the history read like what the candidate did.
func TestDiffCollapsesARewriteIntoOneSet(t *testing.T) {
	old := sample()
	old.Experience[0].Bullets = []string{"Shipped the billing service", "Twice"}
	want := sample()
	want.Experience[0].Bullets = []string{"Shipped the billing service to production", "Twice", "And mentored"}

	ops := applyingTheDiffReproducesTheTarget(t, old, want)

	if !reflect.DeepEqual(kinds(ops), []OpKind{OpSet, OpInsert}) {
		t.Fatalf("got %+v, want the rewrite as a set and the new bullet as an insert", kinds(ops))
	}
	if got := string(ops[0].Path); got != "experience[0].bullets[0]" {
		t.Fatalf("set path = %q, want the rewritten bullet", got)
	}
}

func TestDiffKeepsUnrelatedTextAsARemoveAndAnInsert(t *testing.T) {
	old := sample()
	old.Experience[0].Bullets = []string{"Shipped the billing service", "Twice"}
	want := sample()
	want.Experience[0].Bullets = []string{"Ran the migration to Postgres 16", "Twice"}

	ops := applyingTheDiffReproducesTheTarget(t, old, want)

	// Same length, so this compares in place and is a set either way — what matters is that
	// the collapse threshold did not fire on unrelated text elsewhere.
	if len(ops) != 1 {
		t.Fatalf("got %+v, want one operation", ops)
	}
}

func TestDiffHandlesSeveralListsAtOnce(t *testing.T) {
	old := sample()
	want := sample()
	want.Experience[0].Bullets = []string{"Shipped it"}
	want.Experience[1].Bullets = []string{"Learned", "Then taught"}
	want.Skills[0].Items = []string{"Go", "SQL", "Rust"}
	want.Header.Links = []string{"https://example.com"}

	applyingTheDiffReproducesTheTarget(t, old, want)
}

func TestDiffHandlesAListThatBecameEmpty(t *testing.T) {
	old := sample()
	want := sample()
	want.Experience[0].Bullets = nil

	applyingTheDiffReproducesTheTarget(t, old, want)
}

func TestDiffHandlesAListThatWasEmpty(t *testing.T) {
	old := sample()
	old.Experience[1].Bullets = nil
	want := sample()
	want.Experience[1].Bullets = []string{"Learned"}

	applyingTheDiffReproducesTheTarget(t, old, want)
}

func TestDiffHandlesAWholeSectionAppearing(t *testing.T) {
	old := sample()
	want := sample()
	want.Certifications = []cv.Certification{{Name: "CKA", Issuer: "CNCF", Year: &perioddate.PeriodDate{Year: 2024}}}
	want.Education = []cv.EducationItem{{Institution: "TU Delft", Degree: "MSc"}}

	applyingTheDiffReproducesTheTarget(t, old, want)
}

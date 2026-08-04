package cvedit

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/cv"
)

// sample is the state every apply test starts from: two entries, so an operation on one can
// be seen not to touch the other.
func sample() State {
	return State{
		Title:      "Backend Engineer",
		TemplateID: "classic-ats",
		Document: cv.Document{
			Margins: cv.DefaultMargins(),
			Style:   cv.Style{FontSize: 10.5},
			Header:  cv.Header{FullName: "Ada Lovelace", Email: "ada@example.com"},
			Summary: "Ten years of Go",
			Experience: []cv.ExperienceItem{
				{Role: "Engineer", Company: "Acme", Bullets: []string{"Shipped it", "Twice"}, Stack: []string{"Go"}},
				{Role: "Junior", Company: "Initech", Bullets: []string{"Learned"}},
			},
			Skills: []cv.SkillGroup{{Group: "Languages", Items: []string{"Go", "SQL"}}},
		},
	}
}

func mustParse(t *testing.T, s string) Path {
	t.Helper()
	p, err := ParsePath(s)
	if err != nil {
		t.Fatalf("ParsePath(%q): %v", s, err)
	}
	return p
}

// applyAndUndo is the invariant the whole feature rests on: applying a batch and then
// applying the inverses it produced returns the state exactly as it was.
func applyAndUndo(t *testing.T, ops []Op) (after State) {
	t.Helper()
	before := sample()

	after, inverse, err := Apply(before, ops)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if reflect.DeepEqual(before, after) {
		t.Fatalf("Apply changed nothing")
	}

	undone, _, err := Apply(after, inverse)
	if err != nil {
		t.Fatalf("Apply(inverse): %v", err)
	}
	if !reflect.DeepEqual(before, undone) {
		t.Fatalf("apply → inverse did not return the original:\n before %+v\n after  %+v", before, undone)
	}
	return after
}

func TestApplySetRoundTrips(t *testing.T) {
	after := applyAndUndo(t, []Op{{Kind: OpSet, Path: mustParse(t, "experience[0].bullets[1]"), Value: "Shipped it again"}})

	if got := after.Experience[0].Bullets[1]; got != "Shipped it again" {
		t.Fatalf("bullet = %q, want the new text", got)
	}
	if got := after.Experience[1].Bullets[0]; got != "Learned" {
		t.Fatalf("the other entry changed: %q", got)
	}
}

func TestApplySetReachesScalarsTheOldVocabularyCouldNot(t *testing.T) {
	after := applyAndUndo(t, []Op{
		{Kind: OpSet, Path: mustParse(t, "style.font_size"), Value: 11.0},
		{Kind: OpSet, Path: mustParse(t, "margins.left"), Value: 0.75},
		{Kind: OpSet, Path: mustParse(t, "template_id"), Value: "sidebar"},
		{Kind: OpSet, Path: mustParse(t, "header.email"), Value: "ada@lovelace.dev"},
	})

	if after.Style.FontSize != 11.0 || after.Margins.Left != 0.75 {
		t.Fatalf("typography or margins unchanged: %+v %+v", after.Style, after.Margins)
	}
	if after.TemplateID != "sidebar" || after.Header.Email != "ada@lovelace.dev" {
		t.Fatalf("template or contact unchanged: %+v", after)
	}
}

func TestApplyInsertRoundTrips(t *testing.T) {
	after := applyAndUndo(t, []Op{{Kind: OpInsert, Path: mustParse(t, "experience[0].bullets[0]"), Value: "Led the rewrite"}})

	want := []string{"Led the rewrite", "Shipped it", "Twice"}
	if !reflect.DeepEqual(after.Experience[0].Bullets, want) {
		t.Fatalf("bullets = %v, want %v", after.Experience[0].Bullets, want)
	}
}

func TestApplyInsertAtTheEndIsAllowed(t *testing.T) {
	after := applyAndUndo(t, []Op{{Kind: OpInsert, Path: mustParse(t, "experience[0].bullets[2]"), Value: "And again"}})

	if got := after.Experience[0].Bullets; len(got) != 3 || got[2] != "And again" {
		t.Fatalf("bullets = %v, want the new one appended", got)
	}
}

func TestApplyInsertBuildsAWholeEntry(t *testing.T) {
	entry := cv.ExperienceItem{Role: "Staff", Company: "Globex", Bullets: []string{"Ran it"}}
	after := applyAndUndo(t, []Op{{Kind: OpInsert, Path: mustParse(t, "experience[2]"), Value: entry}})

	if len(after.Experience) != 3 || after.Experience[2].Company != "Globex" {
		t.Fatalf("experience = %+v, want the new entry appended", after.Experience)
	}
}

func TestApplyRemoveRoundTrips(t *testing.T) {
	after := applyAndUndo(t, []Op{{Kind: OpRemove, Path: mustParse(t, "experience[0].bullets[0]")}})

	want := []string{"Twice"}
	if !reflect.DeepEqual(after.Experience[0].Bullets, want) {
		t.Fatalf("bullets = %v, want %v", after.Experience[0].Bullets, want)
	}
}

func TestApplyMoveRoundTrips(t *testing.T) {
	to := 0
	after := applyAndUndo(t, []Op{{Kind: OpMove, Path: mustParse(t, "experience[0].bullets[1]"), To: &to}})

	want := []string{"Twice", "Shipped it"}
	if !reflect.DeepEqual(after.Experience[0].Bullets, want) {
		t.Fatalf("bullets = %v, want %v", after.Experience[0].Bullets, want)
	}
}

func TestApplyMixedBatchRoundTrips(t *testing.T) {
	to := 0
	applyAndUndo(t, []Op{
		{Kind: OpSet, Path: mustParse(t, "summary"), Value: "Distributed systems"},
		{Kind: OpInsert, Path: mustParse(t, "experience[0].bullets[2]"), Value: "Mentored two juniors"},
		{Kind: OpRemove, Path: mustParse(t, "experience[1].bullets[0]")},
		{Kind: OpMove, Path: mustParse(t, "skills[0].items[1]"), To: &to},
		{Kind: OpSet, Path: mustParse(t, "style.line_height"), Value: 0.6},
	})
}

func TestApplyIsAllOrNothing(t *testing.T) {
	before := sample()
	ops := []Op{
		{Kind: OpSet, Path: mustParse(t, "summary"), Value: "Distributed systems"},
		{Kind: OpSet, Path: mustParse(t, "experience[0].bullets[0]"), Value: "Rewrote it"},
		{Kind: OpSet, Path: mustParse(t, "experience[9].bullets[0]"), Value: "Never happened"},
		{Kind: OpSet, Path: mustParse(t, "title"), Value: "Staff Engineer"},
	}

	after, inverse, err := Apply(before, ops)
	if err == nil {
		t.Fatal("Apply succeeded, want a refusal on the third operation")
	}
	if !strings.Contains(err.Error(), "experience[9]") {
		t.Fatalf("error %q does not name the failing address", err)
	}
	if inverse != nil {
		t.Fatalf("a refused batch returned inverses: %+v", inverse)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatal("a refused batch changed the state")
	}
	// The caller's own value is untouched too — Apply works on its own copy.
	if before.Summary != "Ten years of Go" {
		t.Fatalf("Apply mutated the caller's state: %q", before.Summary)
	}
}

func TestApplyRefusesAnOperationItCannotCarryOut(t *testing.T) {
	past := 9
	for _, tc := range []struct {
		name string
		op   Op
	}{
		{"set past the end", Op{Kind: OpSet, Path: mustParse(t, "experience[0].bullets[9]"), Value: "x"}},
		{"insert past the end", Op{Kind: OpInsert, Path: mustParse(t, "experience[0].bullets[9]"), Value: "x"}},
		{"remove past the end", Op{Kind: OpRemove, Path: mustParse(t, "experience[9]")}},
		{"move past the end", Op{Kind: OpMove, Path: mustParse(t, "experience[0].bullets[0]"), To: &past}},
		{"move without a destination", Op{Kind: OpMove, Path: mustParse(t, "experience[0].bullets[0]")}},
		{"insert into something that is not a list", Op{Kind: OpInsert, Path: mustParse(t, "summary"), Value: "x"}},
		{"remove something that is not a list entry", Op{Kind: OpRemove, Path: mustParse(t, "summary")}},
		{"a value of the wrong shape", Op{Kind: OpSet, Path: mustParse(t, "style.font_size"), Value: "large"}},
		{"an unknown kind", Op{Kind: "replace", Path: mustParse(t, "summary"), Value: "x"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := sample()
			after, _, err := Apply(before, []Op{tc.op})
			if err == nil {
				t.Fatalf("Apply(%s) succeeded, want a refusal", tc.name)
			}
			if !reflect.DeepEqual(before, after) {
				t.Fatal("a refused operation changed the state")
			}
		})
	}
}

// A batch names its positions against the document it was written for. Removing an earlier
// position first shifts every later one, so applying them front-to-back makes a batch of two
// removals refuse itself — the failure the tailoring agent kept hitting on prod.
func TestApplyRemovesTwoPositionsOfOneList(t *testing.T) {
	before := sample()
	before.Experience[0].Bullets = []string{"zero", "one", "two", "three", "four"}

	after, inverse, err := Apply(before, OrderAgainstOriginal([]Op{
		{Kind: OpRemove, Path: mustParse(t, "experience[0].bullets[3]")},
		{Kind: OpRemove, Path: mustParse(t, "experience[0].bullets[4]")},
	}))
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}

	want := []string{"zero", "one", "two"}
	if !reflect.DeepEqual(after.Experience[0].Bullets, want) {
		t.Fatalf("bullets = %q, want %q", after.Experience[0].Bullets, want)
	}

	undone, _, err := Apply(after, inverse)
	if err != nil {
		t.Fatalf("Apply(inverse): %v", err)
	}
	if !reflect.DeepEqual(before.Experience, undone.Experience) {
		t.Fatalf("undo left %+v, want %+v", undone.Experience, before.Experience)
	}
}

// Reordering removals forgives an address invalidated by the batch itself. An address the list
// never held is a different thing and stays refused, whole batch and all.
func TestApplyRefusesARemovalTheListNeverHeld(t *testing.T) {
	before := sample()
	before.Experience[0].Bullets = []string{"zero", "one", "two", "three", "four"}

	after, _, err := Apply(before, OrderAgainstOriginal([]Op{
		{Kind: OpRemove, Path: mustParse(t, "experience[0].bullets[3]")},
		{Kind: OpRemove, Path: mustParse(t, "experience[0].bullets[9]")},
	}))
	if err == nil {
		t.Fatal("Apply succeeded, want a refusal")
	}
	if !errors.Is(err, ErrInvalidOp) {
		t.Fatalf("err = %v, want ErrInvalidOp", err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatal("a refused batch changed the state")
	}
}

func TestApplyInversesUndoInReverseOrder(t *testing.T) {
	before := sample()
	// Two operations on the same list where order matters: undoing them in the order they
	// were applied would leave the bullet in the wrong place.
	ops := []Op{
		{Kind: OpInsert, Path: mustParse(t, "experience[0].bullets[0]"), Value: "First"},
		{Kind: OpRemove, Path: mustParse(t, "experience[0].bullets[2]")},
	}

	after, inverse, err := Apply(before, ops)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	undone, _, err := Apply(after, inverse)
	if err != nil {
		t.Fatalf("Apply(inverse): %v", err)
	}
	if !reflect.DeepEqual(before.Experience, undone.Experience) {
		t.Fatalf("undo out of order: %+v, want %+v", undone.Experience, before.Experience)
	}
}

// Diff states its indices against the list AS THE BATCH CHANGES IT, so its removals must be
// applied in the order it wrote them. This is the everyday save path — the editor ships a whole
// document and Diff turns it into operations — and the property it rests on is that applying
// Diff(a, b) to a produces b exactly.
func TestDiffOfTwoDeletionsRoundTrips(t *testing.T) {
	before := sample()
	before.Experience[0].Bullets = []string{"A", "B", "C", "D"}

	after := sample()
	after.Experience[0].Bullets = []string{"A", "C"}

	ops := Diff(before, after)
	got, _, err := Apply(before, ops)
	if err != nil {
		t.Fatalf("Apply(Diff): %v", err)
	}
	if !reflect.DeepEqual(got.Experience[0].Bullets, after.Experience[0].Bullets) {
		t.Fatalf("bullets = %q, want %q — the user's own deletion removed the wrong line",
			got.Experience[0].Bullets, after.Experience[0].Bullets)
	}
}

// Undo replays the inverses Apply produced, and those are sequential too: an insert's inverse
// is a removal at the position the insert used. Reordering them turned "undo the run" into
// removing the candidate's own line while leaving the agent's insertion in place.
func TestUndoOfTwoInsertsRestoresTheOriginal(t *testing.T) {
	before := sample()
	before.Experience[0].Bullets = []string{"A", "B", "C", "D"}

	ops := []Op{
		{Kind: OpInsert, Path: mustParse(t, "experience[0].bullets[3]"), Value: "X"},
		{Kind: OpInsert, Path: mustParse(t, "experience[0].bullets[1]"), Value: "Y"},
	}
	after, inverse, err := Apply(before, ops)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	undone, _, err := Apply(after, inverse)
	if err != nil {
		t.Fatalf("Apply(inverse): %v", err)
	}
	if !reflect.DeepEqual(undone.Experience[0].Bullets, before.Experience[0].Bullets) {
		t.Fatalf("undo left %q, want %q", undone.Experience[0].Bullets, before.Experience[0].Bullets)
	}
}

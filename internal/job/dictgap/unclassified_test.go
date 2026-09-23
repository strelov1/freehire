package dictgap

import (
	"testing"

	"github.com/strelov1/freehire/internal/dict/classify"
)

func titleCounts(pairs ...any) []TitleCount {
	out := make([]TitleCount, 0, len(pairs)/2)
	for i := 0; i+1 < len(pairs); i += 2 {
		out = append(out, TitleCount{Title: pairs[i].(string), Count: pairs[i+1].(int)})
	}
	return out
}

// A title neither dictionary places is what the report exists for, and the ranking is
// by how many postings carry it: the catalogue holds 1.5M distinct unrecognised
// titles, so an unranked list is unreadable and a curator would never reach the ones
// that matter.
func TestUnclassifiedTitlesRanksByCount(t *testing.T) {
	got := UnclassifiedTitles(titleCounts(
		"Швея", 12,
		"Музыкальный руководитель", 40,
		"Горничная", 25,
	))
	want := []string{"Музыкальный руководитель", "Горничная", "Швея"}
	if len(got) != len(want) {
		t.Fatalf("got %d candidates, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i].Title != want[i] {
			t.Errorf("rank %d = %q, want %q", i, got[i].Title, want[i])
		}
	}
}

// A title the TECH dictionary places is not a gap. This is the half that must answer
// for the dictionary as it stands rather than for a stored column: the postings
// carrying it may still read is_tech NULL in the database, because a dictionary change
// only reaches stored rows through backfill-derive.
func TestUnclassifiedTitlesExcludesATechTitle(t *testing.T) {
	got := UnclassifiedTitles(titleCounts(
		"Senior Software Engineer", 100,
		"Швея", 5,
	))
	if len(got) != 1 || got[0].Title != "Швея" {
		t.Fatalf("got %+v, want only the seamstress", got)
	}
}

// A title the CATEGORY dictionary places is not a gap either: a technical category
// already yields is_tech through the derivation, so reporting it would send a curator
// after a term that would change nothing.
func TestUnclassifiedTitlesExcludesACategorisedTitle(t *testing.T) {
	// "Project Manager" resolves to project_management — no tech-title signal, but a
	// category all the same, which is exactly the case a tech-only check would miss.
	if c := classify.Parse("Project Manager"); c.Category == "" {
		t.Skip("fixture no longer resolves a category; pick another")
	}
	got := UnclassifiedTitles(titleCounts(
		"Project Manager", 100,
		"Швея", 5,
	))
	if len(got) != 1 || got[0].Title != "Швея" {
		t.Fatalf("got %+v, want only the seamstress", got)
	}
}

// The SQL groups per id chunk, so one title comes back once per chunk it appears in —
// the same shape ListTitlesForClassifyDrift produces. Summing is the caller's job and
// it happens here, because a report that ranked a title by its largest chunk rather
// than its true total would rank the wrong things first.
func TestUnclassifiedTitlesSumsRepeatedTitles(t *testing.T) {
	got := UnclassifiedTitles(titleCounts(
		"Швея", 10,
		"Горничная", 15,
		"Швея", 12,
	))
	if len(got) != 2 {
		t.Fatalf("got %d candidates, want 2: %+v", len(got), got)
	}
	if got[0].Title != "Швея" || got[0].Count != 22 {
		t.Errorf("got %+v, want Швея with 22 (10+12) ranked first", got[0])
	}
}

// Ties break on the title so two runs over the same data print the same list — a
// report a curator diffs against last week's run is worth more than one they cannot.
func TestUnclassifiedTitlesBreaksTiesDeterministically(t *testing.T) {
	got := UnclassifiedTitles(titleCounts("Горничная", 7, "Швея", 7))
	if len(got) != 2 || got[0].Title != "Горничная" {
		t.Fatalf("got %+v, want Горничная first on the tie", got)
	}
}

// An empty title is never a candidate: it names no role, so no dictionary entry could
// ever place it and a curator can do nothing with it.
func TestUnclassifiedTitlesSkipsAnEmptyTitle(t *testing.T) {
	if got := UnclassifiedTitles(titleCounts("", 900, "Швея", 3)); len(got) != 1 || got[0].Title != "Швея" {
		t.Fatalf("got %+v, want only the seamstress", got)
	}
}

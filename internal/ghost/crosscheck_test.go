package ghost

import (
	"slices"
	"testing"
)

func posting(id int64, company, title string, stamped bool) Posting {
	return Posting{ID: id, CompanySlug: company, Title: title, Stamped: stamped}
}

// Absence is evidence only where we looked. Without the gate the signal reports our
// own board-coverage blind spots as the employer's fault — and the previous attempt
// at this feature died precisely by measuring our data rather than the world.
func TestCrosscheck_NoCompanyBoardMeansNoVerdict(t *testing.T) {
	got := Crosscheck([]Posting{posting(1, "acme", "Go Developer", false)}, nil)

	if len(got.Stamp) != 0 {
		t.Errorf("stamp = %v, want none — we do not crawl this company's board", got.Stamp)
	}
	if len(got.Clear) != 0 {
		t.Errorf("clear = %v, want none", got.Clear)
	}
	if got.Skipped != 1 {
		t.Errorf("skipped = %d, want 1 — the gate must be visible in the report", got.Skipped)
	}
}

func TestCrosscheck_AbsentRoleIsStamped(t *testing.T) {
	got := Crosscheck(
		[]Posting{posting(1, "acme", "Go Developer", false)},
		[]string{"Product Designer"},
	)

	if !slices.Equal(got.Stamp, []int64{1}) {
		t.Errorf("stamp = %v, want [1]", got.Stamp)
	}
}

func TestCrosscheck_PresentRoleIsNotStamped(t *testing.T) {
	got := Crosscheck(
		[]Posting{posting(1, "acme", "Go Developer", false)},
		[]string{"Go Developer"},
	)

	if len(got.Stamp) != 0 {
		t.Errorf("stamp = %v, want none — the role is on the company's own board", got.Stamp)
	}
}

// The stamp tracks the world rather than accumulating: a role that reappears on the
// company's board loses its stamp on the next run.
func TestCrosscheck_ReappearingRoleIsCleared(t *testing.T) {
	got := Crosscheck(
		[]Posting{posting(1, "acme", "Go Developer", true)},
		[]string{"Go Developer"},
	)

	if !slices.Equal(got.Clear, []int64{1}) {
		t.Errorf("clear = %v, want [1]", got.Clear)
	}
	if len(got.Stamp) != 0 {
		t.Errorf("stamp = %v, want none", got.Stamp)
	}
}

// A posting already stamped and still absent needs no write; re-stamping every run
// would churn updated_at across the catalogue for no change in meaning.
func TestCrosscheck_AlreadyStampedAndStillAbsentIsRestamped(t *testing.T) {
	got := Crosscheck(
		[]Posting{posting(1, "acme", "Go Developer", true)},
		[]string{"Product Designer"},
	)

	if !slices.Equal(got.Stamp, []int64{1}) {
		t.Errorf("stamp = %v, want [1] — the stamp must be refreshed or it ages out", got.Stamp)
	}
}

// An unstamped posting whose role is present needs no write either.
func TestCrosscheck_PresentAndUnstampedWritesNothing(t *testing.T) {
	got := Crosscheck(
		[]Posting{posting(1, "acme", "Go Developer", false)},
		[]string{"Go Developer"},
	)

	if len(got.Stamp)+len(got.Clear) != 0 {
		t.Errorf("stamp=%v clear=%v, want no writes", got.Stamp, got.Clear)
	}
}

// The board's titles go through the same normalization as the postings', so a city
// suffix on either side does not manufacture an absence.
func TestCrosscheck_MatchesAcrossACitySuffix(t *testing.T) {
	got := Crosscheck(
		[]Posting{posting(1, "acme", "Senior Backend Engineer", false)},
		[]string{"Senior Backend Engineer, Krakow"},
	)

	if len(got.Stamp) != 0 {
		t.Errorf("stamp = %v, want none — the city variant is the same role", got.Stamp)
	}
}

// A posting whose title normalizes to nothing has no key, so it can neither match
// nor be judged absent. Stamping it would make every untitled posting evidence.
func TestCrosscheck_UntitledPostingIsSkipped(t *testing.T) {
	got := Crosscheck([]Posting{posting(1, "acme", "   ", false)}, []string{"Go Developer"})

	if len(got.Stamp) != 0 {
		t.Errorf("stamp = %v, want none for a posting with no usable title", got.Stamp)
	}
	if got.Skipped != 1 {
		t.Errorf("skipped = %d, want 1", got.Skipped)
	}
}

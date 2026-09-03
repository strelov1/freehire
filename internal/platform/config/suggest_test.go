package config

import (
	"math"
	"strconv"
	"testing"
)

// The ceiling is the part that matters. The company floor is handed to Postgres as an
// int32, and narrowing a wider value wraps — a wrapped floor can come out NEGATIVE,
// which admits every company slug in the catalogue instead of excluding the long tail.
// That failure would look exactly like the floor not working.
func TestLoadSuggest_FloorsSurviveNarrowingToInt32(t *testing.T) {
	t.Setenv("SUGGEST_TITLE_FLOOR", strconv.Itoa(math.MaxInt64))
	t.Setenv("SUGGEST_COMPANY_FLOOR", strconv.Itoa(math.MaxInt64))

	s := LoadSuggest()
	if int32(s.CompanyFloor) < 1 {
		t.Errorf("company floor narrowed to %d — a non-positive floor admits everything", int32(s.CompanyFloor))
	}
	if int32(s.TitleFloor) < 1 {
		t.Errorf("title floor narrowed to %d", int32(s.TitleFloor))
	}
}

// Zero and below would admit every one-off spelling in the catalogue: hundreds of
// thousands of rows that answer nothing.
func TestLoadSuggest_FloorsAreAtLeastOne(t *testing.T) {
	t.Setenv("SUGGEST_TITLE_FLOOR", "0")
	t.Setenv("SUGGEST_COMPANY_FLOOR", "-5")

	s := LoadSuggest()
	if s.TitleFloor != 1 || s.CompanyFloor != 1 {
		t.Errorf("floors = %d/%d, want 1/1", s.TitleFloor, s.CompanyFloor)
	}
}

// An unparseable value falls back rather than failing the worker: the defaults are
// measured figures, and a typo should not empty the box's vocabulary.
func TestLoadSuggest_UnparseableFallsBack(t *testing.T) {
	t.Setenv("SUGGEST_TITLE_FLOOR", "twenty")

	if got := LoadSuggest().TitleFloor; got != 25 {
		t.Errorf("title floor = %d, want the default 25", got)
	}
}

package resumeextract

import (
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/candidate/perioddate"
)

func TestDatedYears(t *testing.T) {
	now := time.Date(2026, 11, 3, 0, 0, 0, 0, time.UTC)
	pd := func(y, m int) *perioddate.PeriodDate { return &perioddate.PeriodDate{Year: y, Month: m} }
	got := datedYears([]Experience{
		{Start: pd(2023, 5), Current: true},    // 3y 5m to now
		{Start: pd(2020, 1), End: pd(2023, 6)}, // overlaps the first by 2 months
		{Title: "undated"},                     // skipped
	}, now)
	// Jan 2020 .. Nov 2026 = 6y 10m, overlap counted once.
	if got != 6 {
		t.Fatalf("datedYears = %d, want 6", got)
	}
	st := Structured{TotalYears: 9}
	if got := st.EffectiveYears(now); got != 9 {
		t.Fatalf("EffectiveYears kept stated = %d, want 9", got)
	}
}

// A year-only date is the least it could mean: "2024-2025" may be one month (December to
// January), and a figure compared against a vacancy's minimum must not claim a year the CV
// does not show.
func TestDatedYears_YearOnlyBoundsAreConservative(t *testing.T) {
	now := time.Date(2026, 11, 3, 0, 0, 0, 0, time.UTC)
	pd := func(y, m int) *perioddate.PeriodDate { return &perioddate.PeriodDate{Year: y, Month: m} }
	for name, c := range map[string]struct {
		exp  []Experience
		want int
	}{
		"adjacent years may be a month":   {[]Experience{{Start: pd(2024, 0), End: pd(2025, 0)}}, 0},
		"three year gap is two full":      {[]Experience{{Start: pd(2020, 0), End: pd(2023, 0)}}, 2},
		"year-only start, ongoing":        {[]Experience{{Start: pd(2023, 0), Current: true}}, 2},
		"month given on both ends":        {[]Experience{{Start: pd(2020, 1), End: pd(2023, 6)}}, 3},
		"year-only start, month-only end": {[]Experience{{Start: pd(2024, 0), End: pd(2025, 6)}}, 0},
	} {
		if got := datedYears(c.exp, now); got != c.want {
			t.Errorf("%s: datedYears = %d, want %d", name, got, c.want)
		}
	}
}

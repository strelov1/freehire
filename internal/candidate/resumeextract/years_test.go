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

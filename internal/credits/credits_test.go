package credits

import (
	"testing"
	"time"
)

func TestPeriodKey(t *testing.T) {
	got := periodKey(time.Date(2026, time.July, 18, 13, 4, 0, 0, time.UTC))
	if got != "2026-07" {
		t.Errorf("periodKey = %q, want 2026-07", got)
	}
}

func TestResetsAt(t *testing.T) {
	// Mid-month resets on the first of next month, UTC midnight.
	got := resetsAt(time.Date(2026, time.July, 18, 13, 4, 0, 0, time.UTC))
	want := time.Date(2026, time.August, 1, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("resetsAt(July) = %v, want %v", got, want)
	}
	// December rolls into January of the next year.
	got = resetsAt(time.Date(2026, time.December, 31, 23, 59, 0, 0, time.UTC))
	want = time.Date(2027, time.January, 1, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("resetsAt(December) = %v, want %v", got, want)
	}
}

func TestConfigCost(t *testing.T) {
	cfg := Config{MonthlyGrant: 20, CostMatch: 1, CostTailor: 3}
	if cfg.cost(FeatureMatch) != 1 {
		t.Errorf("cost(match) = %d, want 1", cfg.cost(FeatureMatch))
	}
	if cfg.cost(FeatureTailor) != 3 {
		t.Errorf("cost(tailor) = %d, want 3", cfg.cost(FeatureTailor))
	}
}

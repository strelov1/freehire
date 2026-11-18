package sweep

import (
	"testing"
	"time"
)

func TestLinkedInShipsWithAThreeDayReReadWindow(t *testing.T) {
	now := time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)
	unset, err := BodyRefreshFor("", "", now)
	if err != nil {
		t.Fatal(err)
	}
	got := unset.WithProviderDefault("linkedin", now)
	if !got.Enabled() || got.Slices != 1 || !got.Cutoff.Equal(now.AddDate(0, 0, -3)) {
		t.Errorf("linkedin default = %+v, want enabled, 3 days, slice 1", got)
	}
	if other := unset.WithProviderDefault("greenhouse", now); other.Enabled() {
		t.Errorf("another provider gained a default refresh window: %+v", other)
	}
	explicit, err := BodyRefreshFor("10", "5", now)
	if err != nil {
		t.Fatal(err)
	}
	if kept := explicit.WithProviderDefault("linkedin", now); kept != explicit {
		t.Errorf("explicit BODY_REFRESH_DAYS was overridden: %+v, want %+v", kept, explicit)
	}
}

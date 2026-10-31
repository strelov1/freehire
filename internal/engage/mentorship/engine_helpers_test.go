package mentorship

import (
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/engage/slots"
)

// berlin is the zone the booking and cache tests state their mentor in. The engine's own
// copy moved to internal/engage/slots with the engine tests.
func berlin(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatalf("LoadLocation(Europe/Berlin): %v", err)
	}
	return loc
}

func mustTimeOfDay(t *testing.T, hour, minute int) slots.TimeOfDay {
	t.Helper()
	got, err := slots.NewTimeOfDay(hour, minute)
	if err != nil {
		t.Fatalf("NewTimeOfDay(%d, %d): %v", hour, minute, err)
	}
	return got
}

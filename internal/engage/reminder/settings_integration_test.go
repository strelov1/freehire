//go:build integration

// Integration test for task 6.4 of notification-frequency-quiet-hours: the
// digest-frequency/quiet-hours fields round-trip through UpsertSettings/
// GetSettings against a real Postgres. Run with:
// go test -tags=integration ./internal/engage/reminder/
package reminder

import (
	"context"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/testdb"
)

func TestSettings_DeliveryTimingRoundTrips(t *testing.T) {
	pool := testdb.Pool(t)
	queries := db.New(pool)
	ctx := context.Background()
	repo := NewQueriesRepository(queries)

	userID := insertReminderIntegrationUser(t, pool, "delivery-timing-settings@example.test")

	digestTime := 9 * time.Hour
	start, end := 22*time.Hour, 8*time.Hour
	in := Settings{
		Enabled:         true,
		Channels:        []string{"email"},
		DigestFrequency: "daily",
		DigestTime:      &digestTime,
		QuietHoursStart: &start,
		QuietHoursEnd:   &end,
	}

	if _, err := repo.UpsertSettings(ctx, userID, in); err != nil {
		t.Fatalf("UpsertSettings: %v", err)
	}

	got, err := repo.GetSettings(ctx, userID)
	if err != nil {
		t.Fatalf("GetSettings: %v", err)
	}
	if got.DigestFrequency != "daily" {
		t.Errorf("DigestFrequency = %q, want %q", got.DigestFrequency, "daily")
	}
	if got.DigestTime == nil || *got.DigestTime != digestTime {
		t.Errorf("DigestTime = %v, want %v", got.DigestTime, digestTime)
	}
	if got.QuietHoursStart == nil || *got.QuietHoursStart != start {
		t.Errorf("QuietHoursStart = %v, want %v", got.QuietHoursStart, start)
	}
	if got.QuietHoursEnd == nil || *got.QuietHoursEnd != end {
		t.Errorf("QuietHoursEnd = %v, want %v", got.QuietHoursEnd, end)
	}
}

func TestSettings_AbsentRowDefaultsToHourlyInterval(t *testing.T) {
	pool := testdb.Pool(t)
	queries := db.New(pool)
	ctx := context.Background()
	repo := NewQueriesRepository(queries)

	userID := insertReminderIntegrationUser(t, pool, "delivery-timing-absent@example.test")

	got, err := repo.GetSettings(ctx, userID)
	if err != nil {
		t.Fatalf("GetSettings: %v", err)
	}
	if got.DigestFrequency != "interval" || got.DigestIntervalHours != 1 {
		t.Errorf("digest = %q/%d, want interval/1 for a never-configured account", got.DigestFrequency, got.DigestIntervalHours)
	}
	if got.QuietHoursStart != nil || got.QuietHoursEnd != nil {
		t.Errorf("quiet hours = %v/%v, want both nil (off) by default", got.QuietHoursStart, got.QuietHoursEnd)
	}
}

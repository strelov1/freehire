//go:build integration

package nudge

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/testdb"
)

func TestClosedListingSettlement(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	queries := db.New(pool)
	for i, tc := range []struct {
		stage                          string
		recent, enabled, applied, open bool
		wantStage                      string
		wantNudge                      int
	}{
		{stage: "applied", recent: true, enabled: true, applied: true, wantStage: "expired", wantNudge: 1},
		{stage: "offer", recent: true, enabled: true, wantStage: "expired", wantNudge: 1},
		{stage: "preparing", recent: true, wantStage: "expired"},
		{stage: "interview", enabled: true, wantStage: "expired"},
		{stage: "screening", wantStage: "expired"},
		{stage: "accepted", recent: true, enabled: true, wantStage: "accepted"},
		{stage: "rejected", recent: true, enabled: true, wantStage: "rejected"},
		{stage: "withdrawn", recent: true, enabled: true, wantStage: "withdrawn"},
		{stage: "expired", recent: true, enabled: true, wantStage: "expired"},
		{stage: "applied", open: true, wantStage: "applied"},
		{applied: true, wantStage: "expired"},
		{}, // An application with only notes is not on the tracker.
	} {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			key := fmt.Sprintf("closure-settlement-%d", i)
			userID := seedNudgeIntegrationUser(t, pool, key+"@example.test")
			jobID := seedClosedJob(t, pool, key)
			seedApplication(t, pool, userID, jobID, "applied")
			if _, err := pool.Exec(ctx, `UPDATE applications SET stage = NULLIF($3, ''), applied_at = CASE WHEN $4 THEN now() ELSE NULL END WHERE user_id = $1 AND job_id = $2`, userID, jobID, tc.stage, tc.applied); err != nil {
				t.Fatal(err)
			}
			if tc.recent || tc.open {
				if _, err := pool.Exec(ctx, `UPDATE jobs SET closed_at = CASE WHEN $2 THEN NULL ELSE now() END WHERE id = $1`, jobID, tc.open); err != nil {
					t.Fatal(err)
				}
			}
			if tc.enabled {
				seedNotificationSettings(t, pool, userID, []string{"email"})
			}
			for pass := 0; pass < 2; pass++ {
				if _, err := queries.SettleClosedApplications(ctx, 7); err != nil {
					t.Fatal(err)
				}
				var stage string
				var events, nudges int
				if err := pool.QueryRow(ctx, `SELECT COALESCE(stage, '') FROM applications WHERE user_id = $1 AND job_id = $2`, userID, jobID).Scan(&stage); err != nil {
					t.Fatal(err)
				}
				if stage != tc.wantStage {
					t.Fatalf("stage = %q, want %q", stage, tc.wantStage)
				}
				if err := pool.QueryRow(ctx, `SELECT count(*) FROM application_events WHERE user_id = $1 AND job_id = $2 AND kind = 'stage_set' AND signal = 'expired' AND source = 'system'`, userID, jobID).Scan(&events); err != nil {
					t.Fatal(err)
				}
				wantEvents := 0
				if tc.wantStage == "expired" && tc.stage != "expired" {
					wantEvents = 1
				}
				if events != wantEvents {
					t.Fatalf("events = %d, want %d", events, wantEvents)
				}
				if err := pool.QueryRow(ctx, `SELECT count(*) FROM application_nudges WHERE user_id = $1 AND job_id = $2 AND kind = 'job_closed'`, userID, jobID).Scan(&nudges); err != nil {
					t.Fatal(err)
				}
				if nudges != tc.wantNudge {
					t.Fatalf("nudges = %d, want %d", nudges, tc.wantNudge)
				}
			}
		})
	}
}

func TestJobClosedEpisodeAfterReopening(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	queries := db.New(pool)
	userID := seedNudgeIntegrationUser(t, pool, "closure-episode@example.test")
	jobID := seedClosedJob(t, pool, "closure-episode")
	seedApplication(t, pool, userID, jobID, "expired")
	nudgeID := seedPendingJobClosedNudge(t, pool, userID, jobID)
	for _, tc := range []struct {
		name, update string
		current      bool
	}{
		{"original closure", "SELECT 1", true},
		{"reopened", "UPDATE jobs SET closed_at = NULL WHERE id = $1", false},
		{"closed again", "UPDATE jobs SET closed_at = now() WHERE id = $1", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.update != "SELECT 1" {
				if _, err := pool.Exec(ctx, tc.update, jobID); err != nil {
					t.Fatal(err)
				}
			}
			info, err := queries.GetNudgeForDelivery(ctx, nudgeID)
			if err != nil {
				t.Fatal(err)
			}
			if info.JobClosedEpisodeCurrent != tc.current {
				t.Fatalf("current = %v, want %v", info.JobClosedEpisodeCurrent, tc.current)
			}
		})
	}
}

func TestSettlementWaitsForConcurrentReopen(t *testing.T) {
	pool := testdb.Pool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	userID := seedNudgeIntegrationUser(t, pool, "concurrent-reopen@example.test")
	jobID := seedClosedJob(t, pool, "concurrent-reopen")
	seedApplication(t, pool, userID, jobID, "interview")
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	if _, err := tx.Exec(ctx, "UPDATE jobs SET closed_at = NULL WHERE id = $1", jobID); err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() { _, err := db.New(pool).SettleClosedApplications(ctx, 7); result <- err }()
	// Observe the blocked statement before committing, so its snapshot predates
	// the reopen. Without the job lock it expires the application immediately.
	for {
		var waiting bool
		if err := pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_stat_activity WHERE datname = current_database() AND wait_event_type = 'Lock' AND query LIKE '-- name: SettleClosedApplications%')`).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			break
		}
		select {
		case err := <-result:
			t.Fatalf("settlement did not wait for the reopening job: %v", err)
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(10 * time.Millisecond):
		}
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-result; err != nil {
		t.Fatal(err)
	}
	var stage string
	if err := pool.QueryRow(ctx, "SELECT stage FROM applications WHERE user_id = $1 AND job_id = $2", userID, jobID).Scan(&stage); err != nil {
		t.Fatal(err)
	}
	if stage != "interview" {
		t.Fatalf("stage = %s, want interview for a reopened listing", stage)
	}
}

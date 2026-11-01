//go:build integration

package nudge

import (
	"context"
	"fmt"
	"testing"

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

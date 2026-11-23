//go:build integration

package main

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/strelov1/freehire/internal/ai/enrich"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/testdb"
)

func TestInvalidatedClaimCannotOverwritePromotedJobEnrichment(t *testing.T) {
	for _, overlap := range []bool{false, true} {
		name := "after_commit"
		if overlap {
			name = "during_transfer"
		}
		t.Run(name, func(t *testing.T) {
			pool := testdb.Pool(t)
			ctx := context.Background()
			q := db.New(pool)
			original, err := q.UpsertJob(ctx, db.UpsertJobParams{Source: "linkedin", ExternalID: "search:123", URL: "https://www.linkedin.com/jobs/view/123/", Title: "Go Engineer", Company: "Acme", PublicSlug: "enrichment-source-handoff", CompanySlug: "acme", Description: "Build Go services.", IsTech: pgtype.Bool{Bool: true, Valid: true}})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := q.EnqueueJobEnrichment(ctx, db.EnqueueJobEnrichmentParams{JobID: original.ID, TargetVersion: int32(enrich.Version)}); err != nil {
				t.Fatal(err)
			}
			var oldID int64
			if err := pool.QueryRow(ctx, "SELECT id FROM enrichment_outbox WHERE job_id=$1", original.ID).Scan(&oldID); err != nil {
				t.Fatal(err)
			}
			tx, err := pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(ctx)
			qtx := q.WithTx(tx)
			if _, err := qtx.PromoteJobSource(ctx, db.PromoteJobSourceParams{JobID: original.ID, ExpectedSource: "linkedin", ExpectedExternalID: "search:123", ExpectedUrl: "https://www.linkedin.com/jobs/view/123/", Source: "hrsmart", ExternalID: "acme.hrdepartment.com:123", URL: "https://acme.hrdepartment.com/hrsmart/ats/Posting/view/123"}); err != nil {
				t.Fatal(err)
			}
			if _, err := qtx.EnqueueJobEnrichment(ctx, db.EnqueueJobEnrichmentParams{JobID: original.ID, TargetVersion: int32(enrich.Version)}); err != nil {
				t.Fatal(err)
			}
			var freshID int64
			if err := tx.QueryRow(ctx, "SELECT id FROM enrichment_outbox WHERE job_id=$1", original.ID).Scan(&freshID); err != nil {
				t.Fatal(err)
			}
			store := newDBStore(pool, false)
			if !overlap {
				if err := tx.Commit(ctx); err != nil {
					t.Fatal(err)
				}
			}
			completionCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()
			done := make(chan error, 1)
			go func() {
				done <- store.Complete(completionCtx, enrich.Claimed{OutboxID: oldID, JobID: original.ID, TargetVersion: enrich.Version}, json.RawMessage(`{"summary":"Stale LinkedIn response"}`))
			}()
			if overlap {
				deadline := time.After(5 * time.Second)
				ticker := time.NewTicker(10 * time.Millisecond)
				defer ticker.Stop()
				for {
					var blocked bool
					if err := pool.QueryRow(completionCtx, `SELECT EXISTS (SELECT 1 FROM pg_stat_activity WHERE datname = current_database() AND wait_event_type = 'Lock' AND $1::bigint = ANY(pg_blocking_pids(pid)))`, int64(tx.Conn().PgConn().PID())).Scan(&blocked); err != nil {
						t.Fatal(err)
					}
					if blocked {
						break
					}
					select {
					case err := <-done:
						t.Fatalf("completion did not wait for ownership transfer: %v", err)
					case <-deadline:
						t.Fatal("completion never waited for ownership transfer")
					case <-ticker.C:
					}
				}
				if err := tx.Commit(ctx); err != nil {
					t.Fatal(err)
				}
			}
			if err := <-done; err != nil {
				t.Fatal(err)
			}
			current, err := q.GetJob(ctx, original.ID)
			if err != nil || current.EnrichedAt.Valid || string(current.Enrichment) != "{}" {
				t.Fatalf("stale completion restored old enrichment: %s, %v", current.Enrichment, err)
			}
			if freshID == oldID {
				t.Fatal("ownership transfer reused its old enrichment lease")
			}
			if err := store.Complete(ctx, enrich.Claimed{OutboxID: freshID, JobID: original.ID, TargetVersion: enrich.Version}, json.RawMessage(`{"summary":"Current native ATS response"}`)); err != nil {
				t.Fatal(err)
			}
			current, err = q.GetJob(ctx, original.ID)
			if err != nil || !current.EnrichedAt.Valid {
				t.Fatalf("fresh native enrichment did not complete: %+v, %v", current, err)
			}
		})
	}
}

// A live claim survives the reap: the reap runs beside the drain, so it can reach a row the
// drain has just claimed for a job that became eligible again. Unclaimed and long-stale
// claims of an ineligible job are still reaped.
func TestReapSkipsLiveClaims(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	q := db.New(pool)
	var ids []int64
	// claim is the claim's age in seconds; -1 means unclaimed.
	for i, claim := range []int{-1, 0, 7200} {
		j, err := q.UpsertJob(ctx, db.UpsertJobParams{Source: "greenhouse", ExternalID: string(rune('a' + i)), URL: "https://x.test/" + string(rune('a'+i)), Title: "Go Engineer", Company: "Acme", PublicSlug: "reap-" + string(rune('a'+i)), CompanySlug: "acme", Description: "Build.", IsTech: pgtype.Bool{Bool: true, Valid: true}})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := q.EnqueueJobEnrichment(ctx, db.EnqueueJobEnrichmentParams{JobID: j.ID, TargetVersion: int32(enrich.Version)}); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, "UPDATE enrichment_outbox SET claimed_at = CASE WHEN $2::int < 0 THEN NULL ELSE now() - make_interval(secs => $2::int) END WHERE job_id=$1", j.ID, claim); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, "UPDATE jobs SET closed_at = now() WHERE id=$1", j.ID); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, j.ID)
	}
	n, err := q.DeleteIneligibleEnrichmentOutbox(ctx, 100)
	if err != nil || n != 2 {
		t.Fatalf("reaped %d, %v; want 2", n, err)
	}
	var left int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM enrichment_outbox WHERE job_id=$1", ids[1]).Scan(&left); err != nil || left != 1 {
		t.Fatalf("live claim reaped: %d, %v", left, err)
	}
}

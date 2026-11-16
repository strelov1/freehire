//go:build integration

package main

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/strelov1/freehire/internal/ai/enrich"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/testdb"
)

func TestInvalidatedClaimCannotOverwritePromotedJobEnrichment(t *testing.T) {
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
	if _, err := q.PromoteJobSource(ctx, db.PromoteJobSourceParams{JobID: original.ID, ExpectedSource: "linkedin", ExpectedExternalID: "search:123", ExpectedUrl: "https://www.linkedin.com/jobs/view/123/", Source: "hrsmart", ExternalID: "acme.hrdepartment.com:123", URL: "https://acme.hrdepartment.com/hrsmart/ats/Posting/view/123"}); err != nil {
		t.Fatal(err)
	}
	if _, err := q.EnqueueJobEnrichment(ctx, db.EnqueueJobEnrichmentParams{JobID: original.ID, TargetVersion: int32(enrich.Version)}); err != nil {
		t.Fatal(err)
	}
	var freshID int64
	if err := pool.QueryRow(ctx, "SELECT id FROM enrichment_outbox WHERE job_id=$1", original.ID).Scan(&freshID); err != nil {
		t.Fatal(err)
	}
	store := newDBStore(pool)
	if err := store.Complete(ctx, enrich.Claimed{OutboxID: oldID, JobID: original.ID, TargetVersion: enrich.Version}, json.RawMessage(`{"summary":"Stale LinkedIn response"}`)); err != nil {
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
}

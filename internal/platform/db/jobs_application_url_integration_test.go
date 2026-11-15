//go:build integration

package db

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestJobApplicationDestinationSurvivesIngest(t *testing.T) {
	pool := startPostgres(t)
	truncate(t, pool)
	q, ctx := New(pool), context.Background()
	p := ingestParams("acme:application-url", "Engineer")
	p.URL = "https://www.linkedin.com/jobs/view/123/"
	created, err := q.UpsertJob(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	const destination = "https://gistworld.mua.hrdepartment.com/hrsmart/ats/Posting/view/3735?src=LinkedIn"
	set := SetJobApplicationURLParams{JobID: created.ID, ApplicationUrl: destination}
	if _, err := q.SetJobApplicationURL(ctx, set); err != nil {
		t.Fatal(err)
	}
	var queued bool
	if err := pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM search_outbox WHERE job_id=$1)", created.ID).Scan(&queued); err != nil || !queued {
		t.Fatalf("destination update not queued: %v, %v", queued, err)
	}
	for _, address := range []string{p.URL, destination + "&utm_source=freehire.me"} {
		slug, err := q.FindOpenJobByURL(ctx, address)
		if err != nil || slug != p.PublicSlug {
			t.Fatalf("lookup %q = %q, %v", address, slug, err)
		}
	}
	var oldQueueID int64
	if err := pool.QueryRow(ctx, "UPDATE search_outbox SET claimed_at=now(), failed_at=now(), attempts=8, last_error='previous failure' WHERE job_id=$1 RETURNING id", created.ID).Scan(&oldQueueID); err != nil {
		t.Fatal(err)
	}
	if _, err := q.UpsertJob(ctx, p); err != nil {
		t.Fatal(err)
	}
	row, err := q.GetJob(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if row.URL != p.URL || row.ApplicationUrl != destination {
		t.Fatalf("re-ingest lost destination or source URL: %q %q", row.URL, row.ApplicationUrl)
	}
	copies, err := q.ListJobCopies(ctx, ListJobCopiesParams{JobID: created.ID, RowLimit: 10})
	if err != nil || len(copies) != 1 || copies[0].URL != destination {
		t.Fatalf("copy destination: %+v, %v", copies, err)
	}
	if _, err := q.SetJobApplicationURL(ctx, set); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("stale curator edit accepted: %v", err)
	}
	set.ExpectedApplicationUrl, set.ApplicationUrl = destination, ""
	if _, err := q.SetJobApplicationURL(ctx, set); err != nil {
		t.Fatal(err)
	}
	var queueID int64
	var retryable bool
	if err := pool.QueryRow(ctx, "SELECT id, attempts=0 AND failed_at IS NULL AND last_error='' FROM search_outbox WHERE job_id=$1", created.ID).Scan(&queueID, &retryable); err != nil || oldQueueID == queueID || !retryable {
		t.Fatalf("queue not replaced/revived: old=%d new=%d retryable=%v err=%v", oldQueueID, queueID, retryable, err)
	}
	if _, err := pool.Exec(ctx, "DELETE FROM search_outbox WHERE id=$1", oldQueueID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM search_outbox WHERE job_id=$1)", created.ID).Scan(&queued); err != nil || !queued {
		t.Fatalf("old drain completion removed update: %v %v", queued, err)
	}
	row, err = q.GetJob(ctx, created.ID)
	if err != nil || row.ApplicationUrl != "" || row.URL != p.URL {
		t.Fatalf("clear: %+v, %v", row, err)
	}
	if _, err := pool.Exec(ctx, "UPDATE jobs SET is_private=true WHERE id=$1", created.ID); err != nil {
		t.Fatal(err)
	}
	set.ExpectedApplicationUrl, set.ApplicationUrl = "", destination
	if _, err := q.SetJobApplicationURL(ctx, set); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("private row edited: %v", err)
	}
	set.JobID = -1
	if _, err := q.SetJobApplicationURL(ctx, set); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("missing row edited: %v", err)
	}
}

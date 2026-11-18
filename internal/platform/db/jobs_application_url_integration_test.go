//go:build integration

package db

import (
	"context"
	"errors"
	"strings"
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
	if slug, err := q.FindOpenJobByURL(ctx, p.URL); err != nil || slug != p.PublicSlug {
		t.Fatalf("source lookup = %q, %v", slug, err)
	}
	if slug, err := q.FindOpenJobByApplicationURL(ctx, destination+"&utm_source=freehire.me"); err != nil || slug != p.PublicSlug {
		t.Fatalf("destination lookup = %q, %v", slug, err)
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
	set.ExpectedApplicationUrl, set.ApplicationUrl = "", destination
	if _, err := q.SetJobApplicationURL(ctx, set); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "UPDATE jobs SET is_private=true WHERE id=$1", created.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := q.FindOpenJobByApplicationURL(ctx, destination); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("private destination exposed: %v", err)
	}
	set.ExpectedApplicationUrl, set.ApplicationUrl = destination, "https://ats.example.org/other"
	if _, err := q.SetJobApplicationURL(ctx, set); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("private row edited: %v", err)
	}
	set.JobID = -1
	if _, err := q.SetJobApplicationURL(ctx, set); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("missing row edited: %v", err)
	}
}

func TestApplicationURLNormalizationPreservesIdentity(t *testing.T) {
	pool := startPostgres(t)
	for _, tc := range []struct {
		left, right string
		equal       bool
	}{
		{"https://ats.test/apply?id=7", "https://ats.test/apply?id=8", false},
		{"http://WWW.ATS.test/Apply/?tenant=acme&id=7#form", "https://ats.test/Apply?id=7&tenant=acme&utm_source=freehire.me&trk=test", true},
		{"https://ats.test/Apply?id=7", "https://ats.test/apply?id=7", false},
		{"https://ats.test/apply?id=7&id=8", "https://ats.test/apply?id=8&id=7", false},
		{"https://ats.test/apply?id=7&tenant=acme", "https://ats.test/apply?id=7&tenant=other", false},
		{"https://ats.test/apply?redirect=next?id=7", "https://ats.test/apply?redirect=next?id=8", false},
	} {
		var equal bool
		if err := pool.QueryRow(context.Background(), "SELECT normalize_application_url($1) = normalize_application_url($2)", tc.left, tc.right).Scan(&equal); err != nil || equal != tc.equal {
			t.Errorf("normalize %q vs %q: equal=%v want=%v err=%v", tc.left, tc.right, equal, tc.equal, err)
		}
	}
}

func TestApplicationURLLookupUsesItsIndex(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	_, err := pool.Exec(ctx, `INSERT INTO jobs (source, external_id, url, title, public_slug, application_url)
	 SELECT 'test', 'destination-' || n, 'https://board.test/jobs/' || n, 'Engineer', 'destination-' || n,
	        'https://ats.test/apply?jobId=' || n FROM generate_series(1, 2000) n`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "ANALYZE jobs; ANALYZE job_activity"); err != nil {
		t.Fatal(err)
	}
	rows, err := pool.Query(ctx, "EXPLAIN "+findOpenJobByApplicationURL, "https://ats.test/apply?jobId=7&utm_source=freehire.me")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var plan strings.Builder
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			t.Fatal(err)
		}
		plan.WriteString(line + "\n")
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(plan.String(), "jobs_normalized_application_url_idx") {
		t.Fatalf("lookup bypassed destination index:\n%s", plan.String())
	}
}

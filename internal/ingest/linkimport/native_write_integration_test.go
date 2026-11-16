//go:build integration

package linkimport

import (
	"context"
	"testing"

	"github.com/strelov1/freehire/internal/ingest/boardcatalog"
	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/job/job"
	"github.com/strelov1/freehire/internal/job/jobderive"
	"github.com/strelov1/freehire/internal/platform/db"
)

func TestWriteNativeTransfersContentAndCrawlOwnershipAtomically(t *testing.T) {
	pool := startPostgres(t)
	ctx, q := context.Background(), db.New(pool)
	origin, err := job.New(job.Draft{Input: jobderive.Input{Source: "linkedin", ExternalID: "software engineer:4472812273", Title: "Senior Java Software Engineer", Company: "Gist Limited", Description: "Description discovered on LinkedIn."}, URL: "https://www.linkedin.com/jobs/view/4472812273/"})
	if err != nil {
		t.Fatal(err)
	}
	created, err := q.UpsertJob(ctx, origin.Fields().UpsertParams())
	if err != nil {
		t.Fatal(err)
	}
	previous, err := q.GetJob(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := q.RequeueSearchOutbox(ctx, created.ID); err != nil {
		t.Fatal(err)
	}
	var oldQueue int64
	if err := pool.QueryRow(ctx, "UPDATE search_outbox SET claimed_at=now(), attempts=8,failed_at=now(),last_error='old lease' WHERE job_id=$1 RETURNING id", created.ID).Scan(&oldQueue); err != nil {
		t.Fatal(err)
	}
	const host = "gistworld.mua.hrdepartment.com"
	minimum, maximum := 90000, 120000
	native := NativeDestination{Entry: sources.CompanyEntry{Provider: "hrsmart", Board: host, Company: "Gist Limited"}, Job: sources.Job{ExternalID: host + ":3735", URL: "https://" + host + "/hrsmart/ats/Posting/view/3735", Title: previous.Title, Company: previous.Company, Description: "The native ATS supplied the current Java role.", Location: "Basingstoke, United Kingdom"}}
	native.Job.SalaryMin, native.Job.SalaryMax = &minimum, &maximum
	native.Job.SalaryCurrency, native.Job.SalaryPeriod = "GBP", "year"
	native.Job.Location = "Remote"
	native.Job.Countries, native.Job.EmploymentType = []string{"gb"}, "full_time"
	im := New(pool, q, nil, nil, sources.All(nil), nil)
	result, err := im.WriteNative(ctx, &previous, native)
	if err != nil {
		t.Fatal(err)
	}
	current, err := q.GetJobBySlug(ctx, previous.PublicSlug)
	if err != nil || current.ID != previous.ID || current.Source != "hrsmart" || current.ExternalID != native.Job.ExternalID || current.URL != native.Job.URL || current.Description != native.Job.Description || result.PublicSlug != previous.PublicSlug {
		t.Fatalf("native ownership/content = %+v, result %+v, %v", current, result, err)
	}
	if !current.SalaryMinSource.Valid || current.SalaryMinSource.Int32 != int32(minimum) || !current.SalaryMaxSource.Valid || current.SalaryMaxSource.Int32 != int32(maximum) || current.SalaryCurrencySource != "GBP" || current.SalaryPeriodSource != "year" || current.EmploymentType != "full_time" || len(current.Countries) != 1 || current.Countries[0] != "gb" {
		t.Fatalf("native structured salary/geography/employment was lost: %+v", current)
	}
	if entry, found, err := boardcatalog.NewQueriesRepository(q).Entry(ctx, "hrsmart", host); err != nil || !found || entry.Company != previous.Company {
		t.Fatalf("native board missing: %+v, %v, %v", entry, found, err)
	}
	if schedule, err := q.GetIngestSchedule(ctx, "hrsmart"); err != nil || !schedule.Enabled || !schedule.Managed {
		t.Fatalf("native owner has no automatic crawl schedule: %+v, %v", schedule, err)
	}
	var newQueue int64
	var retryable bool
	if err := pool.QueryRow(ctx, "SELECT id,attempts=0 AND failed_at IS NULL AND last_error='' FROM search_outbox WHERE job_id=$1", current.ID).Scan(&newQueue, &retryable); err != nil || oldQueue == newQueue || !retryable {
		t.Fatalf("new source was not queued safely: old=%d new=%d retryable=%v, %v", oldQueue, newQueue, retryable, err)
	}
	if err := pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM enrichment_outbox WHERE job_id=$1)", current.ID).Scan(&retryable); err != nil || !retryable {
		t.Fatalf("native content not queued for enrichment: %v %v", retryable, err)
	}
}

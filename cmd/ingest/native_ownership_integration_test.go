//go:build integration

package main

import (
	"context"
	"testing"

	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/job/job"
	"github.com/strelov1/freehire/internal/job/jobderive"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/testdb"
)

type ownershipPostingFixture struct{ posting sources.Job }

func (ownershipPostingFixture) Provider() string { return "linkedin" }
func (s ownershipPostingFixture) Fetch(context.Context, sources.CompanyEntry) ([]sources.Job, error) {
	return []sources.Job{s.posting}, nil
}
func (s ownershipPostingFixture) Posting(context.Context, string) (sources.Job, error) {
	return s.posting, nil
}

func TestOwnershipReconciliationTransfersExistingLinkedInIdentity(t *testing.T) {
	pool := testdb.Pool(t)
	ctx, q := context.Background(), db.New(pool)
	origin, err := job.New(job.Draft{Input: jobderive.Input{Source: "linkedin", ExternalID: "software engineer:4472812273", Title: "Senior Java Software Engineer", Company: "Gist Limited", Description: "Old LinkedIn discovery."}, URL: "https://www.linkedin.com/jobs/view/4472812273/"})
	if err != nil {
		t.Fatal(err)
	}
	created, err := q.UpsertJob(ctx, origin.Fields().UpsertParams())
	if err != nil {
		t.Fatal(err)
	}
	registry := sources.Taxonomy()
	registry["hrsmart"] = nativeDiscoveryFixture{}
	reader := ownershipPostingFixture{posting: sources.Job{ExternalID: "4472812273", Title: "Senior Java Software Engineer", Company: "Gist Limited", URL: "https://www.linkedin.com/jobs/view/4472812273/", ApplyMethod: sources.ApplyExternal, CompanyWebsite: "https://www.gistworld.com", ExternalApplyURL: "https://gistworld.mua.hrdepartment.com/hrsmart/ats/Posting/view/3735"}}
	store := newDBStore(pool, 1, nil, newWriteTally(), defaultSeenPolicy())
	report, err := reconcileLinkedInOwnership(ctx, q, reader, withEmployerProof(newNativeDiscovery(pool, registry, store)), 0)
	if err != nil || report.Transferred != 1 || report.Pending != 0 {
		t.Fatalf("ownership report = %+v, %v", report, err)
	}
	current, err := q.GetJob(ctx, created.ID)
	if err != nil || current.Source != "hrsmart" || current.PublicSlug != origin.Fields().PublicSlug {
		t.Fatalf("identity was not preserved: %+v, %v", current, err)
	}
}

func TestOwnershipReconciliationDoesNotCallUnresolvedDiscoveriesComplete(t *testing.T) {
	pool := testdb.Pool(t)
	ctx, q := context.Background(), db.New(pool)
	origin, err := job.New(job.Draft{Input: jobderive.Input{Source: "linkedin", ExternalID: "Go:123", Title: "Go Engineer", Company: "Unknown Employer", Description: "Build Go services."}, URL: "https://www.linkedin.com/jobs/view/123/"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.UpsertJob(ctx, origin.Fields().UpsertParams()); err != nil {
		t.Fatal(err)
	}
	registry := sources.Taxonomy()
	reader := ownershipPostingFixture{posting: sources.Job{ExternalID: "123", Title: "Go Engineer", Company: "Unknown Employer", URL: "https://www.linkedin.com/jobs/view/123/", ApplyMethod: sources.ApplyExternal}}
	store := newDBStore(pool, 1, nil, newWriteTally(), defaultSeenPolicy())
	report, err := reconcileLinkedInOwnership(ctx, q, reader, withEmployerProof(newNativeDiscovery(pool, registry, store)), 0)
	current, readErr := q.GetJobBySourceExternalID(ctx, db.GetJobBySourceExternalIDParams{Source: "linkedin", ExternalID: "Go:123"})
	if readErr != nil || !current.ClosedAt.Valid || current.ClosedReason != "native_source_pending" {
		t.Fatalf("unverified legacy discovery is still publicly listed: %+v, %v", current, readErr)
	}
	if err == nil || report.Pending != 1 || report.Native != 0 {
		t.Fatalf("unresolved external posting passed as native: %+v, %v", report, err)
	}
}

func TestOwnershipReconciliationRestoresOnlyVerifiedPendingApplications(t *testing.T) {
	pool := testdb.Pool(t)
	ctx, q := context.Background(), db.New(pool)
	origin, err := job.New(job.Draft{Input: jobderive.Input{Source: "linkedin", ExternalID: "Go:123", Title: "Go Engineer", Company: "Unknown Employer", Description: "Build Go services."}, URL: "https://www.linkedin.com/jobs/view/123/"})
	if err != nil {
		t.Fatal(err)
	}
	created, err := q.UpsertJob(ctx, origin.Fields().UpsertParams())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.WithholdLinkedInDiscovery(ctx, created.ID); err != nil {
		t.Fatal(err)
	}
	reader := ownershipPostingFixture{posting: sources.Job{ExternalID: "123", ApplyMethod: sources.ApplyOnSite}}
	report, err := reconcileLinkedInOwnership(ctx, q, reader, nil, 0)
	if err != nil || report.Native != 1 {
		t.Fatalf("confirmed restoration = %+v, %v", report, err)
	}
	current, err := q.GetJob(ctx, created.ID)
	if err != nil || current.ClosedAt.Valid || current.PublicSlug != origin.Fields().PublicSlug {
		t.Fatalf("pending identity not restored: %+v, %v", current, err)
	}
	if _, err := pool.Exec(ctx, "UPDATE jobs SET closed_at = now(), closed_reason = 'employer_closed' WHERE id = $1", created.ID); err != nil {
		t.Fatal(err)
	}
	count, err := q.RestoreConfirmedLinkedInApplication(ctx, created.ID)
	if err != nil || count != 0 {
		t.Fatalf("genuine closure restored: %d, %v", count, err)
	}
	report, err = reconcileLinkedInOwnership(ctx, q, reader, nil, 0)
	if err != nil || report.Native != 0 {
		t.Fatalf("genuine closure reconciled: %+v, %v", report, err)
	}
}

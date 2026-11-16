//go:build integration

package main

import (
	"context"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/job/job"
	"github.com/strelov1/freehire/internal/job/jobderive"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/testdb"
)

func linkedInStorePosting() job.Job {
	j, err := job.New(job.Draft{
		Input: jobderive.Input{Source: "linkedin", ExternalID: "4490679811", Title: "Backend Engineer", Company: "Acme", Description: "Build Go services."},
		URL:   "https://www.linkedin.com/jobs/view/4490679811/",
	})
	if err != nil {
		panic(err)
	}
	return j
}

func TestSave_LinkedInUnchangedLegacyRowUsesCheapWrite(t *testing.T) {
	pool, ctx := testdb.Pool(t), context.Background()
	store := newDBStore(pool, 1, nil, newWriteTally(), defaultSeenPolicy())
	posting := linkedInStorePosting()
	if err := store.Save(ctx, posting); err != nil {
		t.Fatal(err)
	}
	// Model a row first stored by the keyword-namespaced crawler, with the same body.
	if _, err := pool.Exec(ctx, `UPDATE jobs SET external_id='software engineer:4490679811',updated_at=now()-interval '10 days' WHERE source='linkedin'`); err != nil {
		t.Fatal(err)
	}
	before, err := db.New(pool).GetJobBySourceExternalID(ctx, db.GetJobBySourceExternalIDParams{Source: "linkedin", ExternalID: "software engineer:4490679811"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(ctx, posting); err != nil {
		t.Fatal(err)
	}
	after, err := db.New(pool).GetJob(ctx, before.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.ID != before.ID || !after.UpdatedAt.Time.Equal(before.UpdatedAt.Time) {
		t.Fatalf("unchanged legacy recrawl rewrote job: id %d->%d updated_at %v->%v", before.ID, after.ID, before.UpdatedAt.Time, after.UpdatedAt.Time)
	}
}

func TestRefreshCovered_LinkedInNumericIDRefreshesLegacyRowOnlyWhenOpen(t *testing.T) {
	pool, ctx := testdb.Pool(t), context.Background()
	store := newDBStore(pool, 1, nil, newWriteTally(), defaultSeenPolicy())
	if _, err := pool.Exec(ctx, `INSERT INTO jobs(source,external_id,url,title,company_slug,public_slug)
		VALUES ('linkedin','software engineer:4490679811','https://www.linkedin.com/jobs/view/4490679811/','Backend Engineer','acme','legacy-covered-linkedin')`); err != nil {
		t.Fatal(err)
	}
	stored, err := db.New(pool).GetJobBySourceExternalID(ctx, db.GetJobBySourceExternalIDParams{Source: "linkedin", ExternalID: "software engineer:4490679811"})
	if err != nil {
		t.Fatal(err)
	}
	age := func() {
		if _, err := pool.Exec(ctx, `UPDATE job_activity SET last_seen_at=now()-interval '10 days' WHERE job_id=$1`, stored.ID); err != nil {
			t.Fatal(err)
		}
	}
	age()
	refreshed, err := store.RefreshCovered(ctx, "linkedin", []string{"4490679811"})
	if err != nil || len(refreshed) != 1 || refreshed[0] != "acme" {
		t.Fatalf("legacy unreadable refresh=%v error=%v", refreshed, err)
	}
	current, err := db.New(pool).GetJob(ctx, stored.ID)
	if err != nil || time.Since(current.LastSeenAt.Time) > time.Minute {
		t.Fatalf("legacy last_seen_at=%v error=%v", current.LastSeenAt.Time, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE jobs SET closed_at=now(),closed_reason='unseen' WHERE id=$1`, stored.ID); err != nil {
		t.Fatal(err)
	}
	age()
	refreshed, err = store.RefreshCovered(ctx, "linkedin", []string{"4490679811"})
	if err != nil || len(refreshed) != 0 {
		t.Fatalf("closed unreadable refresh=%v error=%v", refreshed, err)
	}
	current, err = db.New(pool).GetJob(ctx, stored.ID)
	if err != nil || !current.ClosedAt.Valid || time.Since(current.LastSeenAt.Time) < 9*24*time.Hour {
		t.Fatalf("closed history changed: closed=%t seen=%v error=%v", current.ClosedAt.Valid, current.LastSeenAt.Time, err)
	}
}

func TestCloseCovered_LinkedInNumericIDRetiresLegacyRow(t *testing.T) {
	pool, ctx := testdb.Pool(t), context.Background()
	store := newDBStore(pool, 1, nil, newWriteTally(), defaultSeenPolicy())
	if _, err := pool.Exec(ctx, `INSERT INTO jobs(source,external_id,url,title,company_slug,public_slug)
		VALUES ('linkedin','software engineer:4490679811','https://www.linkedin.com/jobs/view/4490679811/','Backend Engineer','acme','legacy-covered-retirement')`); err != nil {
		t.Fatal(err)
	}
	closed, err := store.CloseCovered(ctx, "linkedin", []string{"4490679811"})
	if err != nil || len(closed) != 1 || closed[0] != "acme" {
		t.Fatalf("legacy covered close=%v error=%v", closed, err)
	}
	stored, err := db.New(pool).GetJobBySourceExternalID(ctx, db.GetJobBySourceExternalIDParams{Source: "linkedin", ExternalID: "software engineer:4490679811"})
	if err != nil || !stored.ClosedAt.Valid || stored.ClosedReason != "ats_covered" {
		t.Fatalf("covered history: closed=%t reason=%q error=%v", stored.ClosedAt.Valid, stored.ClosedReason, err)
	}
}

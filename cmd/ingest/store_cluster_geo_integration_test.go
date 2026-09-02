//go:build integration

// Integration test for the write-path search-index enqueue around a collapsed role: a
// content change on the canon must re-queue it in search_outbox so cmd/search-drain
// picks it up (that worker owns building the document and widening it with the
// cluster's geography — see cmd/search-drain's own integration test for that part).
// Run with: go test -tags=integration ./cmd/ingest/
package main

import (
	"context"
	"testing"

	"github.com/strelov1/freehire/internal/ingest/pipeline"
	"github.com/strelov1/freehire/internal/job/job"
	"github.com/strelov1/freehire/internal/job/jobderive"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/testdb"
)

// editedCityPosting is cityPosting with a changed apply URL: a content change the index
// push reacts to, chosen because it leaves the title and the description alone. Those two
// are what RoleFingerprint hashes, so editing either would move the canon into a cluster of
// its own — the role would become a singleton and there would be nothing to widen with,
// which is the wrong reason for this test to pass or fail.
func editedCityPosting(externalID, location string) job.Job {
	j, err := job.New(job.Draft{
		Input: jobderive.Input{
			Source:      "zohorecruit",
			ExternalID:  externalID,
			Title:       "Senior Full Stack Engineer ID78855",
			Company:     "AgileEngine",
			Location:    location,
			Description: "<div>We are looking for a Senior Full Stack Engineer with a backend orientation.</div>",
		},
		URL: "https://agileengine.zohorecruit.com/jobs/Careers/" + externalID + "?v=2",
	})
	if err != nil {
		panic(err)
	}
	return j
}

func TestSave_ContentChangeRequeuesTheCanonForSearch(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	q := db.New(pool)

	store := newDBStore(pool, 1, nil, nil, pipeline.HydrationRetryWindow, false)

	// The same role crawled once per city: the first is the canon, the second a repost
	// that is kept as a row but never queued for the index.
	const canonID, repostID = "248544000257794970", "248544000257794973"
	if err := store.Save(ctx, cityPosting(canonID, "Querétaro, Mexico")); err != nil {
		t.Fatalf("save the canon: %v", err)
	}
	if err := store.Save(ctx, cityPosting(repostID, "Bogota, Colombia")); err != nil {
		t.Fatalf("save the repost: %v", err)
	}

	canon, err := q.GetJobBySourceExternalID(ctx, db.GetJobBySourceExternalIDParams{
		Source: "zohorecruit", ExternalID: canonID,
	})
	if err != nil {
		t.Fatalf("load the canon: %v", err)
	}
	repost, err := q.GetJobBySourceExternalID(ctx, db.GetJobBySourceExternalIDParams{
		Source: "zohorecruit", ExternalID: repostID,
	})
	if err != nil {
		t.Fatalf("load the repost: %v", err)
	}
	if got := searchOutboxCount(t, pool, canon.ID); got != 1 {
		t.Fatalf("search_outbox entries for the canon after the first save = %d, want 1", got)
	}
	if got := searchOutboxCount(t, pool, repost.ID); got != 0 {
		t.Fatalf("search_outbox entries for the non-canonical repost = %d, want 0", got)
	}

	// Drain the outbox row the insert queued, so the assertion below observes only
	// what the content change itself queues.
	if _, err := pool.Exec(ctx, `DELETE FROM search_outbox WHERE job_id = $1`, canon.ID); err != nil {
		t.Fatalf("drain search_outbox after insert: %v", err)
	}

	// The next crawl reports a content change on the canon, so it must be re-queued —
	// cmd/search-drain rebuilds the document (including the cluster geography) from
	// the persisted row when it drains this entry.
	if err := store.Save(ctx, editedCityPosting(canonID, "Querétaro, Mexico")); err != nil {
		t.Fatalf("re-save the canon: %v", err)
	}
	if got := searchOutboxCount(t, pool, canon.ID); got != 1 {
		t.Errorf("search_outbox entries for the canon after the content change = %d, want 1", got)
	}
}

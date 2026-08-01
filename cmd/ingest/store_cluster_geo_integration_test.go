//go:build integration

// Integration test for the incremental index push and a collapsed role's geography: the
// push is a field-level document update and the geography facets are always present in the
// payload, so a writer that does not widen the canon with its cluster's union actively
// REPLACES the reindex's widened values with the canon's own narrow set — and the role
// stops being findable by the cities its reposts hold until the next full rebuild.
// Run with: go test -tags=integration ./cmd/ingest/
package main

import (
	"context"
	"slices"
	"testing"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/job"
	"github.com/strelov1/freehire/internal/jobderive"
	"github.com/strelov1/freehire/internal/testdb"
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

func TestSave_IncrementalPushKeepsTheClusterGeography(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	q := db.New(pool)

	pusher := &fakePusher{}
	store := newDBStore(pool, 1, newBatchIndexer(pusher.push, 1), nil)

	// The same role crawled once per city: the first is the canon, the second a repost
	// that is kept as a row but never indexed.
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
	if len(canon.Cities) == 0 || len(repost.Cities) == 0 {
		t.Fatalf("the fixture needs both rows to carry a city, got canon=%v repost=%v",
			canon.Cities, repost.Cities)
	}

	// The next crawl reports a content change on the canon, so it is pushed again. That
	// push must not narrow the document back to the canon's own city.
	if err := store.Save(ctx, editedCityPosting(canonID, "Querétaro, Mexico")); err != nil {
		t.Fatalf("re-save the canon: %v", err)
	}

	// The first save pushed the canon once; the re-save must push it again, or the
	// assertion below would be reading the original push and pass for the wrong reason.
	docs := pusher.all()
	if len(docs) < 2 {
		t.Fatalf("the content change pushed nothing new: %d document(s) pushed in total", len(docs))
	}
	last := docs[len(docs)-1]
	if last.ID != canon.ID {
		t.Fatalf("last pushed document is job %d, want the canon %d", last.ID, canon.ID)
	}

	want := slices.Concat(canon.Cities, repost.Cities)
	slices.Sort(want)
	want = slices.Compact(want)
	if !slices.Equal(last.Cities, want) {
		t.Errorf("the pushed canon carries cities %v, want the cluster union %v — an incremental "+
			"push replaces the geography facets, so omitting the union un-widens the canon",
			last.Cities, want)
	}
}

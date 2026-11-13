//go:build integration

package main

import (
	"context"
	"testing"

	"github.com/strelov1/freehire/internal/job/job"
	"github.com/strelov1/freehire/internal/job/jobderive"
	"github.com/strelov1/freehire/internal/platform/testdb"
)

// A body the pipeline took from the catalogue for a failed fetch was not read, so a write
// carrying it must leave hydrated_at alone — on the cheap path and the full one — or the stale
// body drops out of BODY_REFRESH_DAYS' re-read set without being re-read.
func TestSave_BodyFromCatalogueLeavesHydratedAt(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	store := newDBStore(pool, 1, nil, nil, defaultSeenPolicy())
	const id = "acme:body-from-catalogue"
	if err := store.Save(ctx, cheapPosting(id, "Backend Engineer")); err != nil {
		t.Fatalf("first Save: %v", err)
	}
	stored := loadJob(t, pool, id)
	substituted := func(title string) job.Job {
		j, err := job.New(job.Draft{
			Input: jobderive.Input{
				Source: "lever", ExternalID: id, Title: title, Company: "Acme",
				Location: "Berlin, Germany", Description: stored.Description,
			},
			URL:               "https://jobs.lever.co/acme/" + id,
			BodyFromCatalogue: true,
		})
		if err != nil {
			t.Fatal(err)
		}
		return j
	}
	backdate := func() {
		if _, err := pool.Exec(ctx, `UPDATE jobs SET hydrated_at = now() - interval '40 days'
			WHERE source = 'lever' AND external_id = $1`, id); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `UPDATE job_activity a SET hydrated_at = j.hydrated_at
			FROM jobs j WHERE a.job_id=j.id AND j.source='lever' AND j.external_id=$1`, id); err != nil {
			t.Fatal(err)
		}
	}

	for _, tc := range []struct{ name, title string }{
		{"cheap write", "Backend Engineer"},       // same content hash
		{"full write", "Senior Backend Engineer"}, // title moved, so UpsertJob
	} {
		backdate()
		before := loadJob(t, pool, id).HydratedAt.Time
		if err := store.Save(ctx, substituted(tc.title)); err != nil {
			t.Fatalf("%s: Save: %v", tc.name, err)
		}
		if after := loadJob(t, pool, id).HydratedAt.Time; !after.Equal(before) {
			t.Errorf("%s: hydrated_at moved %v -> %v for a body that was not read", tc.name, before, after)
		}
	}

	backdate()
	before := loadJob(t, pool, id).HydratedAt.Time
	if err := store.Save(ctx, cheapPosting(id, "Senior Backend Engineer")); err != nil {
		t.Fatalf("fetched Save: %v", err)
	}
	if after := loadJob(t, pool, id).HydratedAt.Time; !after.After(before) {
		t.Errorf("hydrated_at = %v, want stamped for a body the crawl read", after)
	}
}

// The substituted body was read before the write's transaction. A newer body stored in between
// must win: the write is refused and the newer body and its facets stay.
func TestSave_BodyFromCatalogueRefusesToOverwriteANewerBody(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	store := newDBStore(pool, 1, nil, nil, defaultSeenPolicy())
	const id = "acme:body-from-catalogue-race"
	if err := store.Save(ctx, cheapPosting(id, "Backend Engineer")); err != nil {
		t.Fatalf("first Save: %v", err)
	}
	lookedUp := loadJob(t, pool, id).Description
	const newer = "<p>We now build services in Python on PostgreSQL.</p>"
	if _, err := pool.Exec(ctx, `UPDATE jobs SET description = $2 WHERE source = 'lever' AND external_id = $1`, id, newer); err != nil {
		t.Fatal(err)
	}
	stale, err := job.New(job.Draft{
		Input: jobderive.Input{
			Source: "lever", ExternalID: id, Title: "Senior Backend Engineer", Company: "Acme",
			Location: "Berlin, Germany", Description: lookedUp,
		},
		URL:               "https://jobs.lever.co/acme/" + id,
		BodyFromCatalogue: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(ctx, stale); err == nil {
		t.Error("Save succeeded over a newer body, want it refused")
	}
	if got := loadJob(t, pool, id).Description; got != newer {
		t.Errorf("description = %q, want the newer body kept", got)
	}
}

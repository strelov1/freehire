//go:build integration

package main

import (
	"context"
	"testing"

	"github.com/strelov1/freehire/internal/dict/normalize"
	"github.com/strelov1/freehire/internal/ingest/applyform"
	"github.com/strelov1/freehire/internal/job/job"
	"github.com/strelov1/freehire/internal/job/jobderive"
	"github.com/strelov1/freehire/internal/platform/testdb"
)

// These distinct identities have the same real 40-bit shortcode (75jwd7n2).
// Different descriptions keep them out of the role-repost cluster.
func slugCollisionPosting(t *testing.T, id, description string) job.Job {
	t.Helper()
	posting, err := job.New(job.Draft{
		Input: jobderive.Input{
			Source: "lever", ExternalID: id, Title: "Backend Engineer", Company: "Acme",
			Location: "Berlin, Germany", Description: description,
		},
		URL: "https://example.com/jobs/" + id,
	})
	if err != nil {
		t.Fatal(err)
	}
	return posting
}

func TestSave_PublicSlugCollisionPreservesBothPostings(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	tally := newWriteTally()
	store := newDBStore(pool, 1, nil, tally, defaultSeenPolicy())
	first := slugCollisionPosting(t, "collision-224028", "<p>Build a Go payments platform.</p>")
	second := slugCollisionPosting(t, "collision-1908612", "<p>Build a Python analytics platform.</p>")
	if first.Fields().PublicSlug != second.Fields().PublicSlug {
		t.Fatal("fixture must collide before persistence")
	}
	if err := store.Save(ctx, first); err != nil {
		t.Fatal(err)
	}
	form := applyform.Form{Provider: "lever"}
	if err := store.SaveWithApplyForm(ctx, second, form); err != nil {
		t.Fatalf("save distinct posting with colliding slug: %v", err)
	}
	a := loadJob(t, pool, first.Fields().ExternalID)
	b := loadJob(t, pool, second.Fields().ExternalID)
	if a.PublicSlug != first.Fields().PublicSlug || a.ID == b.ID || a.PublicSlug == b.PublicSlug {
		t.Fatal("collision must preserve the first URL and insert a distinct second posting")
	}
	if code, ok := normalize.JobSlugCode(b.PublicSlug); !ok || code != "75jwd7n2" {
		t.Fatalf("fallback shortcode = %q, %v; want the original identity code", code, ok)
	}
	if b.DuplicateOf.Valid || searchOutboxCount(t, pool, b.ID) != 1 || !enrichmentQueued(t, pool, b.ID) {
		t.Fatal("recovered posting must reach the normal search and enrichment queues")
	}
	if provider, _ := storedForm(t, pool, b.ID); provider != form.Provider {
		t.Fatal("collision retry must preserve the application form")
	}
	if _, err := pool.Exec(ctx, `DELETE FROM search_outbox WHERE job_id = $1`, b.ID); err != nil {
		t.Fatal(err)
	}
	backdateStamps(t, pool, second.Fields().ExternalID)
	if err := store.Save(ctx, second); err != nil {
		t.Fatalf("unchanged recrawl: %v", err)
	}
	after := loadJob(t, pool, second.Fields().ExternalID)
	if after.PublicSlug != b.PublicSlug || searchOutboxCount(t, pool, b.ID) != 0 {
		t.Fatal("unchanged recrawl must preserve the fallback URL without reindexing")
	}
	if got := tally.summary(); got != "lever cheap=1/3 (33%)" {
		t.Fatalf("tally = %q; rolled-back attempts must not count", got)
	}
	second = slugCollisionPosting(t, second.Fields().ExternalID, "<p>Maintain the Python analytics platform.</p>")
	if err := store.Save(ctx, second); err != nil {
		t.Fatalf("edited recrawl: %v", err)
	}
	edited := loadJob(t, pool, second.Fields().ExternalID)
	if edited.PublicSlug != b.PublicSlug || edited.Description != second.Fields().Description || searchOutboxCount(t, pool, b.ID) != 1 {
		t.Fatal("edited recrawl must update content and search without moving the published URL")
	}
}

//go:build integration

// Integration tests for the cheap write seam in save: a re-crawl of an unchanged posting must
// take RefreshUnchangedJob and write only its liveness, while everything the write path does
// AROUND that write — the enrichment enqueue, the apply-form write, the crawled-set record —
// keeps happening, and a closed posting still reopens through the full upsert.
// Run with: go test -tags=integration ./cmd/ingest/
// Requires Docker (testcontainers spins up a throwaway Postgres with the migrations).
package main

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/applyform"
	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/job"
	"github.com/strelov1/freehire/internal/jobderive"
	"github.com/strelov1/freehire/internal/pipeline"
	"github.com/strelov1/freehire/internal/testdb"
)

// cheapPosting is a plain board posting. The package's integration tests share one database and
// keep apart by distinct external ids rather than truncating.
func cheapPosting(externalID, title string) job.Job {
	j, err := job.New(job.Draft{
		Input: jobderive.Input{
			Source:      "lever",
			ExternalID:  externalID,
			Title:       title,
			Company:     "Acme",
			Location:    "Berlin, Germany",
			Description: "<p>We are looking for a backend engineer to build services.</p>",
		},
		URL: "https://jobs.lever.co/acme/" + externalID,
	})
	if err != nil {
		panic(err)
	}
	return j
}

// backdateStamps ages a row's liveness and change stamps so a write that touches either is
// visible as a move rather than as two equal timestamps.
func backdateStamps(t *testing.T, pool *pgxpool.Pool, externalID string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		`UPDATE jobs SET last_seen_at = now() - interval '10 days', updated_at = now() - interval '10 days'
		 WHERE source = 'lever' AND external_id = $1`, externalID,
	); err != nil {
		t.Fatalf("back-date stamps for %s: %v", externalID, err)
	}
}

func loadJob(t *testing.T, pool *pgxpool.Pool, externalID string) db.Job {
	t.Helper()
	row, err := db.New(pool).GetJobBySourceExternalID(context.Background(), db.GetJobBySourceExternalIDParams{
		Source: "lever", ExternalID: externalID,
	})
	if err != nil {
		t.Fatalf("load job %q: %v", externalID, err)
	}
	return row
}

// searchOutboxCount reports how many live search_outbox entries a job has queued —
// 0 or 1, since EnqueueSearchOutbox is ON CONFLICT (job_id) DO NOTHING.
func searchOutboxCount(t *testing.T, pool *pgxpool.Pool, jobID int64) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM search_outbox WHERE job_id = $1`, jobID,
	).Scan(&n); err != nil {
		t.Fatalf("count search_outbox for job %d: %v", jobID, err)
	}
	return n
}

// A second crawl of a posting nobody edited must refresh its liveness and write nothing else —
// while everything around the write still runs, because the sweep, the enrichment queue and the
// apply-form queue all depend on it.
func TestSave_UnchangedRecrawlWritesOnlyLiveness(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	crawled := newCrawledSet()
	tally := newWriteTally()
	store := newDBStore(pool, 1, crawled, tally, pipeline.HydrationRetryWindow)
	posting := cheapPosting("acme:cheap-1", "Backend Engineer")

	if err := store.Save(ctx, posting); err != nil {
		t.Fatalf("first Save: %v", err)
	}
	backdateStamps(t, pool, "acme:cheap-1")
	before := loadJob(t, pool, "acme:cheap-1")
	// Drain the outbox entry the insert queued, so the assertion below observes only
	// what the re-crawl itself queues.
	if _, err := pool.Exec(ctx, `DELETE FROM search_outbox WHERE job_id = $1`, before.ID); err != nil {
		t.Fatalf("drain search_outbox after insert: %v", err)
	}

	if err := store.Save(ctx, posting); err != nil {
		t.Fatalf("re-crawl Save: %v", err)
	}

	after := loadJob(t, pool, "acme:cheap-1")
	if !after.LastSeenAt.Time.After(before.LastSeenAt.Time) {
		t.Errorf("last_seen_at = %v, want advanced — the unseen sweep depends on it",
			after.LastSeenAt.Time)
	}
	if !after.UpdatedAt.Time.Equal(before.UpdatedAt.Time) {
		t.Errorf("updated_at = %v, want unchanged %v — an unedited posting did not change",
			after.UpdatedAt.Time, before.UpdatedAt.Time)
	}
	if after.Description != before.Description || after.ContentHash != before.ContentHash {
		t.Errorf("content rewritten on an unchanged re-crawl:\n before = %q/%v\n after  = %q/%v",
			before.Description, before.ContentHash, after.Description, after.ContentHash)
	}

	// No search-index queue entry: the row's indexed content did not move.
	if got := searchOutboxCount(t, pool, before.ID); got != 0 {
		t.Errorf("search_outbox entries = %d, want 0 for a re-crawl that changed nothing", got)
	}
	// The company must still be in the sweep scope, or its removed postings never close.
	if slugs := crawled.slugs("lever"); len(slugs) != 1 || slugs[0] != "acme" {
		t.Errorf("crawled slugs = %v, want [acme] — a company written only by cheap writes "+
			"would drop out of the post-run sweep", slugs)
	}
	// The enrichment enqueue is deliberately kept on this path, so a never-enriched posting
	// is still re-offered on every crawl.
	if !enrichmentQueued(t, pool, before.ID) {
		t.Error("enrichment not queued after the re-crawl, want the enqueue to still run")
	}
	// The run must be able to SAY the cheap path was reached — one insert, then one cheap
	// refresh. Without this the share is assumed rather than measured.
	if got, want := tally.summary(), "lever cheap=1/2 (50%)"; got != want {
		t.Errorf("tally = %q, want %q", got, want)
	}
}

// The regression guard for RefreshUnchangedJob's `closed_at IS NULL`: a posting that was closed
// and reappears unchanged must reach the full upsert, which is what reopens it. Without the
// predicate the cheap path would refresh its liveness and leave it closed forever, invisible
// while the sweep kept seeing it.
func TestSave_ClosedPostingReopensOnUnchangedRecrawl(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	store := newDBStore(pool, 1, nil, nil, pipeline.HydrationRetryWindow)
	posting := cheapPosting("acme:cheap-2", "Platform Engineer")

	if err := store.Save(ctx, posting); err != nil {
		t.Fatalf("first Save: %v", err)
	}
	backdateStamps(t, pool, "acme:cheap-2")
	if _, err := pool.Exec(ctx,
		`UPDATE jobs SET closed_at = now(), closed_reason = 'unseen', liveness_strikes = 2
		 WHERE source = 'lever' AND external_id = 'acme:cheap-2'`,
	); err != nil {
		t.Fatalf("close setup: %v", err)
	}
	before := loadJob(t, pool, "acme:cheap-2")

	if err := store.Save(ctx, posting); err != nil {
		t.Fatalf("re-crawl Save: %v", err)
	}

	after := loadJob(t, pool, "acme:cheap-2")
	if after.ClosedAt.Valid {
		t.Error("closed_at still set, want reopened")
	}
	if after.ClosedReason != "" {
		t.Errorf("closed_reason = %q, want cleared on reopen", after.ClosedReason)
	}
	if after.LivenessStrikes != 0 {
		t.Errorf("liveness_strikes = %d, want reset to 0 on reopen", after.LivenessStrikes)
	}
	// A reopen IS a change the search reconciler must see.
	if !after.UpdatedAt.Time.After(before.UpdatedAt.Time) {
		t.Errorf("updated_at = %v, want stamped on reopen", after.UpdatedAt.Time)
	}
}

// The apply form is written after the seam, so it must land on both branches — otherwise a
// Recruitee board would stop refreshing its stored forms the moment its postings settled.
func TestSaveWithApplyForm_UnchangedRecrawlStillWritesTheForm(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	store := newDBStore(pool, 1, nil, nil, pipeline.HydrationRetryWindow)
	posting := cheapPosting("acme:cheap-3", "Data Engineer")

	first := applyform.Form{Provider: "lever", Fields: []applyform.Field{{ID: "1", Label: "Old question", RawType: "string"}}}
	if err := store.SaveWithApplyForm(ctx, posting, first); err != nil {
		t.Fatalf("first SaveWithApplyForm: %v", err)
	}
	backdateStamps(t, pool, "acme:cheap-3")
	before := loadJob(t, pool, "acme:cheap-3")

	// The posting itself is byte-identical, so this re-crawl takes the cheap write — and the
	// employer's edited form must still be stored.
	second := applyform.Form{Provider: "lever", Fields: []applyform.Field{{ID: "1", Label: "New question", RawType: "string"}}}
	if err := store.SaveWithApplyForm(ctx, posting, second); err != nil {
		t.Fatalf("re-crawl SaveWithApplyForm: %v", err)
	}

	id := jobIDFor(t, pool, "lever", "acme:cheap-3")
	_, got := storedForm(t, pool, id)
	if len(got.Fields) != 1 || got.Fields[0].Label != "New question" {
		t.Errorf("stored form = %+v, want the later capture — the form write must not be "+
			"skipped when the posting itself is unchanged", got.Fields)
	}

	// That the CHEAP branch is what ran has to be asserted on updated_at, not on last_seen_at:
	// both branches advance the liveness stamp, so it cannot tell them apart and a seam that
	// stopped working would leave this test green.
	if j := loadJob(t, pool, "acme:cheap-3"); !j.UpdatedAt.Time.Equal(before.UpdatedAt.Time) {
		t.Errorf("updated_at = %v, want unchanged %v — the form write must not drag the posting "+
			"onto the full upsert", j.UpdatedAt.Time, before.UpdatedAt.Time)
	}
}

// A hydrating source re-lists an offer through Touch, not Save — and Touch is that source's
// cheap write, refreshing liveness without rewriting content. It has to be counted, or every
// hydrating provider reports a 0% cheap share and reads as the exact churn the run-end line
// exists to expose: a false alarm produced by construction, on nine providers.
func TestTouch_CountsAsACheapWrite(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	tally := newWriteTally()
	store := newDBStore(pool, 1, newCrawledSet(), tally, pipeline.HydrationRetryWindow)

	if err := store.Save(ctx, cheapPosting("acme:cheap-4", "Site Engineer")); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := store.Touch(ctx, "lever", "acme:cheap-4"); err != nil {
		t.Fatalf("Touch: %v", err)
	}

	if got, want := tally.summary(), "lever cheap=1/2 (50%)"; got != want {
		t.Errorf("tally = %q, want %q — the touch is a cheap write and must be counted", got, want)
	}
}

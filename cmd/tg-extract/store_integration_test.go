//go:build integration

// Integration test for the extraction worker's store adapter: an extracted vacancy is
// queued for the live facet index atomically with its row, like every other write path.
// Run with: go test -tags=integration ./cmd/tg-extract/
// Requires Docker (testcontainers spins up a throwaway Postgres with the migrations).
package main

import (
	"context"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/ingest/telegram"
	"github.com/strelov1/freehire/internal/job/job"
	"github.com/strelov1/freehire/internal/job/jobderive"
	"github.com/strelov1/freehire/internal/platform/testdb"
)

// Telegram extraction is a stream, not the occasional curated write, and it wrote jobs
// without queueing them: they reached /jobs/search and the facet counts only on the next
// full rebuild-and-swap. cmd/ingest and cmd/hydrate-adzuna-description both queue here.
func TestWriteQueuesEachExtractedJobForTheLiveIndex(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	store := newExtractStore(pool)

	post := telegram.PendingPost{
		Channel:  "devjobs",
		MsgID:    4242,
		Text:     "Backend Engineer at Acme",
		PostedAt: time.Now(),
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO telegram_posts (channel, msg_id, text, posted_at) VALUES ($1, $2, $3, $4)`,
		post.Channel, post.MsgID, post.Text, post.PostedAt); err != nil {
		t.Fatalf("seed post: %v", err)
	}

	j, err := job.New(job.Draft{Input: jobderive.Input{
		Source:      "telegram",
		ExternalID:  "devjobs:4242:0",
		Title:       "Backend Engineer",
		Company:     "Acme",
		Location:    "Berlin, Germany",
		Description: "We are hiring a backend engineer to work on Go services.",
	}})
	if err != nil {
		t.Fatalf("build job: %v", err)
	}

	if err := store.Complete(ctx, post, []job.Job{j}); err != nil {
		t.Fatalf("Complete: %v", err)
	}

	var queued int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM search_outbox o
		   JOIN jobs jb ON jb.id = o.job_id
		  WHERE jb.source = $1 AND jb.external_id = $2`,
		"telegram", "devjobs:4242:0").Scan(&queued); err != nil {
		t.Fatalf("count search_outbox: %v", err)
	}
	if queued != 1 {
		t.Errorf("search_outbox holds %d entries for the extracted job, want 1", queued)
	}
}

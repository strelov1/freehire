//go:build integration

package main

import (
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/job/liveness"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/testdb"
)

func TestDiceProbeDoesNotClosePostingSeenAgain(t *testing.T) {
	ctx := t.Context()
	pool := testdb.Pool(t)
	q := db.New(pool)
	const slug = "dice-seen-again"
	var id int64
	if err := pool.QueryRow(ctx, `INSERT INTO jobs (source,external_id,url,title,public_slug)
		VALUES ('dice',':99ccfb87-b404-47de-90e6-bae6eebc958b','https://www.dice.com/job-detail/99ccfb87-b404-47de-90e6-bae6eebc958b',$1,$1)
		RETURNING id`, slug).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE job_activity SET last_seen_at = now()-interval '3 days', liveness_strikes = 1 WHERE job_id = $1`, id); err != nil {
		t.Fatal(err)
	}
	cutoff := time.Now().Add(-staleCutoff)
	_, rows, err := selectProbeCandidates(ctx, q, probeScope{source: "dice"}, classifiedProviderKeys(), probeDespiteRegistered, cutoff)
	if err != nil || len(rows) != 1 || rows[0].ID != id {
		t.Fatalf("stale Dice job not selected: rows=%+v err=%v", rows, err)
	}
	c := staleCandidate(rows[0], cutoff)

	// Ingest can see the posting after candidate selection and before the verdict write.
	if _, err := pool.Exec(ctx, `UPDATE job_activity SET last_seen_at = now() WHERE job_id = $1`, id); err != nil {
		t.Fatal(err)
	}
	var s stats
	s.apply(ctx, q, c, liveness.Expired, "http_gone")
	var closed, queued bool
	var strikes int
	if err := pool.QueryRow(ctx, `SELECT j.closed_at IS NOT NULL, a.liveness_strikes,
		EXISTS (SELECT FROM search_delete_outbox WHERE job_id = j.id)
		FROM jobs j JOIN job_activity a ON a.job_id = j.id WHERE j.id = $1`, id).Scan(&closed, &strikes, &queued); err != nil {
		t.Fatal(err)
	}
	if closed || queued || strikes != 1 || s.failed.Load() != 0 {
		t.Fatalf("fresh posting changed or counted as a failure: closed=%v queued=%v strikes=%d failed=%d", closed, queued, strikes, s.failed.Load())
	}
}

//go:build integration

// Integration test for the board-scoped close through its real adapter and a real Postgres:
// the leak this whole change exists to close, exercised end to end rather than argued about.
// boardSweepTargets decides WHICH boards may be swept and is unit-tested; this proves that what
// it names actually retires the right rows and leaves the rest alone.
// Run with: go test -tags=integration ./cmd/ingest/
// Requires Docker (testcontainers spins up a throwaway Postgres with the migrations).
package main

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/platform/db"
)

func TestSweepBoardsClosesACompanyTheRunNeverWrote(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	queries := db.New(pool)

	// seed writes one posting of one board, aged so it sits past the sweep's window.
	seed := func(t *testing.T, board, id, companySlug string, seenAgo time.Duration) int64 {
		t.Helper()
		var jobID int64
		err := pool.QueryRow(ctx, `
			INSERT INTO jobs (source, external_id, url, title, public_slug, company, company_slug,
			                  company_slug_folded, last_seen_at)
			VALUES ('greenhouse', $1, 'https://x.test/'||$1, 'Engineer', $1, $2, $2,
			        replace($2, '-', ''), now() - $3::interval)
			RETURNING id`, board+":"+id, companySlug, seenAgo.String()).Scan(&jobID)
		if err != nil {
			t.Fatalf("seed %s: %v", id, err)
		}
		return jobID
	}

	// The leak, in miniature. "acme-subsidiary" has exactly one posting on the acme board. If
	// that posting stops being listed, the run writes nothing for its slug, the slug never
	// enters the crawled set, and the company-scoped close can never reach the row — not this
	// run, not any future one.
	orphan := seed(t, "acme", "1", "acme-subsidiary", 49*time.Hour)
	// A second company on the same board, equally stale — swept for the same reason.
	alsoStale := seed(t, "acme", "2", "acme", 49*time.Hour)
	// Same board, seen recently: the window still protects it.
	fresh := seed(t, "acme", "3", "acme", 6*time.Hour)
	// A board this run did not crawl: nothing about it was proved, so nothing of it closes.
	uncrawled := seed(t, "globex", "1", "globex", 49*time.Hour)

	closed, _, failures := sweepBoards(ctx, queries, "greenhouse", []string{"acme"},
		pgtype.Timestamptz{Time: time.Now().Add(-48 * time.Hour), Valid: true})
	if failures != 0 {
		t.Fatalf("board sweep reported %d failures, want 0", failures)
	}
	if closed != 2 {
		t.Fatalf("closed %d, want 2", closed)
	}

	for _, tc := range []struct {
		name   string
		id     int64
		closed bool
	}{
		{"the orphan company's only posting — the leak this closes", orphan, true},
		{"another stale posting of the same board", alsoStale, true},
		{"a recently seen posting of the same board", fresh, false},
		{"a stale posting of a board the run did not crawl", uncrawled, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			job, err := queries.GetJob(ctx, tc.id)
			if err != nil {
				t.Fatalf("get: %v", err)
			}
			if job.ClosedAt.Valid != tc.closed {
				t.Errorf("closed = %v, want %v", job.ClosedAt.Valid, tc.closed)
			}
		})
	}
}

func TestSweepBoardsSurvivesOneBadBoard(t *testing.T) {
	// One statement per board is what keeps a failure local. A board whose statement cannot run
	// must cost that board and nothing else — the provider-wide close had the opposite property,
	// and one corrupted row once blocked greenhouse's entire sweep on every run (2026-08-11).
	pool := startPostgres(t)
	ctx := context.Background()
	queries := db.New(pool)

	var good int64
	err := pool.QueryRow(ctx, `
		INSERT INTO jobs (source, external_id, url, title, public_slug, company, company_slug,
		                  company_slug_folded, last_seen_at)
		VALUES ('greenhouse', 'good:1', 'https://x.test/good', 'Engineer', 'good-1', 'Good', 'good',
		        'good', now() - interval '49 hours')
		RETURNING id`).Scan(&good)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	// A cancelled context makes the first board's statement fail the way a real one would,
	// without needing a corrupted row to reproduce.
	dead, cancel := context.WithCancel(ctx)
	cancel()
	if _, _, failures := sweepBoards(dead, queries, "greenhouse", []string{"good"}, pgtype.Timestamptz{
		Time: time.Now().Add(-48 * time.Hour), Valid: true}); failures != 1 {
		t.Fatalf("failures = %d, want 1 — a failed board must be counted, not swallowed", failures)
	}

	// The same sweep on a live context still works: the failure above left no residue.
	closed, _, failures := sweepBoards(ctx, queries, "greenhouse", []string{"good"}, pgtype.Timestamptz{
		Time: time.Now().Add(-48 * time.Hour), Valid: true})
	if failures != 0 || closed != 1 {
		t.Errorf("closed=%d failures=%d, want 1 and 0", closed, failures)
	}
}

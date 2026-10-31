//go:build integration

package db

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestRepairESSJobURL(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	const oldURL = "https://www.ess.gov.si/iskalci-zaposlitve/iskanje-zaposlitve/iskanje-dela/?idp=3493570"
	const fixedURL = "https://www.ess.gov.si/iskalci-zaposlitve/iskanje-zaposlitve/iskanje-dela/?idp=3493570/#/pdm/3493570"
	for _, tt := range []struct {
		name       string
		source     string
		closed     bool
		private    bool
		storedHash pgtype.Text
		oldURL     string
		oldHash    pgtype.Text
		wantUpdate int64
		wantQueue  int64
	}{
		{name: "open", source: "ess", oldURL: oldURL, wantUpdate: 1, wantQueue: 1},
		{name: "closed", source: "ess", closed: true, oldURL: oldURL, wantUpdate: 1},
		{name: "private", source: "ess", private: true, oldURL: oldURL},
		{name: "other source", source: "other", oldURL: oldURL},
		{name: "concurrent url change", source: "ess", oldURL: "https://stale.example"},
		{
			name: "concurrent hash change", source: "ess", oldURL: oldURL,
			storedHash: pgtype.Text{String: "newer-hash", Valid: true},
			oldHash:    pgtype.Text{String: "stale-hash", Valid: true},
		},
		{
			name: "matching hash", source: "ess", oldURL: oldURL, wantUpdate: 1, wantQueue: 1,
			storedHash: pgtype.Text{String: "old-hash", Valid: true},
			oldHash:    pgtype.Text{String: "old-hash", Valid: true},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			truncate(t, pool)
			var id int64
			err := pool.QueryRow(ctx, `
				INSERT INTO jobs (source, external_id, url, title, public_slug, content_hash, closed_at, is_private)
				VALUES ($1, ':3493570', $2, 'Developer', 'stable-published-slug', $3,
				        CASE WHEN $4::boolean THEN now() END, $5)
				RETURNING id`, tt.source, oldURL, tt.storedHash, tt.closed, tt.private).Scan(&id)
			if err != nil {
				t.Fatal(err)
			}
			p := RepairESSJobURLParams{
				ID: id, URL: fixedURL, OldUrl: tt.oldURL, OldContentHash: tt.oldHash,
				ContentHash: pgtype.Text{String: "fixed-hash", Valid: true},
			}
			got, err := q.RepairESSJobURL(ctx, p)
			if err != nil || got.UpdatedCount != tt.wantUpdate || got.EnqueuedCount != tt.wantQueue {
				t.Fatalf("repair = %+v, %v; want updated=%d queued=%d", got, err, tt.wantUpdate, tt.wantQueue)
			}
			var url, slug string
			var hash pgtype.Text
			if err := pool.QueryRow(ctx, "SELECT url, content_hash, public_slug FROM jobs WHERE id = $1", id).
				Scan(&url, &hash, &slug); err != nil {
				t.Fatal(err)
			}
			wantURL, wantHash := oldURL, tt.storedHash
			if tt.wantUpdate == 1 {
				wantURL, wantHash = fixedURL, p.ContentHash
			}
			if url != wantURL || hash != wantHash || slug != "stable-published-slug" {
				t.Fatalf("stored url=%q hash=%+v slug=%q", url, hash, slug)
			}
			var queued int64
			if err := pool.QueryRow(ctx, "SELECT count(*) FROM search_outbox WHERE job_id = $1", id).Scan(&queued); err != nil {
				t.Fatal(err)
			}
			if queued != tt.wantQueue {
				t.Fatalf("queue has %d rows, want %d", queued, tt.wantQueue)
			}
			repeated, err := q.RepairESSJobURL(ctx, p)
			if err != nil || repeated.UpdatedCount != 0 || repeated.EnqueuedCount != 0 {
				t.Fatalf("repeat = %+v, %v; want no update or enqueue", repeated, err)
			}
		})
	}
}

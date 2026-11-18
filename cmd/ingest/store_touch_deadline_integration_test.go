//go:build integration

package main

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/ingest/pipeline"
	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/testdb"
)

type deadlineListing struct{ deadline *time.Time }

func (deadlineListing) Provider() string { return "lever" }
func (s deadlineListing) Fetch(context.Context, sources.CompanyEntry) ([]sources.Job, error) {
	return []sources.Job{{ExternalID: "deadline", Title: "Backend Engineer", Company: "Acme", SeenRefresh: true, ApplicationDeadline: s.deadline}}, nil
}

func TestListingRefreshPersistsDeadlineAndLifecycle(t *testing.T) {
	today := time.Now().UTC().Truncate(24 * time.Hour)
	for _, tc := range []struct {
		name           string
		storedDeadline *time.Time
		wasClosed      bool
		deadline       *time.Time
		want           time.Time
		closed         bool
	}{
		{"shortened", nil, false, new(today.AddDate(0, 0, -1)), today.AddDate(0, 0, -1), true},
		{"extended", new(today.AddDate(0, 0, -1)), true, new(today.AddDate(0, 0, 3)), today.AddDate(0, 0, 3), false},
		{"changed while open", new(today.AddDate(0, 0, 3)), false, new(today.AddDate(0, 0, 5)), today.AddDate(0, 0, 5), false},
		{"missing preserves stored date", new(today.AddDate(0, 0, 5)), false, nil, today.AddDate(0, 0, 5), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pool := testdb.Pool(t)
			ctx := context.Background()
			store := newDBStore(pool, 1, newCrawledSet(), nil, defaultSeenPolicy())
			if err := store.Save(ctx, cheapPosting("acme:deadline", "Backend Engineer")); err != nil {
				t.Fatal(err)
			}
			original := loadJob(t, pool, "acme:deadline")
			if _, err := pool.Exec(ctx, `UPDATE jobs SET application_deadline=$1::date,
                closed_at=CASE WHEN $2::boolean THEN now() END,
                closed_reason=CASE WHEN $2::boolean THEN 'expired' ELSE '' END WHERE id=$3`, tc.storedDeadline, tc.wasClosed, original.ID); err != nil {
				t.Fatal(err)
			}
			if tc.wasClosed {
				// Retain the previous expiry's queued removal: a delayed worker must
				// recognize the reopened posting instead of hiding it again.
				if _, err := pool.Exec(ctx, `INSERT INTO search_delete_outbox(job_id) VALUES($1)`, original.ID); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := pool.Exec(ctx, `DELETE FROM search_outbox WHERE job_id=$1`, original.ID); err != nil {
				t.Fatal(err)
			}
			backdateStamps(t, pool, "acme:deadline")
			runner := pipeline.Runner{Store: store, Registry: map[string]sources.Source{"lever": deadlineListing{tc.deadline}}}
			stats, err := runner.Run(ctx, []sources.CompanyEntry{{Provider: "lever", Board: "acme", Company: "Acme"}})
			if err != nil {
				t.Fatal(err)
			}
			if stats["lever"].Ingested != 1 {
				t.Fatalf("refresh stats = %+v", stats)
			}
			got := loadJob(t, pool, "acme:deadline")
			if !got.ApplicationDeadline.Valid || !got.ApplicationDeadline.Time.Equal(tc.want) {
				t.Fatalf("deadline = %v, want %s", got.ApplicationDeadline, tc.want)
			}
			if got.ClosedAt.Valid != tc.closed {
				t.Errorf("closed = %v, want %v", got.ClosedAt.Valid, tc.closed)
			}
			if tc.closed && got.ClosedReason != "expired" {
				t.Errorf("close reason = %q", got.ClosedReason)
			}
			if got.Description != original.Description || !slices.Equal(got.Skills, original.Skills) {
				t.Error("refresh rewrote hydrated content")
			}
			if tc.deadline != nil && time.Since(got.UpdatedAt.Time) > time.Minute {
				t.Error("deadline change did not stamp updated_at")
			}
			if !tc.closed && tc.deadline != nil && searchOutboxCount(t, pool, original.ID) != 1 {
				t.Error("deadline change was not queued for search")
			}
			if tc.wasClosed {
				rows, err := db.New(pool).PrepareSearchDeletions(ctx, []int64{original.ID})
				if err != nil {
					t.Fatal(err)
				}
				if len(rows) != 0 {
					t.Fatal("a stale expiry removal would hide the reopened posting")
				}
				var pending int
				if err := pool.QueryRow(ctx, `SELECT count(*) FROM search_delete_outbox WHERE job_id=$1`, original.ID).Scan(&pending); err != nil {
					t.Fatal(err)
				}
				if pending != 0 {
					t.Error("obsolete removal was not cancelled by the drain")
				}
				if searchOutboxCount(t, pool, original.ID) != 1 {
					t.Error("reopened posting lost its search restoration")
				}
			}
			if tc.closed {
				var n int
				if err := pool.QueryRow(ctx, `SELECT count(*) FROM search_delete_outbox WHERE job_id=$1`, original.ID).Scan(&n); err != nil {
					t.Fatal(err)
				}
				if n != 1 {
					t.Error("expired posting was not queued for search removal")
				}
			}
		})
	}
}

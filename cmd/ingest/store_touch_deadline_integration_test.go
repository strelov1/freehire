//go:build integration

package main

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/ingest/pipeline"
	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/platform/testdb"
)

type deadlineListing struct{ deadline *time.Time }

func (deadlineListing) Provider() string { return "lever" }
func (s deadlineListing) Fetch(context.Context, sources.CompanyEntry) ([]sources.Job, error) {
	return []sources.Job{{ExternalID: "deadline", Title: "Backend Engineer", Company: "Acme", SeenRefresh: true, ApplicationDeadline: s.deadline}}, nil
}

func TestListingRefreshPersistsDeadlineAndLifecycle(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	store := newDBStore(pool, 1, newCrawledSet(), nil, defaultSeenPolicy())
	if err := store.Save(ctx, cheapPosting("acme:deadline", "Backend Engineer")); err != nil {
		t.Fatal(err)
	}
	original := loadJob(t, pool, "acme:deadline")
	today := time.Now().UTC().Truncate(24 * time.Hour)
	for _, tc := range []struct {
		name     string
		deadline *time.Time
		want     time.Time
		closed   bool
	}{
		{"shortened", new(today.AddDate(0, 0, -1)), today.AddDate(0, 0, -1), true},
		{"extended", new(today.AddDate(0, 0, 3)), today.AddDate(0, 0, 3), false},
		{"changed while open", new(today.AddDate(0, 0, 5)), today.AddDate(0, 0, 5), false},
		{"missing preserves stored date", nil, today.AddDate(0, 0, 5), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := pool.Exec(ctx, `DELETE FROM search_outbox WHERE job_id=$1`, original.ID); err != nil {
				t.Fatal(err)
			}
			if _, err := pool.Exec(ctx, `DELETE FROM search_delete_outbox WHERE job_id=$1`, original.ID); err != nil {
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

//go:build integration

// Integration tests for ListAppliedCompanySlugs — the set the feed's "you already applied
// to this company" badge and /jobs/search's hide_applied_companies read. Run with:
// go test -tags=integration ./internal/platform/db/
package db

import (
	"context"
	"slices"
	"testing"
	"time"
)

func TestListAppliedCompanySlugs(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	user := seedResponseUser(t, q, "applied-co@example.test", false)
	other := seedResponseUser(t, q, "applied-co-other@example.test", false)

	// Applied through a live posting: the posting's slug is the one that counts.
	live := seedResponseJob(t, q, "applied-co-live", "acme")
	if _, err := q.MarkJobApplied(ctx, MarkJobAppliedParams{UserID: user, JobID: live, EventSource: "user"}); err != nil {
		t.Fatalf("MarkJobApplied: %v", err)
	}
	// The slug rule moved after the apply: the posting now says "acme" while the
	// application's own snapshot still says "acme-inc". The posting wins.
	if _, err := pool.Exec(ctx, `UPDATE applications SET company_slug = 'acme-inc' WHERE job_id = $1`, live); err != nil {
		t.Fatalf("age the snapshot: %v", err)
	}

	// A posting deleted since: only the snapshot is left, resolved through the alias
	// registry to the employer it was merged into.
	seedOrphanedApplication(t, pool, user, "dollartree", time.Now().Add(-time.Hour))
	if _, err := pool.Exec(ctx,
		`INSERT INTO company_slug_aliases (alias_slug, canonical_slug, folded_key, reason)
		 VALUES ('dollartree', 'dollar-tree', 'dollartree', 'spelling')`); err != nil {
		t.Fatalf("seed alias: %v", err)
	}

	// Tracked but never applied (the tailor's "preparing" placement): not an application.
	preparing := seedResponseJob(t, q, "applied-co-preparing", "initech")
	if _, err := pool.Exec(ctx,
		`INSERT INTO applications (user_id, job_id, company_slug, stage) VALUES ($1, $2, 'initech', 'preparing')`,
		user, preparing); err != nil {
		t.Fatalf("seed preparing: %v", err)
	}

	// No employer at all: nothing to compare a posting against.
	seedOrphanedApplication(t, pool, user, "", time.Now().Add(-2*time.Hour))

	// Someone else's application never leaks into this caller's set.
	theirs := seedResponseJob(t, q, "applied-co-theirs", "globex")
	if _, err := q.MarkJobApplied(ctx, MarkJobAppliedParams{UserID: other, JobID: theirs, EventSource: "user"}); err != nil {
		t.Fatalf("MarkJobApplied other: %v", err)
	}

	got, err := q.ListAppliedCompanySlugs(ctx, ListAppliedCompanySlugsParams{UserID: user, MaxApplications: 100})
	if err != nil {
		t.Fatalf("ListAppliedCompanySlugs: %v", err)
	}
	want := []string{"acme", "dollar-tree"} // newest application first
	if !slices.Equal(got, want) {
		t.Errorf("slugs = %v, want %v", got, want)
	}

	// The cap bounds the applications read, newest first.
	got, err = q.ListAppliedCompanySlugs(ctx, ListAppliedCompanySlugsParams{UserID: user, MaxApplications: 1})
	if err != nil {
		t.Fatalf("ListAppliedCompanySlugs capped: %v", err)
	}
	if !slices.Equal(got, []string{"acme"}) {
		t.Errorf("capped slugs = %v, want [acme]", got)
	}
}

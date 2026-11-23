//go:build integration

// Integration test for the feed watermark and drain high-water queries.
// Run with: go test -tags=integration ./internal/application/jobtracking/ -run FeedState
package jobtracking_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/testdb"
)

func TestFeedState(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Pool(t)
	q := db.New(pool)
	ts := func(tm time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: tm, Valid: true} }

	var userID int64
	if err := pool.QueryRow(ctx, `INSERT INTO users (email) VALUES ('fs@example.test') RETURNING id`).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	t0 := time.Now().Truncate(time.Second)
	first, err := q.GetOrInitFeedSeen(ctx, db.GetOrInitFeedSeenParams{UserID: userID, Source: "profile", InitAt: ts(t0)})
	if err != nil || !first.Time.Equal(t0) {
		t.Fatalf("first read = %v, %v; want the init instant", first.Time, err)
	}
	again, err := q.GetOrInitFeedSeen(ctx, db.GetOrInitFeedSeenParams{UserID: userID, Source: "profile", InitAt: ts(t0.Add(time.Hour))})
	if err != nil || !again.Time.Equal(t0) {
		t.Fatalf("second read = %v, %v; the stored value must win", again.Time, err)
	}
	advance := func(at time.Time) {
		t.Helper()
		if err := q.AdvanceFeedSeen(ctx, db.AdvanceFeedSeenParams{UserID: userID, Source: "profile", At: ts(at)}); err != nil {
			t.Fatal(err)
		}
	}
	advance(t0.Add(time.Minute))
	advance(t0.Add(-time.Minute)) // an older snapshot never moves it back
	got, _ := q.GetOrInitFeedSeen(ctx, db.GetOrInitFeedSeenParams{UserID: userID, Source: "profile", InitAt: ts(t0)})
	if !got.Time.Equal(t0.Add(time.Minute)) {
		t.Fatalf("watermark = %v, want t0+1m", got.Time)
	}

	hw, err := q.SearchDrainHighWater(ctx)
	if err != nil || hw.Valid {
		t.Fatalf("empty outbox: high-water = %+v, %v; want NULL", hw, err)
	}
	job := func(n string, created time.Time) int64 {
		t.Helper()
		var id int64
		if err := pool.QueryRow(ctx, `
			INSERT INTO jobs (source, external_id, url, title, company, company_slug, public_slug, created_at)
			VALUES ('t', $1, '', 'T', 'Acme', 'acme', $1, $2) RETURNING id`, n, created).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	queue := func(id int64) {
		t.Helper()
		if _, err := pool.Exec(ctx, `INSERT INTO search_outbox (job_id) VALUES ($1)`, id); err != nil {
			t.Fatal(err)
		}
	}
	queue(job("old", time.Now().Add(-48*time.Hour))) // a backfill requeue of an old job cannot pin it
	recent := time.Now().Add(-2 * time.Hour).Truncate(time.Second)
	queue(job("recent", recent))
	queue(job("newer", time.Now().Add(-time.Hour)))
	if hw, err = q.SearchDrainHighWater(ctx); err != nil || !hw.Valid || !hw.Time.Equal(recent) {
		t.Fatalf("high-water = %+v, %v; want the oldest recent pending job %v", hw, err, recent)
	}
}

// TestFeedSource: no row is the profile, the chosen search survives a delete of the search
// (no FK, so the feed can say "deleted"), and deleting the user removes the row.
func TestFeedSource(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Pool(t)
	q := db.New(pool)

	var userID int64
	if err := pool.QueryRow(ctx, `INSERT INTO users (email) VALUES ('fsrc@example.test') RETURNING id`).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	if _, err := q.GetFeedSource(ctx, userID); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("no row: err = %v, want pgx.ErrNoRows", err)
	}
	if err := q.SetFeedSource(ctx, db.SetFeedSourceParams{UserID: userID, SavedSearchID: pgtype.Int8{Int64: 42, Valid: true}}); err != nil {
		t.Fatal(err)
	}
	if got, err := q.GetFeedSource(ctx, userID); err != nil || !got.Valid || got.Int64 != 42 {
		t.Fatalf("source = %+v, %v; want 42", got, err)
	}
	if err := q.SetFeedSource(ctx, db.SetFeedSourceParams{UserID: userID}); err != nil { // back to profile
		t.Fatal(err)
	}
	if got, err := q.GetFeedSource(ctx, userID); err != nil || got.Valid {
		t.Fatalf("source = %+v, %v; want NULL", got, err)
	}
}

// TestFeedSourceSurvivesDeletingTheSearch: the choice must outlive its saved search, so the
// feed can answer "deleted" instead of silently reverting to the profile.
func TestFeedSourceSurvivesDeletingTheSearch(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Pool(t)
	q := db.New(pool)

	var userID int64
	if err := pool.QueryRow(ctx, `INSERT INTO users (email) VALUES ('fsdel@example.test') RETURNING id`).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	row, err := q.CreateSavedSearch(ctx, db.CreateSavedSearchParams{UserID: userID, Name: "Go", Query: "q=go"})
	if err != nil {
		t.Fatal(err)
	}
	if err := q.SetFeedSource(ctx, db.SetFeedSourceParams{UserID: userID, SavedSearchID: pgtype.Int8{Int64: row.ID, Valid: true}}); err != nil {
		t.Fatal(err)
	}
	if n, err := q.DeleteSavedSearch(ctx, db.DeleteSavedSearchParams{ID: row.ID, UserID: userID}); err != nil || n != 1 {
		t.Fatalf("delete = %d, %v", n, err)
	}
	if got, err := q.GetFeedSource(ctx, userID); err != nil || !got.Valid || got.Int64 != row.ID {
		t.Fatalf("source after delete = %+v, %v; want the deleted id kept", got, err)
	}
	if _, err := q.GetSavedSearch(ctx, db.GetSavedSearchParams{ID: row.ID, UserID: userID}); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("search still readable: %v", err)
	}
}

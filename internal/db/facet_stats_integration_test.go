//go:build integration

// Integration tests for the facet-distribution snapshot (insights_facet_stats). The
// cmd/rollup-facets worker computes the rows from Meilisearch and swaps the table via
// DeleteAllFacetStats + InsertFacetStat inside one transaction; those transactional
// semantics (atomic replace, isolation from concurrent readers, idempotent rerun) are
// SQL behavior only verifiable against a real Postgres. Run with:
// go test -tags=integration ./internal/db/
package db

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func truncateFacetStats(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), "TRUNCATE insights_facet_stats"); err != nil {
		t.Fatalf("truncate: %v", err)
	}
}

// snapshotMap reads the whole snapshot into facet → value → count for assertions.
func snapshotMap(t *testing.T, q *Queries) map[string]map[string]int64 {
	t.Helper()
	rows, err := q.ListFacetStats(context.Background())
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	out := map[string]map[string]int64{}
	for _, r := range rows {
		if out[r.Facet] == nil {
			out[r.Facet] = map[string]int64{}
		}
		out[r.Facet][r.Value] = r.Count
	}
	return out
}

// writeSnapshot performs the worker's atomic swap for a given set of rows: clear then
// reinsert inside one transaction.
func writeSnapshot(t *testing.T, ctx context.Context, pool *pgxpool.Pool, rows []InsertFacetStatParams) {
	t.Helper()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer tx.Rollback(ctx)
	q := New(pool).WithTx(tx)
	if err := q.DeleteAllFacetStats(ctx); err != nil {
		t.Fatalf("delete: %v", err)
	}
	for _, r := range rows {
		if err := q.InsertFacetStat(ctx, r); err != nil {
			t.Fatalf("insert %s/%s: %v", r.Facet, r.Value, err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}
}

func TestFacetStatsSnapshot(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	first := []InsertFacetStatParams{
		{Facet: "countries", Value: "us", Count: 10},
		{Facet: "countries", Value: "de", Count: 4},
		{Facet: "skills", Value: "go", Count: 7},
		{Facet: "work_mode", Value: "remote", Count: 8},
	}

	t.Run("list returns the whole snapshot ordered by count desc within a facet", func(t *testing.T) {
		truncateFacetStats(t, pool)
		writeSnapshot(t, ctx, pool, first)

		rows, err := q.ListFacetStats(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != len(first) {
			t.Fatalf("got %d rows, want %d", len(rows), len(first))
		}
		// countries appears before work_mode (facet asc), and within countries us (10)
		// precedes de (4) (count desc).
		var countryOrder []string
		for _, r := range rows {
			if r.Facet == "countries" {
				countryOrder = append(countryOrder, r.Value)
			}
		}
		if len(countryOrder) != 2 || countryOrder[0] != "us" || countryOrder[1] != "de" {
			t.Errorf("countries not ordered by count desc: %v", countryOrder)
		}
	})

	t.Run("recompute fully replaces the prior snapshot", func(t *testing.T) {
		truncateFacetStats(t, pool)
		writeSnapshot(t, ctx, pool, first)

		second := []InsertFacetStatParams{
			{Facet: "countries", Value: "us", Count: 12},    // changed count
			{Facet: "seniority", Value: "senior", Count: 5}, // new facet
			// "de", "skills:go", "work_mode:remote" are gone from this run.
		}
		writeSnapshot(t, ctx, pool, second)

		got := snapshotMap(t, q)
		if got["countries"]["us"] != 12 {
			t.Errorf("us count not updated: %v", got)
		}
		if _, ok := got["countries"]["de"]; ok {
			t.Errorf("stale value survived replace: %v", got)
		}
		if _, ok := got["skills"]; ok {
			t.Errorf("stale facet survived replace: %v", got)
		}
		if got["seniority"]["senior"] != 5 {
			t.Errorf("new facet missing after replace: %v", got)
		}
	})

	t.Run("a concurrent reader sees the prior snapshot until commit", func(t *testing.T) {
		truncateFacetStats(t, pool)
		writeSnapshot(t, ctx, pool, first)

		// Open the swap transaction but do NOT commit yet.
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		txq := New(pool).WithTx(tx)
		if err := txq.DeleteAllFacetStats(ctx); err != nil {
			t.Fatal(err)
		}
		if err := txq.InsertFacetStat(ctx, InsertFacetStatParams{Facet: "countries", Value: "fr", Count: 99}); err != nil {
			t.Fatal(err)
		}

		// A reader on a separate connection still sees the committed first snapshot.
		mid := snapshotMap(t, q)
		if mid["countries"]["us"] != 10 || len(mid["countries"]) != 2 {
			t.Errorf("reader saw partial rebuild before commit: %v", mid)
		}

		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		after := snapshotMap(t, q)
		if after["countries"]["fr"] != 99 || len(after["countries"]) != 1 {
			t.Errorf("reader did not see the new snapshot after commit: %v", after)
		}
	})

	t.Run("rerunning with unchanged input is idempotent", func(t *testing.T) {
		truncateFacetStats(t, pool)
		writeSnapshot(t, ctx, pool, first)
		once := snapshotMap(t, q)
		writeSnapshot(t, ctx, pool, first)
		twice := snapshotMap(t, q)

		if len(once) != len(twice) {
			t.Fatalf("facet count changed on rerun: %v vs %v", once, twice)
		}
		for facet, dist := range once {
			for value, count := range dist {
				if twice[facet][value] != count {
					t.Errorf("rerun changed %s/%s: %d vs %d", facet, value, count, twice[facet][value])
				}
			}
		}
	})
}

//go:build integration

// Integration test for ListJobsToPing's gate and tiers. The gate mirrors what the search
// index admits and the tier split is a SQL predicate, so only a real Postgres can say
// which rows each tier returns.
// Run with: go test -tags=integration ./internal/platform/db/
package db

import (
	"context"
	"slices"
	"testing"
)

func TestListJobsToPingTiersPartitionTheIndexedCatalogue(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	// id order is creation order; each fixture's shape is set after the upsert so the
	// test states it directly rather than through the pipeline's derivation.
	fixtures := []struct {
		externalID string
		set        string // SQL applied to the row after insert
	}{
		{"tech", "is_tech = true, category = 'backend'"},
		{"tech-uncategorised", "is_tech = true, category = ''"},
		{"nurse", "is_tech = false, category = 'healthcare'"},
		{"unknown-tech-signal", "is_tech = NULL, category = 'retail'"},
		{"enrichment-only", `is_tech = false, category = '', enrichment = '{"category":"logistics"}'`},
		// Uncategorized: indexed under "other" (#4412), so eligible in the non-technical tier.
		{"unresolved", "is_tech = false, category = ''"},
		{"enrichment-other", `is_tech = NULL, category = '', enrichment = '{"category":"other"}'`},
		// Excluded from both tiers: what search.DescriptionMissing keeps out of the index.
		{"no-body", "is_tech = false, category = 'retail', description = ''"},
		{"private", "is_tech = true, category = 'backend', is_private = true"},
		{"closed", "is_tech = false, category = 'retail', closed_at = now()"},
	}
	ids := map[string]int64{}
	for _, f := range fixtures {
		job, err := ingestUpsert(ctx, q, ingestParams(f.externalID, "Role "+f.externalID))
		if err != nil {
			t.Fatalf("upsert %s: %v", f.externalID, err)
		}
		if _, err := pool.Exec(ctx, "UPDATE jobs SET "+f.set+" WHERE id = $1", job.ID); err != nil {
			t.Fatalf("shape %s: %v", f.externalID, err)
		}
		ids[f.externalID] = job.ID
	}

	tier := func(technical bool) []string {
		t.Helper()
		rows, err := q.ListJobsToPing(ctx, ListJobsToPingParams{Technical: technical, Engine: "google", BatchSize: 100})
		if err != nil {
			t.Fatalf("list technical=%v: %v", technical, err)
		}
		var got []string
		for _, r := range rows {
			for name, id := range ids {
				if id == r.ID {
					got = append(got, name)
				}
			}
		}
		slices.Sort(got)
		return got
	}

	if got, want := tier(true), []string{"tech", "tech-uncategorised"}; !slices.Equal(got, want) {
		t.Errorf("technical tier = %v, want %v", got, want)
	}
	if got, want := tier(false), []string{"enrichment-only", "enrichment-other", "nurse", "unknown-tech-signal", "unresolved"}; !slices.Equal(got, want) {
		t.Errorf("non-technical tier = %v, want %v", got, want)
	}

	// The ledger is per engine and shared by the tiers: a posting announced once is
	// never offered again, whichever tier it was drawn from.
	if err := q.RecordJobSearchPing(ctx, RecordJobSearchPingParams{JobID: ids["nurse"], Engine: "google", Kind: "created"}); err != nil {
		t.Fatalf("record: %v", err)
	}
	if got, want := tier(false), []string{"enrichment-only", "enrichment-other", "unknown-tech-signal", "unresolved"}; !slices.Equal(got, want) {
		t.Errorf("non-technical tier after recording nurse = %v, want %v", got, want)
	}
}

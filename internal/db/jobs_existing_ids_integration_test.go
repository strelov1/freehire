//go:build integration

// Integration test for the hydrating-source seen-set query: ExistingExternalIDs returns the
// external_ids stored for one provider (and only that provider), so a hydrating adapter fetches
// per-posting detail only for postings the catalogue lacks. A SQL behavior, verifiable only
// against a real Postgres. Run with: go test -tags=integration ./internal/db/
package db

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/sources"
)

// farFuture is a hydration cutoff every row predates, which reduces the seen-set to its
// pre-hydration-retry behaviour: everything stored counts as seen.
func farFuture() pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: time.Now().Add(24 * time.Hour), Valid: true}
}

func TestExistingExternalIDsScopedToProvider(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	// Two greenhouse rows and one lever row.
	for _, ext := range []string{"acme:1", "acme:2"} {
		if _, err := ingestUpsert(ctx, q, ingestParams(ext, "Engineer")); err != nil {
			t.Fatalf("upsert %s: %v", ext, err)
		}
	}
	lever := ingestParams("other:9", "Designer")
	lever.Source = "lever"
	if _, err := ingestUpsert(ctx, q, lever); err != nil {
		t.Fatalf("upsert lever: %v", err)
	}

	rows, err := q.ExistingExternalIDs(ctx, ExistingExternalIDsParams{
		Source:          "greenhouse",
		HydrationCutoff: farFuture(),
	})
	if err != nil {
		t.Fatalf("ExistingExternalIDs: %v", err)
	}
	ids := make([]string, len(rows))
	for i, r := range rows {
		ids[i] = r.ExternalID
	}
	slices.Sort(ids)
	if !slices.Equal(ids, []string{"acme:1", "acme:2"}) {
		t.Errorf("ids = %v, want [acme:1 acme:2] (lever row excluded)", ids)
	}
}

// A multi-board provider loads the seen-set once per board, so the query must read one board's
// namespace rather than the provider's whole catalogue. Two hazards are covered: a sibling board
// whose name extends this one's, and an underscore in a board name — LIKE reads it as "any one
// character", which is why the caller passes an escaped pattern.
func TestExistingExternalIDsScopedToBoard(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	boards := map[string][]string{
		"dollartreeus": {"1", "2"}, // the board under test
		"dollartreeca": {"3"},      // a sibling whose name extends "dollartree"
		"Capital_One":  {"4"},      // the underscore hazard
		"CapitalXOne":  {"5"},      // matches "Capital_One:%" unless the underscore is escaped
	}
	for board, ids := range boards {
		for _, id := range ids {
			p := ingestParams(sources.NamespaceExternalID(board, id), "Engineer")
			p.Source = "workday"
			// The refresh path judges the catalogue filter on the row's stored evidence, so
			// the seen-set carries it: mark the first posting of each board technical.
			p.IsTech = pgtype.Bool{Bool: id == ids[0], Valid: true}
			if _, err := ingestUpsert(ctx, q, p); err != nil {
				t.Fatalf("upsert %s: %v", p.ExternalID, err)
			}
		}
	}

	for _, tc := range []struct {
		board    string
		want     []string
		wantTech string // the id whose row carries tech evidence
	}{
		{"dollartreeus", []string{"dollartreeus:1", "dollartreeus:2"}, "dollartreeus:1"},
		{"dollartreeca", []string{"dollartreeca:3"}, "dollartreeca:3"},
		{"Capital_One", []string{"Capital_One:4"}, "Capital_One:4"},
	} {
		rows, err := q.ExistingExternalIDsByBoard(ctx, ExistingExternalIDsByBoardParams{
			Source:          "workday",
			Pattern:         sources.BoardIDPattern(tc.board),
			HydrationCutoff: farFuture(),
		})
		if err != nil {
			t.Fatalf("ExistingExternalIDsByBoard(%s): %v", tc.board, err)
		}
		ids := make([]string, len(rows))
		for i, r := range rows {
			ids[i] = r.ExternalID
			if wantTech := r.ExternalID == tc.wantTech; r.IsTech.Bool != wantTech {
				t.Errorf("board %s: %s is_tech = %v, want %v", tc.board, r.ExternalID, r.IsTech.Bool, wantTech)
			}
		}
		slices.Sort(ids)
		if !slices.Equal(ids, tc.want) {
			t.Errorf("board %s: ids = %v, want %v", tc.board, ids, tc.want)
		}
	}
}

// A row stored without a description is half-ingested: its detail fetch failed, and because
// being stored is what marks a posting seen, nothing would ever retry it. It is withheld from
// the seen-set while it is younger than the cutoff (so the crawl hydrates it as if new) and
// counts as seen once it is older (so a source that publishes no body stops costing a detail
// request every crawl). freehire#1866.
func TestExistingExternalIDsWithholdsUnhydratedRowsUntilCutoff(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	withBody := ingestParams("acme:hydrated", "Engineer")
	empty := ingestParams("acme:empty", "Engineer")
	empty.Description = ""
	for _, p := range []UpsertJobParams{withBody, empty} {
		if _, err := ingestUpsert(ctx, q, p); err != nil {
			t.Fatalf("upsert %s: %v", p.ExternalID, err)
		}
	}

	// Cutoff in the past: both rows are newer, so the body-less one is withheld for retry.
	rows, err := q.ExistingExternalIDs(ctx, ExistingExternalIDsParams{
		Source:          "greenhouse",
		HydrationCutoff: pgtype.Timestamptz{Time: time.Now().Add(-time.Hour), Valid: true},
	})
	if err != nil {
		t.Fatalf("ExistingExternalIDs (fresh): %v", err)
	}
	ids := seenIDs(rows)
	if !slices.Equal(ids, []string{"acme:hydrated"}) {
		t.Errorf("within the window: ids = %v, want [acme:hydrated] (body-less row withheld)", ids)
	}

	// Cutoff in the future: the body-less row has outlived its retry window and is seen again.
	rows, err = q.ExistingExternalIDs(ctx, ExistingExternalIDsParams{
		Source:          "greenhouse",
		HydrationCutoff: farFuture(),
	})
	if err != nil {
		t.Fatalf("ExistingExternalIDs (aged out): %v", err)
	}
	ids = seenIDs(rows)
	if !slices.Equal(ids, []string{"acme:empty", "acme:hydrated"}) {
		t.Errorf("past the window: ids = %v, want both (retry given up)", ids)
	}
}

// The board-scoped seen-set withholds a body-less row on the same rule as the provider-wide one.
func TestExistingExternalIDsByBoardWithholdsUnhydratedRows(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	for _, ext := range []string{"acme:hydrated", "acme:empty"} {
		p := ingestParams(ext, "Engineer")
		p.Source = "workday"
		if ext == "acme:empty" {
			p.Description = ""
		}
		if _, err := ingestUpsert(ctx, q, p); err != nil {
			t.Fatalf("upsert %s: %v", ext, err)
		}
	}

	rows, err := q.ExistingExternalIDsByBoard(ctx, ExistingExternalIDsByBoardParams{
		Source:          "workday",
		Pattern:         sources.BoardIDPattern("acme"),
		HydrationCutoff: pgtype.Timestamptz{Time: time.Now().Add(-time.Hour), Valid: true},
	})
	if err != nil {
		t.Fatalf("ExistingExternalIDsByBoard: %v", err)
	}
	if ids := seenBoardIDs(rows); !slices.Equal(ids, []string{"acme:hydrated"}) {
		t.Errorf("ids = %v, want [acme:hydrated] (body-less row withheld)", ids)
	}
}

// seenIDs collects a provider-wide seen-set result's external_ids, sorted for comparison.
func seenIDs(rows []ExistingExternalIDsRow) []string {
	ids := make([]string, len(rows))
	for i, r := range rows {
		ids[i] = r.ExternalID
	}
	slices.Sort(ids)
	return ids
}

// seenBoardIDs is seenIDs for the board-scoped result, which sqlc gives its own row type.
func seenBoardIDs(rows []ExistingExternalIDsByBoardRow) []string {
	ids := make([]string, len(rows))
	for i, r := range rows {
		ids[i] = r.ExternalID
	}
	slices.Sort(ids)
	return ids
}

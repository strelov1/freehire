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

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/sources"
)

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

	rows, err := q.ExistingExternalIDs(ctx, "greenhouse")
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
			Source:  "workday",
			Pattern: sources.BoardIDPattern(tc.board),
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

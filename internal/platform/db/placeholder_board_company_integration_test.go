//go:build integration

// Integration tests for the queries behind cmd/backfill-company-names' placeholder pass: a
// board whose company is its own board id, and the stored jobs that carry that id as their
// employer's name.
// Run with: go test -tags=integration ./internal/platform/db/
package db

import (
	"context"
	"testing"
)

// TestListPlaceholderCompanyBoards pins what counts as a placeholder: the company equal to the
// board id (case-insensitively), on a live board, of a provider the caller can resolve.
func TestListPlaceholderCompanyBoards(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	q := New(pool)

	seed := func(provider, board, company, status string) {
		t.Helper()
		if _, err := q.InsertBoard(ctx, InsertBoardParams{
			Provider: provider, Board: board, Company: company,
			Tenants: []byte("{}"), Status: status, Surface: "curator",
		}); err != nil {
			t.Fatalf("seed %s/%s: %v", provider, board, err)
		}
	}
	seed("oracle", "eabw.fa.us2.oraclecloud.com/CX", "eabw.fa.us2.oraclecloud.com/CX", "active")
	seed("oracle", "egmn.fa.us2.oraclecloud.com/CX", "EGMN.fa.us2.oraclecloud.com/cx", "pending")
	seed("oracle", "edva.fa.us2.oraclecloud.com/CX", "Archrock", "active")                        // already named
	seed("oracle", "ebtw.fa.us2.oraclecloud.com/CX", "ebtw.fa.us2.oraclecloud.com/CX", "retired") // not live
	seed("taleo", "mch.taleo.net/2", "mch.taleo.net/2", "active")                                 // provider not asked for

	rows, err := q.ListPlaceholderCompanyBoards(ctx, []string{"oracle", "paylocity"})
	if err != nil {
		t.Fatalf("ListPlaceholderCompanyBoards: %v", err)
	}
	got := map[string]bool{}
	for _, r := range rows {
		got[r.Board] = true
	}
	if len(rows) != 2 || !got["eabw.fa.us2.oraclecloud.com/CX"] || !got["egmn.fa.us2.oraclecloud.com/CX"] {
		t.Fatalf("rows = %+v, want the two live oracle placeholders", rows)
	}
}

// TestRenamePlaceholderBoardJobs pins the reach of the jobs half: one board's namespace of one
// source, only rows still carrying the placeholder, the folded slug kept in step, and the
// similar-jobs list invalidated the way UpsertJob invalidates it on a company change.
func TestRenamePlaceholderBoardJobs(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	q := New(pool)
	truncate(t, pool)

	const board = "secure.saashr.com/oma"
	seed := func(source, externalID, company string) int64 {
		t.Helper()
		p := ingestParams(externalID, "Curator")
		p.Source, p.Company, p.CompanySlug = source, company, "secure-saashr-com-oma"
		job, err := ingestUpsert(ctx, q, p)
		if err != nil {
			t.Fatalf("seed %s: %v", externalID, err)
		}
		if _, err := pool.Exec(ctx, "UPDATE jobs SET similar_computed_at = now() WHERE id = $1", job.ID); err != nil {
			t.Fatalf("stamp %s: %v", externalID, err)
		}
		return job.ID
	}
	target := seed("ukgready", board+":1", board)
	otherBoard := seed("ukgready", "secure.saashr.com/omax:2", board) // a prefix of the board, not it
	otherSource := seed("paylocity", board+":3", board)
	renamed := seed("ukgready", board+":4", "Someone Else")

	n, err := q.RenamePlaceholderBoardJobs(ctx, RenamePlaceholderBoardJobsParams{
		Name: "Oceanside Museum Of Art", NewSlug: "oceanside-museum-of-art",
		Source: "ukgready", BoardPattern: board + ":%", OldName: board,
	})
	if err != nil {
		t.Fatalf("RenamePlaceholderBoardJobs: %v", err)
	}
	if n != 1 {
		t.Errorf("rows = %d, want 1", n)
	}

	for id, want := range map[int64]string{
		target:      "Oceanside Museum Of Art|oceanside-museum-of-art|oceansidemuseumofart|true",
		otherBoard:  board + "|secure-saashr-com-oma|securesaashrcomoma|false",
		otherSource: board + "|secure-saashr-com-oma|securesaashrcomoma|false",
		renamed:     "Someone Else|secure-saashr-com-oma|securesaashrcomoma|false",
	} {
		var got string
		if err := pool.QueryRow(ctx,
			"SELECT company || '|' || company_slug || '|' || company_slug_folded || '|' || (similar_computed_at IS NULL)::text FROM jobs WHERE id = $1",
			id).Scan(&got); err != nil {
			t.Fatalf("read job %d: %v", id, err)
		}
		if got != want {
			t.Errorf("job %d = %q, want %q", id, got, want)
		}
	}
}

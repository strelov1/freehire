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
// board id (case-insensitively), on a live board, of a provider the caller can resolve; a
// SuccessFactors hub is excluded because its host page names the hub rather than its tenants.
func TestListPlaceholderCompanyBoards(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	q := New(pool)

	seed := func(provider, board, company, status string, hub bool) {
		t.Helper()
		if _, err := q.InsertBoard(ctx, InsertBoardParams{
			Provider: provider, Board: board, Company: company, Hub: hub,
			Tenants: []byte("{}"), Status: status, Surface: "curator",
		}); err != nil {
			t.Fatalf("seed %s/%s: %v", provider, board, err)
		}
	}
	seed("oracle", "eabw.fa.us2.oraclecloud.com/CX", "eabw.fa.us2.oraclecloud.com/CX", "active", false)
	seed("oracle", "egmn.fa.us2.oraclecloud.com/CX", "EGMN.fa.us2.oraclecloud.com/cx", "pending", false)
	seed("oracle", "edva.fa.us2.oraclecloud.com/CX", "Archrock", "active", false)                        // already named
	seed("oracle", "ebtw.fa.us2.oraclecloud.com/CX", "ebtw.fa.us2.oraclecloud.com/CX", "retired", false) // not live
	seed("taleo", "mch.taleo.net/2", "mch.taleo.net/2", "active", false)                                 // provider not asked for
	seed("successfactors", "jobs.volvogroup.com", "jobs.volvogroup.com", "active", false)
	seed("successfactors", "jobs.example.com", "Example Group", "active", false)           // curator already named it
	seed("successfactors", "jobs.hub.example.com", "jobs.hub.example.com", "active", true) // hub page names its parent, not its tenants
	seed("teamtailor", "jobs.bookbeat.com", "jobs.bookbeat.com", "pending", false)

	providers := []string{"oracle", "paylocity", "successfactors", "teamtailor"}
	rows, err := q.ListPlaceholderCompanyBoards(ctx, providers)
	if err != nil {
		t.Fatalf("ListPlaceholderCompanyBoards: %v", err)
	}
	got := map[string]bool{}
	for _, r := range rows {
		got[r.Board] = true
	}
	want := []string{
		"eabw.fa.us2.oraclecloud.com/CX",
		"egmn.fa.us2.oraclecloud.com/CX",
		"jobs.volvogroup.com",
		"jobs.bookbeat.com",
	}
	if len(rows) != len(want) {
		t.Fatalf("got %d placeholder boards: %+v; want %d", len(rows), rows, len(want))
	}
	for _, board := range want {
		if !got[board] {
			t.Errorf("missing placeholder board %q in rows: %+v", board, rows)
		}
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
		return job.ID
	}
	target := seed("ukgready", board+":1", board)
	otherBoard := seed("ukgready", "secure.saashr.com/omax:2", board) // a prefix of the board, not it
	otherSource := seed("paylocity", board+":3", board)
	renamed := seed("ukgready", board+":4", "Someone Else")

	oldSlugs, err := q.RenamePlaceholderBoardJobs(ctx, RenamePlaceholderBoardJobsParams{
		Name: "Oceanside Museum Of Art", NewSlug: "oceanside-museum-of-art",
		Source: "ukgready", BoardPattern: board + ":%", OldName: board,
	})
	if err != nil {
		t.Fatalf("RenamePlaceholderBoardJobs: %v", err)
	}
	// The slug the moved row carried BEFORE the write, which the caller sweeps for orphans.
	if len(oldSlugs) != 1 || oldSlugs[0] != "secure-saashr-com-oma" {
		t.Errorf("old slugs = %q, want [secure-saashr-com-oma]", oldSlugs)
	}

	for id, want := range map[int64]string{
		target:      "Oceanside Museum Of Art|oceanside-museum-of-art|oceansidemuseumofart",
		otherBoard:  board + "|secure-saashr-com-oma|securesaashrcomoma",
		otherSource: board + "|secure-saashr-com-oma|securesaashrcomoma",
		renamed:     "Someone Else|secure-saashr-com-oma|securesaashrcomoma",
	} {
		var got string
		if err := pool.QueryRow(ctx,
			"SELECT company || '|' || company_slug || '|' || company_slug_folded FROM jobs WHERE id = $1",
			id).Scan(&got); err != nil {
			t.Fatalf("read job %d: %v", id, err)
		}
		if got != want {
			t.Errorf("job %d = %q, want %q", id, got, want)
		}
	}
}

// TestCompanySlugAliasAcceptsBoardName pins migration 0195: the placeholder pass records the
// retired slug under its own reason, and the 301 lookup finds it like any other alias.
func TestCompanySlugAliasAcceptsBoardName(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	q := New(pool)

	if _, err := q.InsertCompanySlugAlias(ctx, InsertCompanySlugAliasParams{
		AliasSlug: "fa-etjg-saasfaprod1-fa-ocs-oraclecloud-com-maggianos", CanonicalSlug: "maggianos",
		FoldedKey: "faetjgsaasfaprod1faocsoraclecloudcommaggianos", Reason: "board_name",
	}); err != nil {
		t.Fatalf("InsertCompanySlugAlias(board_name): %v", err)
	}
	got, err := q.GetCompanySlugAlias(ctx, "fa-etjg-saasfaprod1-fa-ocs-oraclecloud-com-maggianos")
	if err != nil || got != "maggianos" {
		t.Fatalf("GetCompanySlugAlias = %q, %v; want maggianos", got, err)
	}
	if _, err := q.InsertCompanySlugAlias(ctx, InsertCompanySlugAliasParams{
		AliasSlug: "a", CanonicalSlug: "b", FoldedKey: "a", Reason: "guess",
	}); err == nil {
		t.Fatal("InsertCompanySlugAlias(guess): want the reason check to refuse it, got nil")
	}
}

// TestRenamePlaceholderBoardJobsQueuesSearch pins that a rename reaches search: the moved row's
// indexed company and slug change, so an open, canonical, public posting is queued in the same
// statement, while a closed or private one (never indexed) is not.
func TestRenamePlaceholderBoardJobsQueuesSearch(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	q := New(pool)
	truncate(t, pool)

	const board = "8a7883d074274b7d01744b8825240e7f"
	seed := func(externalID string) int64 {
		t.Helper()
		p := ingestParams(externalID, "Engineer")
		p.Source, p.Company, p.CompanySlug = "paycor", board, board
		job, err := ingestUpsert(ctx, q, p)
		if err != nil {
			t.Fatalf("seed %s: %v", externalID, err)
		}
		return job.ID
	}
	open := seed(board + ":1")
	closed := seed(board + ":2")
	private := seed(board + ":3")
	if _, err := pool.Exec(ctx, "UPDATE jobs SET closed_at = now() WHERE id = $1", closed); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "UPDATE jobs SET is_private = true WHERE id = $1", private); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "DELETE FROM search_outbox"); err != nil {
		t.Fatal(err)
	}

	if _, err := q.RenamePlaceholderBoardJobs(ctx, RenamePlaceholderBoardJobsParams{
		Name: "Salvagnini America, Inc.", NewSlug: "salvagnini-america-inc",
		Source: "paycor", BoardPattern: board + ":%", OldName: board,
	}); err != nil {
		t.Fatalf("RenamePlaceholderBoardJobs: %v", err)
	}

	rows, err := pool.Query(ctx, "SELECT job_id FROM search_outbox ORDER BY job_id")
	if err != nil {
		t.Fatal(err)
	}
	var queued []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		queued = append(queued, id)
	}
	if len(queued) != 1 || queued[0] != open {
		t.Errorf("queued = %v, want only the open posting %d", queued, open)
	}
}

// TestRenamePlaceholderBoardJobsSurvivesAnInFlightClaim pins that a rename landing while the
// drain holds the job's entry is not lost: completing the old claim (deleting it by id) must
// leave the rename's own entry pending, as RequeueSearchOutbox guarantees for other rewrites.
func TestRenamePlaceholderBoardJobsSurvivesAnInFlightClaim(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	q := New(pool)
	truncate(t, pool)

	const board = "8a7883d074274b7d01744b8825240e7f"
	p := ingestParams(board+":1", "Engineer")
	p.Source, p.Company, p.CompanySlug = "paycor", board, board
	job, err := ingestUpsert(ctx, q, p)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := pool.Exec(ctx, "DELETE FROM search_outbox"); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "INSERT INTO search_outbox (job_id, job_posted_at) VALUES ($1, now())", job.ID); err != nil {
		t.Fatal(err)
	}
	claimed, err := q.ClaimSearchOutboxBatch(ctx, ClaimSearchOutboxBatchParams{LeaseSeconds: 3600, BatchSize: 10})
	if err != nil || len(claimed) != 1 {
		t.Fatalf("claim: %v, %v", claimed, err)
	}

	if _, err := q.RenamePlaceholderBoardJobs(ctx, RenamePlaceholderBoardJobsParams{
		Name: "Salvagnini America, Inc.", NewSlug: "salvagnini-america-inc",
		Source: "paycor", BoardPattern: board + ":%", OldName: board,
	}); err != nil {
		t.Fatalf("RenamePlaceholderBoardJobs: %v", err)
	}
	if err := q.DeleteSearchOutboxEntries(ctx, []int64{claimed[0].ID}); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM search_outbox WHERE job_id = $1", job.ID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("pending entries for the renamed job after the old claim completed = %d, want 1", n)
	}
}

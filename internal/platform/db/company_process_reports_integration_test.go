//go:build integration

// Integration tests for the company_process_reports SQL (migration 0156). Every
// behaviour here is a property of the CONSTRAINTS and the ON CONFLICT branch, not of
// Go: that a second live report returns no row, that withdrawal keeps the row, that
// re-filing revives the same row instead of inserting a second, and that the
// materialized counter equals the un-retracted rows after each. Only a real Postgres
// proves any of it. Run with:
//
//	go test -tags=integration ./internal/platform/db/
package db

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

func seedProcessReportUser(t *testing.T, pool *pgxpool.Pool, email string) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO users (email) VALUES ($1) RETURNING id`, email).Scan(&id); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	return id
}

func seedProcessReportCompany(t *testing.T, pool *pgxpool.Pool, slug string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO companies (slug, name) VALUES ($1, $1)`, slug); err != nil {
		t.Fatalf("insert company: %v", err)
	}
}

// liveRowCount is every row for the pair, retracted or not — what proves a revival
// reused the row rather than adding one beside it.
func processReportRowCount(t *testing.T, pool *pgxpool.Pool, userID int64, slug string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM company_process_reports WHERE user_id = $1 AND company_slug = $2`,
		userID, slug).Scan(&n); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	return n
}

func TestFileCompanyProcessReportIsIdempotentPerUser(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	seedProcessReportCompany(t, pool, "acme-ai")
	user := seedProcessReportUser(t, pool, "filer@example.com")
	arg := FileCompanyProcessReportParams{UserID: user, CompanySlug: "acme-ai", Kind: "ai_interview"}

	first, err := q.FileCompanyProcessReport(ctx, arg)
	if err != nil {
		t.Fatalf("first file: %v", err)
	}
	if first == 0 {
		t.Fatal("first file returned no id")
	}

	// The 409 signal: an already-live report updates nothing, so the ON CONFLICT
	// branch's WHERE excludes it and RETURNING yields no row.
	if _, err := q.FileCompanyProcessReport(ctx, arg); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("second file: want pgx.ErrNoRows, got %v", err)
	}

	if n := processReportRowCount(t, pool, user, "acme-ai"); n != 1 {
		t.Fatalf("rows after duplicate file = %d, want 1", n)
	}

	count, err := q.RecountCompanyProcessReports(ctx, "acme-ai")
	if err != nil {
		t.Fatalf("recount: %v", err)
	}
	if count != 1 {
		t.Fatalf("counter = %d, want 1", count)
	}
}

func TestRetractCompanyProcessReportKeepsTheRow(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	seedProcessReportCompany(t, pool, "retracto")
	user := seedProcessReportUser(t, pool, "retractor@example.com")
	fileArg := FileCompanyProcessReportParams{UserID: user, CompanySlug: "retracto", Kind: "ai_interview"}
	retractArg := RetractCompanyProcessReportParams{UserID: user, CompanySlug: "retracto", Kind: "ai_interview"}

	filed, err := q.FileCompanyProcessReport(ctx, fileArg)
	if err != nil {
		t.Fatalf("file: %v", err)
	}

	retracted, err := q.RetractCompanyProcessReport(ctx, retractArg)
	if err != nil {
		t.Fatalf("retract: %v", err)
	}
	if retracted != filed {
		t.Fatalf("retracted id = %d, want the filed id %d", retracted, filed)
	}

	// Retraction is not deletion: the row must survive, or the uniqueness bound stops
	// holding and a retraction becomes a way to file repeatedly.
	if n := processReportRowCount(t, pool, user, "retracto"); n != 1 {
		t.Fatalf("rows after retraction = %d, want the row kept", n)
	}

	count, err := q.RecountCompanyProcessReports(ctx, "retracto")
	if err != nil {
		t.Fatalf("recount: %v", err)
	}
	if count != 0 {
		t.Fatalf("counter after retraction = %d, want 0", count)
	}

	// A second withdrawal has nothing to withdraw, and must say so rather than
	// silently restamping the timestamp.
	if _, err := q.RetractCompanyProcessReport(ctx, retractArg); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("second retract: want pgx.ErrNoRows, got %v", err)
	}
}

func TestFileAfterRetractionRevivesTheSameRow(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	seedProcessReportCompany(t, pool, "revive-co")
	user := seedProcessReportUser(t, pool, "reviver@example.com")
	fileArg := FileCompanyProcessReportParams{UserID: user, CompanySlug: "revive-co", Kind: "ai_interview"}

	filed, err := q.FileCompanyProcessReport(ctx, fileArg)
	if err != nil {
		t.Fatalf("file: %v", err)
	}
	if _, err := q.RetractCompanyProcessReport(ctx, RetractCompanyProcessReportParams{
		UserID: user, CompanySlug: "revive-co", Kind: "ai_interview",
	}); err != nil {
		t.Fatalf("retract: %v", err)
	}

	revived, err := q.FileCompanyProcessReport(ctx, fileArg)
	if err != nil {
		t.Fatalf("re-file: %v", err)
	}
	if revived != filed {
		t.Fatalf("revived id = %d, want the original row %d", revived, filed)
	}
	if n := processReportRowCount(t, pool, user, "revive-co"); n != 1 {
		t.Fatalf("rows after revival = %d, want 1", n)
	}

	count, err := q.RecountCompanyProcessReports(ctx, "revive-co")
	if err != nil {
		t.Fatalf("recount: %v", err)
	}
	if count != 1 {
		t.Fatalf("counter after revival = %d, want 1", count)
	}
}

func TestCounterSumsDistinctReporters(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	seedProcessReportCompany(t, pool, "crowd-co")
	for _, email := range []string{"one@example.com", "two@example.com", "three@example.com"} {
		user := seedProcessReportUser(t, pool, email)
		if _, err := q.FileCompanyProcessReport(ctx, FileCompanyProcessReportParams{
			UserID: user, CompanySlug: "crowd-co", Kind: "ai_interview",
		}); err != nil {
			t.Fatalf("file for %s: %v", email, err)
		}
	}

	count, err := q.RecountCompanyProcessReports(ctx, "crowd-co")
	if err != nil {
		t.Fatalf("recount: %v", err)
	}
	if count != 3 {
		t.Fatalf("counter = %d, want 3", count)
	}
}

func TestUnknownKindIsRejectedByTheDatabase(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	seedProcessReportCompany(t, pool, "checked-co")
	user := seedProcessReportUser(t, pool, "checker@example.com")

	// The service rejects an unknown kind before it gets here; the CHECK is the
	// backstop that keeps the column from becoming free text if that guard is ever
	// bypassed, since the vocabulary decides what the badge renders.
	if _, err := q.FileCompanyProcessReport(ctx, FileCompanyProcessReportParams{
		UserID: user, CompanySlug: "checked-co", Kind: "unpaid_test_task",
	}); err == nil {
		t.Fatal("a kind outside the CHECK list was accepted")
	}
}

func TestCountRecentCompanyProcessReportsIgnoresRevivals(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	seedProcessReportCompany(t, pool, "rate-co")
	user := seedProcessReportUser(t, pool, "rated@example.com")
	fileArg := FileCompanyProcessReportParams{UserID: user, CompanySlug: "rate-co", Kind: "ai_interview"}

	if _, err := q.FileCompanyProcessReport(ctx, fileArg); err != nil {
		t.Fatalf("file: %v", err)
	}
	if _, err := q.RetractCompanyProcessReport(ctx, RetractCompanyProcessReportParams{
		UserID: user, CompanySlug: "rate-co", Kind: "ai_interview",
	}); err != nil {
		t.Fatalf("retract: %v", err)
	}
	if _, err := q.FileCompanyProcessReport(ctx, fileArg); err != nil {
		t.Fatalf("re-file: %v", err)
	}

	// A revival takes the ON CONFLICT branch and leaves created_at untouched, so the
	// rate window counts one genuinely new row — retract-and-refile is not a way to
	// spend somebody else's allowance.
	n, err := q.CountRecentCompanyProcessReports(ctx, CountRecentCompanyProcessReportsParams{
		UserID:    user,
		CreatedAt: pgtype.Timestamptz{Time: time.Now().Add(-time.Hour), Valid: true},
	})
	if err != nil {
		t.Fatalf("count recent: %v", err)
	}
	if n != 1 {
		t.Fatalf("recent count = %d, want 1", n)
	}
}

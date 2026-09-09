//go:build integration

// Integration tests for internal/engage/processreport's SQL-backed writes — Service is
// bound directly to *db.Queries/*pgxpool.Pool with no fake seam, so these behaviors can
// only be verified against a real Postgres. Run with:
// go test -tags=integration ./internal/engage/processreport/
package processreport

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/testdb"
)

func insertCompany(t *testing.T, pool *pgxpool.Pool, slug string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO companies (slug, name, job_count) VALUES ($1, $1, 1)`, slug); err != nil {
		t.Fatalf("insert company %q: %v", slug, err)
	}
}

func insertUser(t *testing.T, pool *pgxpool.Pool, email string) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO users (email) VALUES ($1) RETURNING id`, email).Scan(&id); err != nil {
		t.Fatalf("insert user %q: %v", email, err)
	}
	return id
}

func newTestService(t *testing.T, cfg Config) (*Service, *pgxpool.Pool) {
	t.Helper()
	pool := testdb.Pool(t)
	if _, err := pool.Exec(context.Background(),
		`TRUNCATE company_process_reports, jobs, companies, users RESTART IDENTITY CASCADE`); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	return New(db.New(pool), pool, cfg), pool
}

// storedCount is the company's materialized counter, read straight from the column —
// what proves the recompute ran in the same transaction as the write rather than being
// something the service merely returned.
func storedCount(t *testing.T, pool *pgxpool.Pool, slug string) int32 {
	t.Helper()
	var n int32
	if err := pool.QueryRow(context.Background(),
		`SELECT ai_interview_reports FROM companies WHERE slug = $1`, slug).Scan(&n); err != nil {
		t.Fatalf("read counter: %v", err)
	}
	return n
}

// seedJob inserts one posting owned by a company, so the sync onto jobs can be
// observed. The jobs table's own required columns are the only ones set.
func seedJob(t *testing.T, pool *pgxpool.Pool, slug, externalID string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO jobs (source, external_id, url, title, company, company_slug, public_slug)
		 VALUES ('test', $2, 'https://example.test/' || $2, 'Engineer', $1, $1, $2)`,
		slug, externalID); err != nil {
		t.Fatalf("insert job: %v", err)
	}
}

func jobCount(t *testing.T, pool *pgxpool.Pool, externalID string) int32 {
	t.Helper()
	var n int32
	if err := pool.QueryRow(context.Background(),
		`SELECT ai_interview_reports FROM jobs WHERE external_id = $1`, externalID).Scan(&n); err != nil {
		t.Fatalf("read job counter: %v", err)
	}
	return n
}

// The whole reason the counter is denormalized onto the posting: a card must not
// disagree with the company page about a fact filed a second ago. A scheduled sync
// would leave exactly that gap, so the write does it.
func TestFileSyncsTheCountOntoTheCompanysJobs(t *testing.T) {
	svc, pool := newTestService(t, Config{})
	insertCompany(t, pool, "synced-co")
	seedJob(t, pool, "synced-co", "job-one")
	seedJob(t, pool, "synced-co", "job-two")
	user := insertUser(t, pool, "syncer@example.com")
	ctx := context.Background()

	if _, err := svc.File(ctx, user, "synced-co", "ai_interview"); err != nil {
		t.Fatalf("file: %v", err)
	}
	for _, ext := range []string{"job-one", "job-two"} {
		if got := jobCount(t, pool, ext); got != 1 {
			t.Fatalf("%s counter after filing = %d, want 1", ext, got)
		}
	}

	if _, err := svc.Retract(ctx, user, "synced-co", "ai_interview"); err != nil {
		t.Fatalf("retract: %v", err)
	}
	for _, ext := range []string{"job-one", "job-two"} {
		if got := jobCount(t, pool, ext); got != 0 {
			t.Fatalf("%s counter after retraction = %d, want 0", ext, got)
		}
	}
}

func TestFileRaisesTheLabelOnOneReport(t *testing.T) {
	svc, pool := newTestService(t, Config{})
	insertCompany(t, pool, "acme-ai")
	user := insertUser(t, pool, "one@example.com")

	count, err := svc.File(context.Background(), user, "acme-ai", "ai_interview")
	if err != nil {
		t.Fatalf("file: %v", err)
	}
	// There is no contributor gate: a single observer knows a bot conducted their
	// interview as well as forty do.
	if count != 1 {
		t.Fatalf("returned count = %d, want 1", count)
	}
	if got := storedCount(t, pool, "acme-ai"); got != 1 {
		t.Fatalf("stored counter = %d, want 1", got)
	}
}

func TestFileTwiceIsAlreadyReported(t *testing.T) {
	svc, pool := newTestService(t, Config{})
	insertCompany(t, pool, "dupe-co")
	user := insertUser(t, pool, "dupe@example.com")
	ctx := context.Background()

	if _, err := svc.File(ctx, user, "dupe-co", "ai_interview"); err != nil {
		t.Fatalf("first file: %v", err)
	}
	if _, err := svc.File(ctx, user, "dupe-co", "ai_interview"); !errors.Is(err, ErrAlreadyReported) {
		t.Fatalf("second file: want ErrAlreadyReported, got %v", err)
	}
	if got := storedCount(t, pool, "dupe-co"); got != 1 {
		t.Fatalf("counter after duplicate = %d, want 1", got)
	}
}

func TestRetractLowersTheCountAndRefileRevives(t *testing.T) {
	svc, pool := newTestService(t, Config{})
	insertCompany(t, pool, "revive-co")
	user := insertUser(t, pool, "revive@example.com")
	ctx := context.Background()

	if _, err := svc.File(ctx, user, "revive-co", "ai_interview"); err != nil {
		t.Fatalf("file: %v", err)
	}
	count, err := svc.Retract(ctx, user, "revive-co", "ai_interview")
	if err != nil {
		t.Fatalf("retract: %v", err)
	}
	if count != 0 {
		t.Fatalf("count after retraction = %d, want 0", count)
	}
	if got := storedCount(t, pool, "revive-co"); got != 0 {
		t.Fatalf("stored counter after retraction = %d, want 0", got)
	}

	// Withdrawing again has nothing to withdraw and must say so rather than silently
	// restamping.
	if _, err := svc.Retract(ctx, user, "revive-co", "ai_interview"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second retract: want ErrNotFound, got %v", err)
	}

	// A retraction is not a way to file repeatedly: re-filing revives the same row,
	// so the count returns to one rather than two.
	count, err = svc.File(ctx, user, "revive-co", "ai_interview")
	if err != nil {
		t.Fatalf("re-file: %v", err)
	}
	if count != 1 {
		t.Fatalf("count after revival = %d, want 1", count)
	}
}

func TestRetractingWhatWasNeverFiled(t *testing.T) {
	svc, pool := newTestService(t, Config{})
	insertCompany(t, pool, "quiet-co")
	user := insertUser(t, pool, "quiet@example.com")

	if _, err := svc.Retract(context.Background(), user, "quiet-co", "ai_interview"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestUnknownKindIsRejectedBeforeAnyWrite(t *testing.T) {
	svc, pool := newTestService(t, Config{})
	insertCompany(t, pool, "checked-co")
	user := insertUser(t, pool, "checker@example.com")

	if _, err := svc.File(context.Background(), user, "checked-co", "unpaid_test_task"); !errors.Is(err, ErrInvalidKind) {
		t.Fatalf("want ErrInvalidKind, got %v", err)
	}
	// Rejected before the transaction opens, so the company is untouched.
	if got := storedCount(t, pool, "checked-co"); got != 0 {
		t.Fatalf("counter = %d, want 0", got)
	}
}

func TestUnknownCompanyIsNotFound(t *testing.T) {
	svc, pool := newTestService(t, Config{})
	user := insertUser(t, pool, "lost@example.com")

	if _, err := svc.File(context.Background(), user, "no-such-company", "ai_interview"); !errors.Is(err, ErrCompanyNotFound) {
		t.Fatalf("want ErrCompanyNotFound, got %v", err)
	}
}

func TestDistinctReportersAccumulate(t *testing.T) {
	svc, pool := newTestService(t, Config{})
	insertCompany(t, pool, "crowd-co")
	ctx := context.Background()

	for i, email := range []string{"a@example.com", "b@example.com", "c@example.com"} {
		user := insertUser(t, pool, email)
		count, err := svc.File(ctx, user, "crowd-co", "ai_interview")
		if err != nil {
			t.Fatalf("file %d: %v", i, err)
		}
		if want := int32(i + 1); count != want {
			t.Fatalf("count after %d reports = %d, want %d", i+1, count, want)
		}
	}
}

func TestRateLimitRefusesOverTheCap(t *testing.T) {
	svc, pool := newTestService(t, Config{Window: time.Hour, Cap: 2})
	for _, slug := range []string{"co-one", "co-two", "co-three"} {
		insertCompany(t, pool, slug)
	}
	user := insertUser(t, pool, "prolific@example.com")
	ctx := context.Background()

	if _, err := svc.File(ctx, user, "co-one", "ai_interview"); err != nil {
		t.Fatalf("first: %v", err)
	}
	if _, err := svc.File(ctx, user, "co-two", "ai_interview"); err != nil {
		t.Fatalf("second: %v", err)
	}
	// The cap exists so one account cannot label the catalogue; honest reporting will
	// not reach it at the shipped default.
	if _, err := svc.File(ctx, user, "co-three", "ai_interview"); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("third: want ErrRateLimited, got %v", err)
	}
	if got := storedCount(t, pool, "co-three"); got != 0 {
		t.Fatalf("refused report still counted: %d", got)
	}
}

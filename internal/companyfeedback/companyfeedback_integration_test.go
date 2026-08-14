//go:build integration

// Integration tests for internal/companyfeedback's SQL-backed writes — Service is
// bound directly to *db.Queries/*pgxpool.Pool with no fake seam, so these behaviors
// can only be verified against a real Postgres. Run with:
// go test -tags=integration ./internal/companyfeedback/
package companyfeedback

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/testdb"
	"github.com/strelov1/freehire/internal/vocab"
)

// fakePersonas hands out one fixed handle — Hide doesn't need a real persona
// resolver, and Upsert only needs SOME handle to mint a feedback row through.
type fakePersonas struct{}

func (fakePersonas) PersonaFor(context.Context, int64) (string, error) { return "quiet-otter", nil }

func insertCompany(t *testing.T, pool *pgxpool.Pool, slug, name string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO companies (slug, name, job_count) VALUES ($1, $2, 1)`, slug, name); err != nil {
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

func newTestService(t *testing.T) (*Service, *pgxpool.Pool) {
	t.Helper()
	pool := testdb.Pool(t)
	if _, err := pool.Exec(context.Background(),
		`TRUNCATE company_feedback, company_feedback_reports, companies, users RESTART IDENTITY CASCADE`); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	return New(db.New(pool), pool, fakePersonas{}, Config{}), pool
}

// Hide now reads the company slug (GetCompanyFeedbackSlug, unlocked) BEFORE locking
// the company row, instead of locking the feedback row first via HideCompanyFeedback's
// UPDATE — the fix for the lock-order mismatch against Upsert/Delete, which both lock
// the company row first. This exercises the resulting call sequence end-to-end: a
// hidden review stops counting toward the company's materialized counters and drops
// out of the public list, in the same transaction.
func TestHideRecomputesCountersAndDropsFromTheList(t *testing.T) {
	svc, pool := newTestService(t)
	insertCompany(t, pool, "acme", "Acme Corp")
	ctx := context.Background()
	reviewer := insertUser(t, pool, "reviewer1@example.test")

	fb, summary, err := svc.Upsert(ctx, reviewer, "acme", 5, vocab.CompanyFeedbackTypeValues[0], "Great place to work")
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if summary.Count != 1 {
		t.Fatalf("summary after upsert: count=%d, want 1", summary.Count)
	}

	if err := svc.Hide(ctx, fb.ID); err != nil {
		t.Fatalf("Hide: %v", err)
	}

	list, err := svc.List(ctx, "acme", 10, 0)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("List after Hide = %v, want empty (hidden reviews are excluded)", list)
	}
	count, err := svc.Count(ctx, "acme")
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if count != 0 {
		t.Errorf("Count after Hide = %d, want 0", count)
	}
}

// Hide is idempotent: hiding an already-hidden review is a no-op, not an error, since
// HideCompanyFeedback's UPDATE matches on id alone (no status predicate).
func TestHideIsIdempotent(t *testing.T) {
	svc, pool := newTestService(t)
	insertCompany(t, pool, "acme", "Acme Corp")
	ctx := context.Background()
	reviewer := insertUser(t, pool, "reviewer2@example.test")

	fb, _, err := svc.Upsert(ctx, reviewer, "acme", 4, vocab.CompanyFeedbackTypeValues[0], "Solid team")
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if err := svc.Hide(ctx, fb.ID); err != nil {
		t.Fatalf("first Hide: %v", err)
	}
	if err := svc.Hide(ctx, fb.ID); err != nil {
		t.Errorf("second Hide (already hidden): %v, want nil", err)
	}
}

func TestHideUnknownIDReturnsErrNotFound(t *testing.T) {
	svc, pool := newTestService(t)
	insertCompany(t, pool, "acme", "Acme Corp")

	if err := svc.Hide(context.Background(), 999999); !errors.Is(err, ErrNotFound) {
		t.Errorf("Hide unknown id: err=%v, want ErrNotFound", err)
	}
}

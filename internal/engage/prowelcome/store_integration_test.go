//go:build integration

// Integration test for the candidate query and the claim against a real Postgres — the
// unit tests in runner_test.go cover the runner's logic with fakes; this covers the SQL
// itself: that a newly-paying account is found exactly once across two runs, that a
// still-paying account is not found again, and that a send failure really does leave the
// row re-selectable. Run with: go test -tags=integration ./internal/engage/prowelcome/
package prowelcome

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/ai/plan"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/testdb"
)

func startPostgres(t *testing.T) *pgxpool.Pool {
	t.Helper()
	return testdb.Pool(t)
}

// makePayingUser inserts an account already entitled to a paying tier — SetProUntilGranted
// is the same "granted, not sold" seam handler tests use (see makePro in
// internal/api/handler/auto_apply_enqueue_integration_test.go): this test is about the
// candidate query and the claim, not about how a subscription is bought.
func makePayingUser(t *testing.T, pool *pgxpool.Pool, email string) int64 {
	t.Helper()
	ctx := context.Background()
	var id int64
	if err := pool.QueryRow(ctx, `INSERT INTO users (email) VALUES ($1) RETURNING id`, email).Scan(&id); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	q := db.New(pool)
	if err := q.SetProUntilGranted(ctx, db.SetProUntilGrantedParams{
		ID: id, Until: pgtype.Timestamptz{Time: time.Now().Add(24 * time.Hour), Valid: true},
	}); err != nil {
		t.Fatalf("grant pro: %v", err)
	}
	return id
}

func TestRunnerAgainstPostgres_WelcomesOnceAcrossTwoRuns(t *testing.T) {
	pool := startPostgres(t)
	store := db.New(pool)
	userID := makePayingUser(t, pool, "welcome-once@example.test")

	mailer := &fakeMailer{}
	r := New(store, mailer, 500)

	first, err := r.Run(context.Background())
	if err != nil {
		t.Fatalf("first run: %v", err)
	}
	if first.Sent != 1 {
		t.Fatalf("first run sent = %d, want 1", first.Sent)
	}
	if len(mailer.sent) != 1 || mailer.sent[0] != userID {
		t.Fatalf("mailer.sent = %v, want [%d]", mailer.sent, userID)
	}

	second, err := r.Run(context.Background())
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if second.Sent != 0 {
		t.Errorf("second run sent = %d, want 0 — the account was already welcomed", second.Sent)
	}
	if len(mailer.sent) != 1 {
		t.Errorf("mailer.sent after the second run = %v, want still just the one call", mailer.sent)
	}
}

func TestRunnerAgainstPostgres_FailedSendStaysACandidate(t *testing.T) {
	pool := startPostgres(t)
	store := db.New(pool)
	userID := makePayingUser(t, pool, "welcome-retry@example.test")

	failing := &fakeMailer{failFor: userID}
	r := New(store, failing, 500)

	first, err := r.Run(context.Background())
	if err != nil {
		t.Fatalf("first run: %v", err)
	}
	if first.Sent != 0 || first.Failed != 1 {
		t.Fatalf("first run = %+v, want {Sent:0 Failed:1}", first)
	}

	// The next run, with a working mailer, finds the SAME account again — a failed
	// send must never have stamped pro_welcome_sent_at.
	working := &fakeMailer{}
	r2 := New(store, working, 500)
	second, err := r2.Run(context.Background())
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if second.Sent != 1 {
		t.Fatalf("second run sent = %d, want 1 (the retried candidate)", second.Sent)
	}
	if len(working.sent) != 1 || working.sent[0] != userID {
		t.Fatalf("second run's mailer.sent = %v, want [%d]", working.sent, userID)
	}
}

func TestRunnerAgainstPostgres_FreeAccountIsNeverACandidate(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `INSERT INTO users (email) VALUES ('free-account@example.test')`); err != nil {
		t.Fatalf("insert user: %v", err)
	}

	mailer := &fakeMailer{}
	r := New(db.New(pool), mailer, 500)

	stats, err := r.Run(ctx)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if stats.Sent != 0 || stats.Failed != 0 || len(mailer.sent) != 0 {
		t.Errorf("stats = %+v, sent = %v — a free account must never be welcomed", stats, mailer.sent)
	}
}

func TestRunnerAgainstPostgres_ResolvesUltraOverPro(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	var id int64
	if err := pool.QueryRow(ctx, `INSERT INTO users (email) VALUES ('ultra-upgrade@example.test') RETURNING id`).Scan(&id); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	q := db.New(pool)
	// An upgrade leaves both live, Pro reaching further — plan.TierOf still resolves
	// Ultra, and the candidate query (which OR's the two columns) must still surface
	// the row for the runner to resolve correctly.
	if err := q.SetProUntilGranted(ctx, db.SetProUntilGrantedParams{
		ID: id, Until: pgtype.Timestamptz{Time: time.Now().Add(60 * 24 * time.Hour), Valid: true},
	}); err != nil {
		t.Fatalf("grant pro: %v", err)
	}
	if err := q.SetUltraUntilGranted(ctx, db.SetUltraUntilGrantedParams{
		ID: id, Until: pgtype.Timestamptz{Time: time.Now().Add(30 * 24 * time.Hour), Valid: true},
	}); err != nil {
		t.Fatalf("grant ultra: %v", err)
	}

	mailer := &fakeMailer{}
	if _, err := New(q, mailer, 500).Run(ctx); err != nil {
		t.Fatalf("run: %v", err)
	}
	if mailer.lastTier != plan.TierUltra {
		t.Errorf("tier = %q, want %q", mailer.lastTier, plan.TierUltra)
	}
}

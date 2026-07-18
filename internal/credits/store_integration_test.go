//go:build integration

// Integration tests for the credits Store against a real Postgres: the debit
// transaction is atomic and idempotent by ref, the monthly grant resets lazily on a
// period rollover, an insufficient balance is rejected without a ledger row, and a
// concurrent race never oversells. Run with: go test -tags=integration ./internal/credits/
// Requires Docker (testcontainers spins up a throwaway Postgres with the migrations).
package credits

import (
	"context"
	"errors"
	"path/filepath"
	"sort"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/strelov1/freehire/internal/db"
)

func startPostgres(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()

	migrationsDir, err := filepath.Abs(filepath.Join("..", "..", "migrations"))
	if err != nil {
		t.Fatalf("resolve migrations dir: %v", err)
	}
	scripts, err := filepath.Glob(filepath.Join(migrationsDir, "*.sql"))
	if err != nil || len(scripts) == 0 {
		t.Fatalf("list migrations: %v (found %d)", err, len(scripts))
	}
	sort.Strings(scripts)

	pg, err := postgres.Run(ctx, "postgres:18-alpine",
		postgres.WithDatabase("hire"),
		postgres.WithUsername("hire"),
		postgres.WithPassword("hire"),
		postgres.WithInitScripts(scripts...),
		postgres.BasicWaitStrategies(),
	)
	if err != nil {
		t.Fatalf("start postgres: %v", err)
	}
	t.Cleanup(func() { _ = pg.Terminate(ctx) })

	dsn, err := pg.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("connection string: %v", err)
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func insertUser(t *testing.T, pool *pgxpool.Pool, email string) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO users (email) VALUES ($1) RETURNING id`, email).Scan(&id); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	return id
}

// countDebits returns how many debit rows exist for a (user, feature, ref).
func countDebits(t *testing.T, pool *pgxpool.Pool, userID int64, feature, ref string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM credit_ledger WHERE user_id=$1 AND kind='debit' AND feature=$2 AND ref=$3`,
		userID, feature, ref).Scan(&n); err != nil {
		t.Fatalf("count debits: %v", err)
	}
	return n
}

// seedBalance forces a balance row into a specific period/remaining (to simulate a
// prior-period state for the rollover tests).
func seedBalance(t *testing.T, pool *pgxpool.Pool, userID int64, period string, remaining int32) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO credit_balances (user_id, period, remaining) VALUES ($1,$2,$3)`,
		userID, period, remaining); err != nil {
		t.Fatalf("seed balance: %v", err)
	}
}

func newStore(pool *pgxpool.Pool, cfg Config) *Store {
	return NewStore(db.New(pool), pool, cfg)
}

func TestBalance_freshUserReportsFullGrant(t *testing.T) {
	pool := startPostgres(t)
	s := newStore(pool, Config{MonthlyGrant: 20, CostMatch: 1, CostTailor: 3})
	uid := insertUser(t, pool, "fresh@example.test")

	bal, err := s.Balance(context.Background(), uid)
	if err != nil {
		t.Fatalf("Balance: %v", err)
	}
	if bal.Remaining != 20 {
		t.Errorf("fresh Remaining = %d, want 20", bal.Remaining)
	}
	if bal.ResetsAt.IsZero() {
		t.Error("ResetsAt should be set")
	}
}

func TestBalance_rolledOverPeriodReportsFullGrant(t *testing.T) {
	pool := startPostgres(t)
	s := newStore(pool, Config{MonthlyGrant: 20, CostMatch: 1, CostTailor: 3})
	uid := insertUser(t, pool, "rolled@example.test")
	seedBalance(t, pool, uid, "2000-01", 2) // ancient period, nearly spent

	bal, err := s.Balance(context.Background(), uid)
	if err != nil {
		t.Fatalf("Balance: %v", err)
	}
	if bal.Remaining != 20 {
		t.Errorf("rolled-over Remaining = %d, want 20 (fresh grant)", bal.Remaining)
	}
}

func TestDebit_sufficientChargesOnce(t *testing.T) {
	pool := startPostgres(t)
	s := newStore(pool, Config{MonthlyGrant: 20, CostMatch: 1, CostTailor: 3})
	uid := insertUser(t, pool, "debit@example.test")

	bal, err := s.Debit(context.Background(), uid, FeatureTailor, "cv-1")
	if err != nil {
		t.Fatalf("Debit: %v", err)
	}
	if bal.Remaining != 17 {
		t.Errorf("Remaining after tailor debit = %d, want 17", bal.Remaining)
	}
	if got := countDebits(t, pool, uid, "tailor", "cv-1"); got != 1 {
		t.Errorf("debit rows = %d, want 1", got)
	}
}

func TestDebit_repeatRefIsFree(t *testing.T) {
	pool := startPostgres(t)
	s := newStore(pool, Config{MonthlyGrant: 20, CostMatch: 1, CostTailor: 3})
	uid := insertUser(t, pool, "repeat@example.test")
	ctx := context.Background()

	if _, err := s.Debit(ctx, uid, FeatureMatch, "job-1"); err != nil {
		t.Fatalf("first Debit: %v", err)
	}
	bal, err := s.Debit(ctx, uid, FeatureMatch, "job-1") // same ref
	if err != nil {
		t.Fatalf("second Debit: %v", err)
	}
	if bal.Remaining != 19 {
		t.Errorf("Remaining after repeat = %d, want 19 (charged once)", bal.Remaining)
	}
	if got := countDebits(t, pool, uid, "match", "job-1"); got != 1 {
		t.Errorf("debit rows for repeated ref = %d, want 1", got)
	}
}

func TestDebit_insufficientRejected(t *testing.T) {
	pool := startPostgres(t)
	s := newStore(pool, Config{MonthlyGrant: 1, CostMatch: 1, CostTailor: 3})
	uid := insertUser(t, pool, "broke@example.test")
	ctx := context.Background()

	if _, err := s.Debit(ctx, uid, FeatureMatch, "job-1"); err != nil {
		t.Fatalf("first Debit: %v", err)
	}
	bal, err := s.Debit(ctx, uid, FeatureMatch, "job-2") // new ref, no points left
	if !errors.Is(err, ErrInsufficient) {
		t.Fatalf("second Debit err = %v, want ErrInsufficient", err)
	}
	if bal.Remaining != 0 {
		t.Errorf("Remaining on insufficient = %d, want 0", bal.Remaining)
	}
	if got := countDebits(t, pool, uid, "match", "job-2"); got != 0 {
		t.Errorf("insufficient must append no debit row, got %d", got)
	}
}

func TestDebit_lazyResetThenDebit(t *testing.T) {
	pool := startPostgres(t)
	s := newStore(pool, Config{MonthlyGrant: 20, CostMatch: 1, CostTailor: 3})
	uid := insertUser(t, pool, "reset@example.test")
	seedBalance(t, pool, uid, "2000-01", 0) // prior period, exhausted

	bal, err := s.Debit(context.Background(), uid, FeatureMatch, "job-1")
	if err != nil {
		t.Fatalf("Debit after rollover: %v", err)
	}
	// Reset to 20, then -1 for the match.
	if bal.Remaining != 19 {
		t.Errorf("Remaining after reset+debit = %d, want 19", bal.Remaining)
	}
}

func TestDebit_concurrentNoOversell(t *testing.T) {
	pool := startPostgres(t)
	s := newStore(pool, Config{MonthlyGrant: 1, CostMatch: 1, CostTailor: 3})
	uid := insertUser(t, pool, "race@example.test")
	ctx := context.Background()

	var wg sync.WaitGroup
	errs := make([]error, 2)
	refs := []string{"job-a", "job-b"}
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, errs[i] = s.Debit(ctx, uid, FeatureMatch, refs[i])
		}(i)
	}
	wg.Wait()

	var ok, broke int
	for _, err := range errs {
		switch {
		case err == nil:
			ok++
		case errors.Is(err, ErrInsufficient):
			broke++
		default:
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if ok != 1 || broke != 1 {
		t.Errorf("race outcome ok=%d insufficient=%d, want 1 and 1", ok, broke)
	}
	// Exactly one debit row total for the two distinct refs.
	var total int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM credit_ledger WHERE user_id=$1 AND kind='debit'`, uid).Scan(&total); err != nil {
		t.Fatalf("count: %v", err)
	}
	if total != 1 {
		t.Errorf("debit rows after race = %d, want exactly 1", total)
	}
}

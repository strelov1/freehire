//go:build integration

// Integration tests for the credits Store against a real Postgres: the debit
// transaction is atomic and idempotent by ref, the monthly grant resets lazily on a
// period rollover, an insufficient balance is rejected without a ledger row, and a
// concurrent race never oversells. Run with: go test -tags=integration ./internal/ai/credits/
// Requires Docker (testcontainers spins up a throwaway Postgres with the migrations).
package credits

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/strelov1/freehire/internal/platform/testdb"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/platform/db"
)

func startPostgres(t *testing.T) *pgxpool.Pool {
	t.Helper()
	return testdb.Pool(t)
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

func TestReward_addsAndIsIdempotent(t *testing.T) {
	pool := startPostgres(t)
	s := newStore(pool, Config{MonthlyGrant: 20, CostMatch: 1, CostTailor: 3, ContributionReward: 5})
	uid := insertUser(t, pool, "reward@example.test")
	ctx := context.Background()

	bal, err := s.Reward(ctx, uid, "contrib-1")
	if err != nil {
		t.Fatalf("Reward: %v", err)
	}
	if bal.Remaining != 25 { // 20 grant + 5 reward
		t.Errorf("Remaining after reward = %d, want 25", bal.Remaining)
	}
	// Same contribution again → no double reward.
	bal, err = s.Reward(ctx, uid, "contrib-1")
	if err != nil {
		t.Fatalf("second Reward: %v", err)
	}
	if bal.Remaining != 25 {
		t.Errorf("Remaining after repeat reward = %d, want 25 (idempotent)", bal.Remaining)
	}
	// A distinct contribution stacks.
	bal, _ = s.Reward(ctx, uid, "contrib-2")
	if bal.Remaining != 30 {
		t.Errorf("Remaining after second distinct reward = %d, want 30", bal.Remaining)
	}
}

func TestReward_banksAboveGrantAcrossPeriod(t *testing.T) {
	pool := startPostgres(t)
	s := newStore(pool, Config{MonthlyGrant: 20, CostMatch: 1, CostTailor: 3, ContributionReward: 5})
	uid := insertUser(t, pool, "bank@example.test")
	ctx := context.Background()

	// Earn a reward this... actually seed a prior period with a banked surplus (grant 20 +
	// 5 reward, unspent) and roll over: the surplus above the grant must survive.
	seedBalance(t, pool, uid, "2000-01", 25)
	bal, err := s.Balance(ctx, uid)
	if err != nil {
		t.Fatalf("Balance: %v", err)
	}
	if bal.Remaining != 25 {
		t.Errorf("rolled-over Remaining = %d, want 25 (banked surplus survives)", bal.Remaining)
	}
	// A debit in the new period works off the banked balance and persists the reset.
	bal, err = s.Debit(ctx, uid, FeatureMatch, "job-1")
	if err != nil {
		t.Fatalf("Debit: %v", err)
	}
	if bal.Remaining != 24 {
		t.Errorf("Remaining after reset+debit = %d, want 24 (25 banked - 1)", bal.Remaining)
	}
}

func TestReward_zeroConfigIsNoOp(t *testing.T) {
	pool := startPostgres(t)
	s := newStore(pool, Config{MonthlyGrant: 20, CostMatch: 1, CostTailor: 3, ContributionReward: 0})
	uid := insertUser(t, pool, "noreward@example.test")

	bal, err := s.Reward(context.Background(), uid, "contrib-1")
	if err != nil {
		t.Fatalf("Reward: %v", err)
	}
	if bal.Remaining != 20 {
		t.Errorf("Remaining with zero reward = %d, want 20 (no-op)", bal.Remaining)
	}
	var rewards int
	_ = pool.QueryRow(context.Background(),
		`SELECT count(*) FROM credit_ledger WHERE user_id=$1 AND kind='reward'`, uid).Scan(&rewards)
	if rewards != 0 {
		t.Errorf("reward rows with zero config = %d, want 0", rewards)
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

// TestRelease_givesTheCreditBackAndFreesTheRef exercises the void statement for real. sqlc
// generates Go from SQL but never runs it, and this one deletes a row that a partial unique
// index depends on — the whole reason a reservation is voided rather than compensated.
func TestRelease_givesTheCreditBackAndFreesTheRef(t *testing.T) {
	pool := startPostgres(t)
	s := newStore(pool, Config{MonthlyGrant: 20, CostMatch: 1, CostTailor: 3})
	uid := insertUser(t, pool, "release@example.test")
	ctx := context.Background()

	if _, err := s.Debit(ctx, uid, FeatureMatch, "job-1"); err != nil {
		t.Fatalf("Debit: %v", err)
	}

	bal, err := s.Release(ctx, uid, FeatureMatch, "job-1")
	if err != nil {
		t.Fatalf("Release: %v", err)
	}
	if bal.Remaining != 20 {
		t.Errorf("Remaining after release = %d, want the full grant back", bal.Remaining)
	}
	if got := countDebits(t, pool, uid, "match", "job-1"); got != 0 {
		t.Errorf("debit rows after release = %d, want 0", got)
	}

	// The ref is chargeable again, which is the point: a candidate whose analysis failed
	// retries and is charged once, not never and not twice. A compensating ledger row could
	// not do this — credit_ledger_debit_ref_uniq would still hold the ref.
	if _, err := s.Debit(ctx, uid, FeatureMatch, "job-1"); err != nil {
		t.Fatalf("re-Debit after release: %v", err)
	}
	if got := countDebits(t, pool, uid, "match", "job-1"); got != 1 {
		t.Errorf("debit rows after re-charge = %d, want 1", got)
	}
	if b, _ := s.Balance(ctx, uid); b.Remaining != 19 {
		t.Errorf("Remaining after re-charge = %d, want 19", b.Remaining)
	}
}

// TestRelease_isIdempotentAndSafeOnNothing pins what lets every failure path call it without
// first working out whether it owes one.
func TestRelease_isIdempotentAndSafeOnNothing(t *testing.T) {
	pool := startPostgres(t)
	s := newStore(pool, Config{MonthlyGrant: 20, CostMatch: 1, CostTailor: 3})
	uid := insertUser(t, pool, "release-idem@example.test")
	ctx := context.Background()

	// Never charged: nothing to give back, and no balance invented.
	bal, err := s.Release(ctx, uid, FeatureMatch, "never-charged")
	if err != nil {
		t.Fatalf("Release of an uncharged ref: %v", err)
	}
	if bal.Remaining != 20 {
		t.Errorf("Remaining = %d, want the untouched grant", bal.Remaining)
	}

	if _, err := s.Debit(ctx, uid, FeatureMatch, "job-2"); err != nil {
		t.Fatalf("Debit: %v", err)
	}
	for i := range 3 {
		if _, err := s.Release(ctx, uid, FeatureMatch, "job-2"); err != nil {
			t.Fatalf("Release %d: %v", i, err)
		}
	}
	if b, _ := s.Balance(ctx, uid); b.Remaining != 20 {
		t.Errorf("Remaining after three releases = %d, want 20 — a release gives back once", b.Remaining)
	}
}

// TestRelease_acrossAPeriodRolloverGivesBackNoMoreThanItTook is the ordering the reset forces.
// Putting the cost back AFTER applying the monthly reset would floor the balance at the fresh
// grant and then add on top, so a release that crossed a month boundary would hand out a
// credit the candidate never had.
func TestRelease_acrossAPeriodRolloverGivesBackNoMoreThanItTook(t *testing.T) {
	pool := startPostgres(t)
	s := newStore(pool, Config{MonthlyGrant: 20, CostMatch: 1, CostTailor: 3})
	uid := insertUser(t, pool, "release-rollover@example.test")
	ctx := context.Background()

	if _, err := s.Debit(ctx, uid, FeatureMatch, "job-1"); err != nil {
		t.Fatalf("Debit: %v", err)
	}
	// Age the stored row into a previous period, leaving 19 banked there.
	if _, err := pool.Exec(ctx,
		`UPDATE credit_balances SET period = '1970-01' WHERE user_id = $1`, uid); err != nil {
		t.Fatalf("age the balance row: %v", err)
	}

	bal, err := s.Release(ctx, uid, FeatureMatch, "job-1")
	if err != nil {
		t.Fatalf("Release: %v", err)
	}
	if bal.Remaining != 20 {
		t.Errorf("Remaining = %d, want exactly the fresh grant 20 — never grant+cost", bal.Remaining)
	}
	if b, _ := s.Balance(ctx, uid); b.Remaining != 20 {
		t.Errorf("stored Remaining = %d, want 20", b.Remaining)
	}
}

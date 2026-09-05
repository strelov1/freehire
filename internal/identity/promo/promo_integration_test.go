//go:build integration

// Integration tests for the discount tables against a real Postgres. Everything here is a
// property of a STATEMENT or a CONSTRAINT — a seat that cannot be won twice, one code per
// account for life, one reward per invitee for life, and the freshness rule that keeps an
// old account from being attributed. None of them can be tested against a fake, because in
// a fake they are whatever the fake says they are.
//
// Run with: go test -tags=integration ./internal/identity/promo/
// Requires Docker (testcontainers spins up a throwaway Postgres with the migrations).
package promo

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/testdb"
)

func newRepo(t *testing.T) (*QueriesRepository, *pgxpool.Pool) {
	t.Helper()
	pool := testdb.Pool(t)
	return NewQueriesRepository(db.New(pool)), pool
}

func addUser(t *testing.T, pool *pgxpool.Pool, email string) int64 {
	t.Helper()
	var id int64
	err := pool.QueryRow(context.Background(),
		`INSERT INTO users (email) VALUES ($1) RETURNING id`, email).Scan(&id)
	if err != nil {
		t.Fatalf("inserting user %s: %v", email, err)
	}
	return id
}

// codeSeq numbers the codes a test run creates, so two tests in one package cannot collide
// on the primary key.
var codeSeq atomic.Int64

// addCode inserts a usable code and returns its spelling.
//
// The spelling is BUILT rather than written down, and that is not fussiness. The table
// refuses anything beginning with 'ZZ', which is what makes the source guard's fixture
// exemption enforceable — so a test that wants a row cannot use the exempted prefix, and a
// test that wrote any other code as a literal would trip the guard. Generating one satisfies
// both: nothing code-shaped appears in this file, and the database accepts what it gets.
func addCode(t *testing.T, pool *pgxpool.Pool, percent int16, maxUses *int32) string {
	t.Helper()
	code := fmt.Sprintf("FIXT%06d", codeSeq.Add(1))
	_, err := pool.Exec(context.Background(),
		`INSERT INTO promo_codes (code, percent_off, max_uses) VALUES ($1, $2, $3)`,
		code, percent, maxUses)
	if err != nil {
		t.Fatalf("inserting code %s: %v", code, err)
	}
	return code
}

func TestTheLastSeatIsWonOnce(t *testing.T) {
	repo, pool := newRepo(t)
	ctx := context.Background()
	one := int32(1)
	code := addCode(t, pool, 50, &one)

	first := addUser(t, pool, "one@example.test")
	second := addUser(t, pool, "two@example.test")

	// Concurrent, because the moment a launch offer runs out of seats is exactly the moment
	// it is popular. A read-then-write would let both of these through.
	var wg sync.WaitGroup
	results := make([]error, 2)
	for i, userID := range []int64{first, second} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, results[i] = repo.Redeem(ctx, userID, code)
		}()
	}
	wg.Wait()

	won := 0
	for _, err := range results {
		switch {
		case err == nil:
			won++
		case errors.Is(err, ErrNotUsable):
		default:
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if won != 1 {
		t.Fatalf("%d redemptions succeeded, want exactly 1", won)
	}

	var uses int32
	if err := pool.QueryRow(ctx, `SELECT uses FROM promo_codes WHERE code = $1`, code).Scan(&uses); err != nil {
		t.Fatalf("reading uses: %v", err)
	}
	if uses != 1 {
		t.Fatalf("uses = %d, want 1 — the losing statement must roll back whole, seat "+
			"increment included", uses)
	}
}

func TestASecondCodeNeverTouchesItsSeatCount(t *testing.T) {
	repo, pool := newRepo(t)
	ctx := context.Background()
	first := addCode(t, pool, 50, nil)
	second := addCode(t, pool, 90, nil)

	userID := addUser(t, pool, "greedy@example.test")
	if _, err := repo.Redeem(ctx, userID, first); err != nil {
		t.Fatalf("first redemption: %v", err)
	}
	if _, err := repo.Redeem(ctx, userID, second); !errors.Is(err, ErrNotUsable) {
		t.Fatalf("second redemption: %v, want ErrNotUsable", err)
	}

	var uses int32
	if err := pool.QueryRow(ctx, `SELECT uses FROM promo_codes WHERE code = $1`, second).Scan(&uses); err != nil {
		t.Fatalf("reading uses: %v", err)
	}
	if uses != 0 {
		t.Fatalf("uses = %d, want 0 — a caller who is already ineligible must not consume a "+
			"seat of the code they were refused", uses)
	}
}

func TestAnOldAccountCannotBeAttributed(t *testing.T) {
	repo, pool := newRepo(t)
	ctx := context.Background()

	referrer := addUser(t, pool, "referrer@example.test")
	old := addUser(t, pool, "old@example.test")
	if _, err := pool.Exec(ctx,
		`UPDATE users SET created_at = now() - interval '2 years' WHERE id = $1`, old); err != nil {
		t.Fatalf("ageing the account: %v", err)
	}

	written, err := repo.Attribute(ctx, referrer, old)
	if err != nil {
		t.Fatalf("Attribute: %v", err)
	}
	if written {
		t.Fatal("an account two years old was attributed — otherwise it could open a " +
			"friend's link, sign in, and collect a first-month discount plus a reward for " +
			"the friend, which is a promo code with extra steps and one nobody rationed")
	}
}

func TestAnInviteeIsWorthOneRewardForLife(t *testing.T) {
	repo, pool := newRepo(t)
	ctx := context.Background()

	first := addUser(t, pool, "first-referrer@example.test")
	second := addUser(t, pool, "second-referrer@example.test")
	invitee := addUser(t, pool, "invitee@example.test")

	if written, err := repo.Attribute(ctx, first, invitee); err != nil || !written {
		t.Fatalf("first attribution: written=%v err=%v", written, err)
	}
	written, err := repo.Attribute(ctx, second, invitee)
	if err != nil {
		t.Fatalf("second attribution: %v", err)
	}
	if written {
		t.Fatal("one invitee was attributed twice")
	}

	var owner int64
	if err := pool.QueryRow(ctx,
		`SELECT referrer_id FROM invite_rewards WHERE referee_id = $1`, invitee).Scan(&owner); err != nil {
		t.Fatalf("reading the reward: %v", err)
	}
	if owner != first {
		t.Fatalf("referrer = %d, want %d — the first attribution stands", owner, first)
	}
}

func TestAStoreOnlyInviteeIsNeverAskedAbout(t *testing.T) {
	repo, pool := newRepo(t)
	ctx := context.Background()

	referrer := addUser(t, pool, "ref@example.test")
	// An invitee who subscribed through the App Store: entitled, no Stripe customer, and
	// therefore no invoice anybody here can read.
	storeBuyer := addUser(t, pool, "store@example.test")
	if _, err := pool.Exec(ctx,
		`UPDATE users SET pro_until_revenuecat = now() + interval '30 days' WHERE id = $1`,
		storeBuyer); err != nil {
		t.Fatalf("granting a store subscription: %v", err)
	}
	if _, err := repo.Attribute(ctx, referrer, storeBuyer); err != nil {
		t.Fatalf("Attribute: %v", err)
	}

	pending, err := repo.PendingRewards(ctx, 100)
	if err != nil {
		t.Fatalf("PendingRewards: %v", err)
	}
	if len(pending) != 0 {
		t.Fatalf("%d rewards offered to the grant pass, want 0 — a store subscription "+
			"produces no invoice we can read, so there is nothing to earn from", len(pending))
	}
}

func TestGrantAndDeliverAreEachIdempotent(t *testing.T) {
	repo, pool := newRepo(t)
	ctx := context.Background()

	referrer := addUser(t, pool, "earner@example.test")
	invitee := addUser(t, pool, "payer@example.test")
	if _, err := repo.Attribute(ctx, referrer, invitee); err != nil {
		t.Fatalf("Attribute: %v", err)
	}

	var rewardID int64
	if err := pool.QueryRow(ctx,
		`SELECT id FROM invite_rewards WHERE referee_id = $1`, invitee).Scan(&rewardID); err != nil {
		t.Fatalf("reading the reward: %v", err)
	}

	moved, err := repo.Grant(ctx, rewardID, 250, 12)
	if err != nil || !moved {
		t.Fatalf("first Grant: moved=%v err=%v", moved, err)
	}
	// A second grant at a DIFFERENT amount, so a guard that only checked the id would show
	// up as a changed row rather than as a no-op.
	if moved, err := repo.Grant(ctx, rewardID, 999, 12); err != nil || moved {
		t.Fatalf("second Grant: moved=%v err=%v, want false", moved, err)
	}

	var amount int64
	if err := pool.QueryRow(ctx,
		`SELECT amount_cents FROM invite_rewards WHERE id = $1`, rewardID).Scan(&amount); err != nil {
		t.Fatalf("reading the amount: %v", err)
	}
	if amount != 250 {
		t.Fatalf("amount = %d, want 250 — the amount is fixed when the reward is earned", amount)
	}

	if stamped, err := repo.MarkDelivered(ctx, rewardID); err != nil || !stamped {
		t.Fatalf("first MarkDelivered: stamped=%v err=%v", stamped, err)
	}
	if stamped, err := repo.MarkDelivered(ctx, rewardID); err != nil || stamped {
		t.Fatalf("second MarkDelivered: stamped=%v err=%v, want false", stamped, err)
	}

	left, err := repo.UndeliveredRewards(ctx, 100)
	if err != nil {
		t.Fatalf("UndeliveredRewards: %v", err)
	}
	if len(left) != 0 {
		t.Fatalf("%d rewards still undelivered, want 0", len(left))
	}
}

// The statement's own ceiling guard, tested SEQUENTIALLY on purpose.
//
// An earlier version of this test granted concurrently and asserted that exactly one
// succeeded. It passed, and it was asserting something the statement does not provide: the
// UPDATE locks only the reward row it touches, so two grants for different rewards of one
// referrer never block each other, and under READ COMMITTED each count subquery reads its
// own statement's snapshot. Both would see the same number. What serializes the pass is the
// advisory lock in cmd/billing-sync; this guard is the second line, and this is what it
// actually promises.
func TestTheCeilingRefusesAGrantOverIt(t *testing.T) {
	repo, pool := newRepo(t)
	ctx := context.Background()

	referrer := addUser(t, pool, "prolific@example.test")

	// Two pending rewards, and a ceiling of two with one seat already spent.
	ids := make([]int64, 0, 2)
	for _, name := range []string{"a@example.test", "b@example.test"} {
		invitee := addUser(t, pool, name)
		if _, err := repo.Attribute(ctx, referrer, invitee); err != nil {
			t.Fatalf("Attribute: %v", err)
		}
		var id int64
		if err := pool.QueryRow(ctx,
			`SELECT id FROM invite_rewards WHERE referee_id = $1`, invitee).Scan(&id); err != nil {
			t.Fatalf("reading the reward: %v", err)
		}
		ids = append(ids, id)
	}

	// One already earned, so the ceiling of two leaves exactly one seat.
	seedInvitee := addUser(t, pool, "seed@example.test")
	if _, err := pool.Exec(ctx,
		`INSERT INTO invite_rewards (referrer_id, referee_id, status, amount_cents, granted_at)
		 VALUES ($1, $2, 'granted', 250, now())`, referrer, seedInvitee); err != nil {
		t.Fatalf("seeding: %v", err)
	}

	// One after the other, as the serialized pass runs them.
	granted := 0
	for _, id := range ids {
		ok, err := repo.Grant(ctx, id, 250, 2)
		if err != nil {
			t.Fatalf("Grant: %v", err)
		}
		if ok {
			granted++
		}
	}
	if granted != 1 {
		t.Fatalf("%d rewards granted, want 1 — the second grant must see the first and "+
			"refuse, or the ceiling is only advice", granted)
	}

	var total int64
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM invite_rewards WHERE referrer_id = $1 AND status = 'granted'`,
		referrer).Scan(&total); err != nil {
		t.Fatalf("counting: %v", err)
	}
	if total != 2 {
		t.Fatalf("granted rewards = %d, want 2 — the ceiling", total)
	}
}

func TestNobodyRefersThemselves(t *testing.T) {
	repo, pool := newRepo(t)
	ctx := context.Background()

	userID := addUser(t, pool, "solo@example.test")
	// Straight at the repository, bypassing the service's own check — the constraint is the
	// backstop, and a backstop nobody has hit is a backstop nobody has checked.
	if _, err := repo.Attribute(ctx, userID, userID); err == nil {
		t.Fatal("a self-referral was written")
	}
}

func TestAnInviteCodeIsMintedOnceAndNeverRotates(t *testing.T) {
	repo, pool := newRepo(t)
	ctx := context.Background()

	userID := addUser(t, pool, "sharer@example.test")
	first, err := repo.EnsureInviteCode(ctx, userID, strings.Repeat("A", 20))
	if err != nil {
		t.Fatalf("first EnsureInviteCode: %v", err)
	}
	second, err := repo.EnsureInviteCode(ctx, userID, strings.Repeat("B", 20))
	if err != nil {
		t.Fatalf("second EnsureInviteCode: %v", err)
	}
	if first != second {
		t.Fatalf("code changed from %q to %q — a link people have already shared must keep "+
			"working", first, second)
	}

	owner, err := repo.ReferrerByCode(ctx, first)
	if err != nil {
		t.Fatalf("ReferrerByCode: %v", err)
	}
	if owner != userID {
		t.Fatalf("owner = %d, want %d", owner, userID)
	}
}

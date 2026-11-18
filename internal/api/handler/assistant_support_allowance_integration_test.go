//go:build integration

package handler

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/strelov1/freehire/internal/ai/assistant"
	"github.com/strelov1/freehire/internal/ai/plan"
	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/platform/db"
)

func TestSupportMessageTakesItsOwnAllowanceWhenAnotherTurnHoldsItsReference(t *testing.T) {
	pool := startPostgres(t)
	iss := auth.NewIssuer("test-secret", time.Hour)
	app, h := supportApp(t, pool, iss, &gateCountingModel{}, plan.DefaultConfig().Enforcing(), nil)
	userID, token := assistantUser(t, pool, iss, "support-shared-ref@example.test", false)
	id := newSupportSession(t, app, token)

	if _, err := h.plans.ConsumeSupport(context.Background(), userID, id+"#turn-1"); err != nil {
		t.Fatalf("seed the in-flight turn's charge: %v", err)
	}
	if status, body := sendSupport(t, app, id, token); status != fiber.StatusOK {
		t.Fatalf("message = %d (%s), want 200", status, body)
	}
	if got := usedToday(t, pool, userID, plan.FeatureSupport); got != 2 {
		t.Errorf("support used today = %d, want 2 — the queued message ran on the other turn's charge", got)
	}
}

func TestSupportConcurrentMessagesEachTakeAnAllowance(t *testing.T) {
	pool := startPostgres(t)
	iss := auth.NewIssuer("test-secret", time.Hour)
	_, h := supportApp(t, pool, iss, &gateCountingModel{}, plan.DefaultConfig().Enforcing(), nil)
	userID, _ := assistantUser(t, pool, iss, "support-concurrent-charge@example.test", false)
	sessionID := seedSupportSession(t, h, userID)

	refs := make(chan string, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ref, d, err := reserveTurnRef(sessionID, 1, func(ref string) (plan.Decision, error) {
				return h.plans.ConsumeSupport(context.Background(), userID, ref)
			})
			if err != nil || d.Charge != 1 {
				t.Errorf("reserve: charge %d, err %v; want a charge of 1", d.Charge, err)
			}
			refs <- ref
		}()
	}
	wg.Wait()
	close(refs)

	seen := map[string]bool{}
	for ref := range refs {
		seen[ref] = true
	}
	if len(seen) != 2 {
		t.Errorf("the two messages shared a charge reference: %v", seen)
	}
	if got := usedToday(t, pool, userID, plan.FeatureSupport); got != 2 {
		t.Errorf("support used today = %d, want 2", got)
	}
}

// A conversation that cannot take another message is refused as busy before the allowance is
// read, so a person whose allowance happens to be spent is not told that when the real
// reason is the queue.
func TestSupportMessageForAFullSessionIsRefusedBusyBeforeItIsMetered(t *testing.T) {
	pool := startPostgres(t)
	iss := auth.NewIssuer("test-secret", time.Hour)
	cfg := plan.DefaultConfig().Enforcing()
	cfg.SupportMessagesPerDay = 1
	app, h := supportApp(t, pool, iss, &gateCountingModel{}, cfg, nil)
	userID, token := assistantUser(t, pool, iss, "support-full-session@example.test", false)
	id := newSupportSession(t, app, token)
	sessionID := uuid.MustParse(id)

	if _, err := h.plans.ConsumeSupport(context.Background(), userID, "spent-elsewhere"); err != nil {
		t.Fatalf("spend the day's allowance: %v", err)
	}
	if _, _, err := h.turns.claim(sessionID, func() {}); err != nil {
		t.Fatalf("hold the running turn: %v", err)
	}
	if _, w, err := h.turns.claim(sessionID, func() {}); err != nil || w == nil {
		t.Fatalf("hold the queued turn: %v, %v", w, err)
	}

	if status, body := sendSupport(t, app, id, token); status != fiber.StatusConflict {
		t.Fatalf("message to a full session = %d (%s), want 409", status, body)
	}
	if got := usedToday(t, pool, userID, plan.FeatureSupport); got != 1 {
		t.Errorf("support used today = %d, want 1", got)
	}
}

// A create racing another must not insert a second empty support session. The test plays
// the first create by hand: it holds the account's lock and has not inserted yet. A second
// create must wait for it, then hand back the row it inserted. Checked and inserted apart,
// the second would find nothing and add its own undeletable row.
func TestARacingSupportCreateReusesTheSessionBeingMade(t *testing.T) {
	pool := startPostgres(t)
	iss := auth.NewIssuer("test-secret", time.Hour)
	app, _ := supportApp(t, pool, iss, &gateCountingModel{}, plan.DefaultConfig().Enforcing(), nil)
	userID, token := assistantUser(t, pool, iss, "support-race@example.test", false)
	ctx := context.Background()

	first, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = first.Rollback(ctx) }()
	if err := db.New(first).LockAssistantSessionCreation(ctx, userID); err != nil {
		t.Fatalf("lock: %v", err)
	}

	type result struct {
		status int
		body   string
	}
	second := make(chan result, 1)
	go func() {
		status, body := supportPost(t, app, "/api/v1/support/sessions", token, `{}`)
		second <- result{status, body}
	}()
	select {
	case r := <-second:
		t.Fatalf("the second create finished (%d) while the first still held the lock", r.status)
	case <-time.After(500 * time.Millisecond):
	}

	made, err := assistant.NewStore(db.New(first)).CreateSession(ctx, userID, assistant.PresetSupport, nil, nil)
	if err != nil {
		t.Fatalf("first create: %v", err)
	}
	if err := first.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}

	r := <-second
	if r.status != fiber.StatusOK || !strings.Contains(r.body, made.ID.String()) {
		t.Fatalf("second create = %d (%s), want 200 with the first one's session %s", r.status, r.body, made.ID)
	}
	var n int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM assistant_sessions WHERE user_id = $1 AND preset = 'support'`, userID).Scan(&n); err != nil {
		t.Fatalf("count sessions: %v", err)
	}
	if n != 1 {
		t.Fatalf("two racing creates left %d support sessions, want 1", n)
	}
}

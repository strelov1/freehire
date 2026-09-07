package handler

import (
	"context"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/api/ratelimit"
	"github.com/strelov1/freehire/internal/identity/auth"
)

// TestPublicReadBudgets_ClearTheMeasuredLivePeaks guards the numbers themselves.
// A ceiling below what production already carries would not bound abuse; it would
// cut off a caller this change was designed to tolerate, and do it silently,
// since nothing else in the suite knows what live traffic looks like. The peaks
// are per-IP per-minute over a 4.6-hour window — see the change's design.md.
func TestPublicReadBudgets_ClearTheMeasuredLivePeaks(t *testing.T) {
	const (
		measuredAgentPeak = 184 // one third-party client, held steadily
		measuredReadPeak  = 258
	)
	if agentSearchPerMinute <= measuredAgentPeak {
		t.Errorf("agentSearchPerMinute = %d, must exceed the measured live peak of %d",
			agentSearchPerMinute, measuredAgentPeak)
	}
	if publicReadsPerMinute <= measuredReadPeak {
		t.Errorf("publicReadsPerMinute = %d, must exceed the measured live peak of %d",
			publicReadsPerMinute, measuredReadPeak)
	}
}

// oneShotThrottler admits the first request per key and refuses the rest, so a
// test can exhaust one budget in two calls without depending on the real ceiling.
type oneShotThrottler struct {
	mu   sync.Mutex
	seen map[string]bool
}

func newOneShotThrottler() *oneShotThrottler {
	return &oneShotThrottler{seen: make(map[string]bool)}
}

func (o *oneShotThrottler) Allow(_ context.Context, key string, limit int, window time.Duration) (ratelimit.Decision, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.seen[key] {
		return ratelimit.Decision{Limit: limit, ResetAfter: window, RetryAfter: window}, nil
	}
	o.seen[key] = true
	return ratelimit.Decision{Allowed: true, Limit: limit, Remaining: limit - 1, ResetAfter: window}, nil
}

// A public read is budgeted per IP even for a signed-in caller — the decision this
// records, in place of a comment that claimed the opposite while the code did this.
//
// The gate is mounted FIRST here, unlike production, so an authenticated caller really is
// authenticated by the time the limiter runs. That is what makes this a guard rather than
// a description: with a user-keyed budget it fails whatever the mounting order, and the
// defect it replaces was precisely a claim that only held in one order.
//
// The cost is stated rather than hidden: colleagues behind one office NAT share the
// allowance. They always did — every public read registers its limiter before the
// optional-auth gate, or has no gate at all, so `auth.UserID` never saw a user here.
func TestPublicReadLimiter_BudgetsBySourceAddressEvenForASignedInCaller(t *testing.T) {
	th := newOneShotThrottler()

	app := fiber.New(fiber.Config{ProxyHeader: fiber.HeaderXForwardedFor})
	signIn := func(id int64) fiber.Handler {
		return func(c *fiber.Ctx) error {
			c.Locals(auth.LocalsUserID, id)
			return c.Next()
		}
	}
	ok := func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) }
	// Gate before limiter, so the limiter sees a real authenticated caller.
	app.Get("/one", signIn(101), publicReadLimiter(th), ok)
	app.Get("/two", signIn(202), publicReadLimiter(th), ok)

	get := func(path string) int {
		req := httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, path, nil)
		req.Header.Set(fiber.HeaderXForwardedFor, "203.0.113.42")
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		defer resp.Body.Close()
		return resp.StatusCode
	}

	if got := get("/one"); got != fiber.StatusOK {
		t.Fatalf("first read = %d, want 200", got)
	}
	if got := get("/two"); got != fiber.StatusTooManyRequests {
		t.Errorf("a different signed-in user on the same address = %d, want 429 — the public read budget is per address", got)
	}
}

// TestPublicReadLimiters_DoNotShareABudget pins that the two classes are keyed
// apart. They exist as two budgets because they cost differently; one shared key
// would quietly undo the split and put facet lookups under the expensive
// endpoint's ceiling.
func TestPublicReadLimiters_DoNotShareABudget(t *testing.T) {
	th := newOneShotThrottler()

	app := fiber.New(fiber.Config{ProxyHeader: fiber.HeaderXForwardedFor})
	app.Get("/cheap", publicReadLimiter(th), func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) })
	app.Get("/agent", agentSearchLimiter(th), func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) })

	get := func(path string) int {
		req := httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, path, nil)
		req.Header.Set(fiber.HeaderXForwardedFor, "203.0.113.9")
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		defer resp.Body.Close()
		return resp.StatusCode
	}

	if got := get("/agent"); got != fiber.StatusOK {
		t.Fatalf("first /agent = %d, want 200", got)
	}
	if got := get("/agent"); got != fiber.StatusTooManyRequests {
		t.Fatalf("second /agent = %d, want 429 (its own budget is spent)", got)
	}
	if got := get("/cheap"); got != fiber.StatusOK {
		t.Errorf("/cheap = %d, want 200 — exhausting the agent budget must not spend the read budget", got)
	}
}

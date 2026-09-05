//go:build integration

// Integration tests for the store billing routes against a real Postgres and a stub
// RevenueCat: the sync route names only its caller, an unconfigured store mounts nothing, and
// /me/plan reports where a plan came from.
//
// Run with: go test -tags=integration ./internal/api/handler/
package handler

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/ai/plan"
	"github.com/strelov1/freehire/internal/api/ratelimit"
	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/identity/billing"
	"github.com/strelov1/freehire/internal/platform/db"
)

const storeSecret = "whsec_store_integration"

// revenuecatSignatureHeader is written out rather than imported, for the reason its Stripe
// counterpart above is: the name is a contract with RevenueCat, so a change to the constant
// should fail a test rather than be followed silently.
const revenuecatSignatureHeader = "X-RevenueCat-Webhook-Signature"

// countingThrottler enforces whatever limit the middleware was built with, per key.
//
// A real one rather than a stub that always refuses, because what is under test is the WIRING
// — that the route carries a limiter at all, keyed per caller, with the ceiling the handler
// declares. A nil throttler fails open by design, so a test without one would pass against a
// route that has no limiter.
type countingThrottler struct {
	mu   sync.Mutex
	seen map[string]int
}

func newCountingThrottler() *countingThrottler {
	return &countingThrottler{seen: make(map[string]int)}
}

func (c *countingThrottler) Allow(_ context.Context, key string, limit int, window time.Duration) (ratelimit.Decision, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.seen[key]++
	if c.seen[key] > limit {
		return ratelimit.Decision{Limit: limit, ResetAfter: window, RetryAfter: window}, nil
	}
	return ratelimit.Decision{Allowed: true, Limit: limit, Remaining: limit - c.seen[key], ResetAfter: window}, nil
}

func signStoreDelivery(body []byte, at time.Time) string {
	ts := at.Unix()
	mac := hmac.New(sha256.New, []byte(storeSecret))
	mac.Write([]byte(strconv.FormatInt(ts, 10) + "."))
	mac.Write(body)
	return fmt.Sprintf("t=%d,v1=%s", ts, hex.EncodeToString(mac.Sum(nil)))
}

// storeApp mounts the billing routes with a configured store provider and no Stripe, which is
// also the check that the two mount independently: every route exercised here exists in a
// deployment that sells only in the apps.
func storeApp(t *testing.T, pool *pgxpool.Pool, providerURL string, iss *auth.Issuer) *fiber.App {
	t.Helper()
	cfg := billing.RevenueCatConfig{APIKey: "sk_rc", WebhookSecret: storeSecret, Entitlement: "pro"}
	h := newBillingHandlers(billing.New(billing.Config{}, db.New(pool)),
		billing.NewRevenueCatWithBase(cfg, db.New(pool), providerURL), nil)

	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	h.register(app.Group("/api/v1"), middleware{
		cookie:    auth.RequireAuth(iss, testVersions),
		throttler: newCountingThrottler(),
	})
	return app
}

func TestStoreSyncNamesOnlyItsCaller(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()

	var caller, victim int64
	if err := pool.QueryRow(ctx, `INSERT INTO users (email) VALUES ('store-caller@example.test') RETURNING id`).Scan(&caller); err != nil {
		t.Fatalf("seed caller: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO users (email) VALUES ('store-victim@example.test') RETURNING id`).Scan(&victim); err != nil {
		t.Fatalf("seed victim: %v", err)
	}

	// NEITHER account is seeded with a store footprint, and that is the point rather than an
	// omission. A first-time buyer has none — that is what "first" means — so seeding one
	// would test the route only in the state it does not need to serve, which is how an
	// earlier draft passed green while refusing every first purchase whose webhook was lost.

	var asked atomic.Int64
	until := time.Now().UTC().Add(60 * 24 * time.Hour).Truncate(time.Second)
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked.Add(1)
		// Whichever subscriber is asked about, the answer entitles — so if the route honoured
		// a caller-supplied id, the victim's column would move and the assertion below would
		// see it.
		if !strings.HasSuffix(r.URL.Path, strconv.FormatInt(caller, 10)) {
			t.Errorf("the provider was asked about %s, want only the session's account %d", r.URL.Path, caller)
		}
		_, _ = w.Write([]byte(`{"subscriber":{"entitlements":{"pro":{"expires_date":"` + until.Format(time.RFC3339) + `"}}}}`))
	}))
	defer stub.Close()

	iss := auth.NewIssuer("test-secret", time.Hour)
	app := storeApp(t, pool, stub.URL, iss)

	post := func(t *testing.T, token, body string) int {
		t.Helper()
		req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/api/v1/billing/revenuecat/sync", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
		}
		resp, err := app.Test(req, 15000)
		if err != nil {
			t.Fatalf("request: %v", err)
		}
		_ = resp.Body.Close()
		return resp.StatusCode
	}

	t.Run("an anonymous caller is refused and the provider is never called", func(t *testing.T) {
		before := asked.Load()
		if status := post(t, "", `{}`); status != http.StatusUnauthorized {
			t.Fatalf("want 401, got %d", status)
		}
		if asked.Load() != before {
			t.Fatal("an unauthenticated request reached the provider")
		}
	})

	t.Run("a user id in the body is ignored", func(t *testing.T) {
		token, _ := iss.Issue(caller, testTokenVersion)
		if status := post(t, token, fmt.Sprintf(`{"user_id":%d}`, victim)); status != http.StatusOK {
			t.Fatalf("want 200, got %d", status)
		}

		var moved *time.Time
		if err := pool.QueryRow(ctx, `SELECT pro_until_revenuecat FROM users WHERE id = $1`, victim).Scan(&moved); err != nil {
			t.Fatalf("read the victim: %v", err)
		}
		if moved != nil && moved.UTC().Equal(until) {
			t.Fatal("the route wrote the plan of the account named in the body")
		}

		var mine *time.Time
		if err := pool.QueryRow(ctx, `SELECT pro_until_revenuecat FROM users WHERE id = $1`, caller).Scan(&mine); err != nil {
			t.Fatalf("read the caller: %v", err)
		}
		if mine == nil || !mine.UTC().Equal(until) {
			t.Fatalf("the caller's own plan is %v, want %v", mine, until)
		}
	})

	t.Run("repeated calls are bounded", func(t *testing.T) {
		token, _ := iss.Issue(caller, testTokenVersion)
		var limited bool
		// One more than the allowance: the budget is per caller and this test has already
		// spent one of it above, so the refusal must arrive inside this loop.
		for i := 0; i <= storeSyncLimit; i++ {
			if post(t, token, `{}`) == http.StatusTooManyRequests {
				limited = true
				break
			}
		}
		if !limited {
			t.Fatalf("more than %d syncs in a minute were allowed; this is the one route a caller can use to make us call a third party", storeSyncLimit)
		}
	})
}

// TestStoreRoutesAreAbsentWhenUnconfigured is the disabled behaviour: not a 404 from inside a
// handler, but a route that was never mounted.
func TestStoreRoutesAreAbsentWhenUnconfigured(t *testing.T) {
	pool := startPostgres(t)
	iss := auth.NewIssuer("test-secret", time.Hour)

	h := newBillingHandlers(billing.New(billing.Config{}, db.New(pool)),
		billing.NewRevenueCat(billing.RevenueCatConfig{}, db.New(pool)), nil)
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	h.register(app.Group("/api/v1"), middleware{cookie: auth.RequireAuth(iss, testVersions)})

	for _, path := range []string{"/api/v1/billing/revenuecat/webhook", "/api/v1/billing/revenuecat/sync"} {
		req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, path, strings.NewReader(`{}`))
		resp, err := app.Test(req, 15000)
		if err != nil {
			t.Fatalf("request: %v", err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("%s answered %d, want 404 from an unmounted route", path, resp.StatusCode)
		}
	}
}

// TestPlanNamesWhereProCameFrom covers the field a mobile client needs to decide whether to
// show a paywall at all: Apple forbids sending an in-app subscriber to the web to cancel, and
// selling Pro to a Stripe subscriber charges them twice for one plan.
func TestPlanNamesWhereProCameFrom(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	iss := auth.NewIssuer("test-secret", time.Hour)

	planH := newPlanHandlers(plan.NewStore(db.New(pool), pool, plan.DefaultConfig()), db.New(pool))
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	planH.register(app.Group("/api/v1"), middleware{key: auth.RequireAuth(iss, testVersions)})

	read := func(t *testing.T, userID int64) (string, bool) {
		t.Helper()
		token, _ := iss.Issue(userID, testTokenVersion)
		req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/api/v1/me/plan", nil)
		req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})

		resp, err := app.Test(req, 15000)
		if err != nil {
			t.Fatalf("request: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("want 200, got %d", resp.StatusCode)
		}

		var out struct {
			Data struct {
				ProSource string     `json:"pro_source"`
				ProUntil  *time.Time `json:"pro_until"`
			} `json:"data"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			t.Fatalf("decode: %v", err)
		}
		return out.Data.ProSource, out.Data.ProUntil != nil
	}

	seed := func(t *testing.T, email, columns string) int64 {
		t.Helper()
		var id int64
		if err := pool.QueryRow(ctx, `INSERT INTO users (email) VALUES ($1) RETURNING id`, email).Scan(&id); err != nil {
			t.Fatalf("seed %s: %v", email, err)
		}
		if columns != "" {
			if _, err := pool.Exec(ctx, `UPDATE users SET `+columns+` WHERE id = $1`, id); err != nil {
				t.Fatalf("seed columns for %s: %v", email, err)
			}
		}
		return id
	}

	t.Run("a store subscriber is identified as one", func(t *testing.T) {
		id := seed(t, "src-store@example.test", `pro_until_revenuecat = now() + interval '30 days'`)
		if source, hasUntil := read(t, id); source != "revenuecat" || !hasUntil {
			t.Fatalf("pro_source = %q (pro_until present: %v), want revenuecat", source, hasUntil)
		}
	})

	t.Run("the furthest source is the one named", func(t *testing.T) {
		id := seed(t, "src-both@example.test",
			`pro_until_stripe = now() + interval '2 days', pro_until_revenuecat = now() + interval '40 days'`)
		if source, _ := read(t, id); source != "revenuecat" {
			t.Fatalf("pro_source = %q, want the source equal to the derived pro_until", source)
		}
	})

	t.Run("a tie resolves in the stated order", func(t *testing.T) {
		id := seed(t, "src-tie@example.test", `pro_until_granted = now() + interval '5 days'`)
		if _, err := pool.Exec(ctx, `UPDATE users SET pro_until_stripe = pro_until_granted WHERE id = $1`, id); err != nil {
			t.Fatalf("seed the tie: %v", err)
		}
		if source, _ := read(t, id); source != "stripe" {
			t.Fatalf("pro_source = %q, want stripe — the tie-break order is stated so the answer is stable", source)
		}
	})

	t.Run("a free account carries no source", func(t *testing.T) {
		id := seed(t, "src-free@example.test", "")
		if source, hasUntil := read(t, id); source != "" || hasUntil {
			t.Fatalf("pro_source = %q (pro_until present: %v), want both absent", source, hasUntil)
		}
	})

	t.Run("a lapsed plan carries no source", func(t *testing.T) {
		id := seed(t, "src-lapsed@example.test", `pro_until_stripe = now() - interval '1 day'`)
		if source, hasUntil := read(t, id); source != "" || hasUntil {
			t.Fatalf("pro_source = %q (pro_until present: %v), want both absent for a plan that has ended", source, hasUntil)
		}
	})
}

// TestStoreWebhookRecordsAndApplies is the same contract the Stripe webhook keeps, over a
// different header and envelope: verify, record, acknowledge, apply.
func TestStoreWebhookRecordsAndApplies(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()

	var userID int64
	if err := pool.QueryRow(ctx, `INSERT INTO users (email) VALUES ('store-hook@example.test') RETURNING id`).Scan(&userID); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	until := time.Now().UTC().Add(45 * 24 * time.Hour).Truncate(time.Second)
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"subscriber":{"entitlements":{"pro":{"expires_date":"` + until.Format(time.RFC3339) + `"}}}}`))
	}))
	defer stub.Close()

	app := storeApp(t, pool, stub.URL, auth.NewIssuer("test-secret", time.Hour))
	body := []byte(fmt.Sprintf(`{"api_version":"1.0","event":{"id":"evt_http_1","type":"INITIAL_PURCHASE","app_user_id":"%d"}}`, userID))

	post := func(signature string) int {
		t.Helper()
		req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/api/v1/billing/revenuecat/webhook", strings.NewReader(string(body)))
		req.Header.Set("Content-Type", "application/json")
		if signature != "" {
			req.Header.Set(revenuecatSignatureHeader, signature)
		}
		resp, err := app.Test(req, 15000)
		if err != nil {
			t.Fatalf("request: %v", err)
		}
		_ = resp.Body.Close()
		return resp.StatusCode
	}

	if status := post(""); status != http.StatusUnauthorized {
		t.Fatalf("an unsigned store delivery answered %d, want 401", status)
	}
	if status := post(signStoreDelivery(body, time.Now())); status != http.StatusOK {
		t.Fatalf("a signed store delivery answered %d, want 200", status)
	}

	var got *time.Time
	if err := pool.QueryRow(ctx, `SELECT pro_until_revenuecat FROM users WHERE id = $1`, userID).Scan(&got); err != nil {
		t.Fatalf("read the column: %v", err)
	}
	if got == nil || !got.UTC().Equal(until) {
		t.Fatalf("pro_until_revenuecat = %v, want %v", got, until)
	}

	// A redelivery reuses the event id and must be acknowledged without recording twice.
	if status := post(signStoreDelivery(body, time.Now())); status != http.StatusOK {
		t.Fatalf("a store redelivery answered %d, want 200", status)
	}
	if n := countBillingEvents(t, pool); n != 1 {
		t.Fatalf("want 1 recorded event after a redelivery, got %d", n)
	}
}

// TestStoreSyncServesAFirstPurchase is the route's reason to exist, over HTTP.
//
// The buyer has no recorded delivery and a NULL source column. Every other recovery path
// skips them for exactly that reason, so if this route does too, a purchase whose first
// webhook was lost never confers anything.
func TestStoreSyncServesAFirstPurchase(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()

	var buyer int64
	if err := pool.QueryRow(ctx, `INSERT INTO users (email) VALUES ('first-buy@example.test') RETURNING id`).Scan(&buyer); err != nil {
		t.Fatalf("seed buyer: %v", err)
	}

	until := time.Now().UTC().Add(30 * 24 * time.Hour).Truncate(time.Second)
	var asked atomic.Int64
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		asked.Add(1)
		_, _ = w.Write([]byte(`{"subscriber":{"entitlements":{"pro":{"expires_date":"` + until.Format(time.RFC3339) + `"}}}}`))
	}))
	defer stub.Close()

	iss := auth.NewIssuer("test-secret", time.Hour)
	app := storeApp(t, pool, stub.URL, iss)

	token, _ := iss.Issue(buyer, testTokenVersion)
	req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/api/v1/billing/revenuecat/sync", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})

	resp, err := app.Test(req, 15000)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}

	if asked.Load() != 1 {
		t.Fatalf("the provider was asked %d times; a buyer with no footprint yet must still be read", asked.Load())
	}
	var got *time.Time
	if err := pool.QueryRow(ctx, `SELECT pro_until_revenuecat FROM users WHERE id = $1`, buyer).Scan(&got); err != nil {
		t.Fatalf("read the column: %v", err)
	}
	if got == nil || !got.UTC().Equal(until) {
		t.Fatalf("pro_until_revenuecat = %v, want %v", got, until)
	}
}

//go:build integration

// Integration tests for the billing routes against a real Postgres: a signed provider
// delivery is recorded and applied, a redelivery is acknowledged without doing the work
// twice, an unsigned one is refused, checkout mints a link carrying the caller's own id,
// and an unconfigured deployment has no billing routes at all. The provider points at a
// stub server so no real call is made.
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
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/identity/billing"
	"github.com/strelov1/freehire/internal/platform/db"
)

const billingSecret = "whsec_integration"

// stripeSignatureHeader is written out rather than read from the billing package, and that is
// the right way round for a test: the name is a contract with Stripe, not an internal choice,
// so a change to the constant should fail here rather than be followed silently.
const stripeSignatureHeader = "Stripe-Signature"

// signDelivery reproduces what the provider sends: HMAC-SHA256 over "<unix>.<raw body>".
func signDelivery(body []byte, at time.Time) string {
	ts := at.Unix()
	mac := hmac.New(sha256.New, []byte(billingSecret))
	mac.Write([]byte(strconv.FormatInt(ts, 10) + "."))
	mac.Write(body)
	return fmt.Sprintf("t=%d,v1=%s", ts, hex.EncodeToString(mac.Sum(nil)))
}

// billingApp mounts the billing routes exactly as Register does, so the disabled case
// exercises the real registration path rather than a reimplementation of it.
func billingApp(t *testing.T, pool *pgxpool.Pool, cfg billing.Config, providerURL string, iss *auth.Issuer) *fiber.App {
	t.Helper()
	// No store provider: these tests are about Stripe, and an unconfigured RevenueCat mounts
	// nothing — which is also what the disabled case below asserts about Stripe itself.
	h := newBillingHandlers(billing.NewWithBase(cfg, db.New(pool), providerURL), nil)
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	h.register(app.Group("/api/v1"), middleware{cookie: auth.RequireAuth(iss, testVersions)})
	return app
}

func enabledBillingConfig() billing.Config {
	return billing.Config{
		APIKey:        "sk_test",
		WebhookSecret: billingSecret,
		// Without a price nothing confers Pro, so the config reports itself disabled and no
		// route is mounted at all.
		Prices:  []string{"price_pro_monthly"},
		SiteURL: "https://freehire.me",
	}
}

// v2CustomerWithPro is what the provider returns for a subscriber who is entitled.
const customerWithPro = `{"object":"list","data":[{"status":"active","current_period_end":1790812800,"items":{"data":[{"price":{"id":"price_pro_monthly"}}]}}]}`

func TestBillingWebhook(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()

	var userID int64
	if err := pool.QueryRow(ctx, `INSERT INTO users (email) VALUES ('billing@example.test') RETURNING id`).Scan(&userID); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	var providerCalls int
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		providerCalls++
		_, _ = w.Write([]byte(customerWithPro))
	}))
	defer stub.Close()

	iss := auth.NewIssuer("test-secret", time.Hour)
	app := billingApp(t, pool, enabledBillingConfig(), stub.URL, iss)

	body := []byte(fmt.Sprintf(`{"id":"evt_int_1","type":"checkout.session.completed","data":{"object":{"customer":"cus_int_1","client_reference_id":"%d"}}}`, userID))

	// Returns the status code and closes the body: nothing here reads a webhook response,
	// and handing back an open body makes every call site owe a close.
	post := func(signature string) int {
		t.Helper()
		req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/api/v1/billing/stripe/webhook", strings.NewReader(string(body)))
		req.Header.Set("Content-Type", "application/json")
		if signature != "" {
			req.Header.Set(stripeSignatureHeader, signature)
		}
		resp, err := app.Test(req, 15000)
		if err != nil {
			t.Fatalf("request: %v", err)
		}
		_ = resp.Body.Close()
		return resp.StatusCode
	}

	t.Run("an unsigned delivery is refused and writes nothing", func(t *testing.T) {
		if status := post(""); status != http.StatusUnauthorized {
			t.Fatalf("want 401, got %d", status)
		}
		if n := countBillingEvents(t, pool); n != 0 {
			t.Fatalf("want nothing recorded, got %d rows", n)
		}
	})

	t.Run("a delivery signed with the wrong secret is refused", func(t *testing.T) {
		mac := hmac.New(sha256.New, []byte("whsec_wrong"))
		mac.Write(body)
		if status := post("t=" + strconv.FormatInt(time.Now().Unix(), 10) + ",v1=" + hex.EncodeToString(mac.Sum(nil))); status != http.StatusUnauthorized {
			t.Fatalf("want 401, got %d", status)
		}
	})

	t.Run("a stale signature is refused", func(t *testing.T) {
		if status := post(signDelivery(body, time.Now().Add(-time.Hour))); status != http.StatusUnauthorized {
			t.Fatalf("want 401 for a replayed delivery, got %d", status)
		}
	})

	t.Run("a signed delivery is recorded and applied", func(t *testing.T) {
		if status := post(signDelivery(body, time.Now())); status != http.StatusOK {
			t.Fatalf("want 200, got %d", status)
		}
		if n := countBillingEvents(t, pool); n != 1 {
			t.Fatalf("want 1 recorded event, got %d", n)
		}

		var proUntil *time.Time
		if err := pool.QueryRow(ctx, `SELECT pro_until FROM users WHERE id = $1`, userID).Scan(&proUntil); err != nil {
			t.Fatalf("read pro_until: %v", err)
		}
		if proUntil == nil {
			t.Fatal("the delivery did not apply the plan")
		}
		if want := "2026-10-01T00:00:00Z"; proUntil.UTC().Format(time.RFC3339) != want {
			t.Fatalf("want %s, got %s", want, proUntil.UTC().Format(time.RFC3339))
		}
	})

	t.Run("a redelivery is acknowledged and does no work twice", func(t *testing.T) {
		before := providerCalls

		if status := post(signDelivery(body, time.Now())); status != http.StatusOK {
			t.Fatalf("want 200 for a redelivery, got %d", status)
		}
		if n := countBillingEvents(t, pool); n != 1 {
			t.Fatalf("want still 1 recorded event, got %d", n)
		}
		// The provider retries anything it did not get a 200 for, reusing the event id.
		// Recognising that before doing anything is what keeps a retry storm cheap.
		if providerCalls != before {
			t.Fatalf("a redelivery called the provider %d extra times", providerCalls-before)
		}
	})
}

// TestBillingWebhookNeverDecodesTheBody is the reason this handler reads
// c.Request().Body() rather than Fiber's Ctx.Body().
//
// Ctx.Body() honours Content-Encoding and decompresses, chaining up to three layers. On the
// one unauthenticated POST in the app that is a hole with two sides. The bytes verified stop
// being the bytes sent, so what the HMAC covers is decided by a header the sender writes.
// And the decoding happens BEFORE the signature is checked, while the server's 8MB BodyLimit
// bounds only the compressed body — so a few megabytes of brotli from anyone at all become
// an unbounded allocation inside the process.
//
// The request below is what probing that looks like: a header claiming an encoding the body
// does not have. Reading raw, it is simply a signed delivery and verifies. Decoding first,
// the gunzip fails, the body becomes an error string, and the signature cannot match — so
// this test answers 401 against the implementation it was written to rule out.
func TestBillingWebhookNeverDecodesTheBody(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()

	var userID int64
	if err := pool.QueryRow(ctx, `INSERT INTO users (email) VALUES ('encoded@example.test') RETURNING id`).Scan(&userID); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(customerWithPro))
	}))
	defer stub.Close()

	iss := auth.NewIssuer("test-secret", time.Hour)
	app := billingApp(t, pool, enabledBillingConfig(), stub.URL, iss)

	body := []byte(fmt.Sprintf(`{"id":"evt_encoded","type":"checkout.session.completed","data":{"object":{"customer":"cus_encoded","client_reference_id":"%d"}}}`, userID))
	req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/api/v1/billing/stripe/webhook", strings.NewReader(string(body)))
	req.Header.Set(stripeSignatureHeader, signDelivery(body, time.Now()))
	// The lie. Nothing here is gzip, and nothing must try to treat it as gzip.
	req.Header.Set("Content-Encoding", "gzip")

	resp, err := app.Test(req, 15000)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200 — the signature covers the bytes as received, whatever Content-Encoding claims — got %d", resp.StatusCode)
	}
}

// TestBillingWebhookRejectsAMalformedBodyWithoutRetries separates the two refusals. A
// delivery that does not VERIFY is 401: nothing proves it came from the provider. A delivery
// that verifies and then does not PARSE is 400, because retrying it can only produce the
// same bytes — and an endpoint that answers a permanent failure with a retryable status is
// how the provider decides the endpoint is broken and disables it, taking the renewals with
// it.
func TestBillingWebhookRejectsAMalformedBodyWithoutRetries(t *testing.T) {
	pool := startPostgres(t)
	iss := auth.NewIssuer("test-secret", time.Hour)
	app := billingApp(t, pool, enabledBillingConfig(), "http://127.0.0.1:1", iss)

	// Correctly signed by the provider, and unreadable: no event id, so there is no
	// idempotency key and nothing to record.
	body := []byte(`{"not":"an event"}`)
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/v1/billing/stripe/webhook", strings.NewReader(string(body)))
	req.Header.Set(stripeSignatureHeader, signDelivery(body, time.Now()))

	resp, err := app.Test(req, 15000)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400 for a signed body we cannot parse, got %d", resp.StatusCode)
	}
	if n := countBillingEvents(t, pool); n != 0 {
		t.Fatalf("want nothing recorded for an unparseable delivery, got %d rows", n)
	}
}

// TestBillingWebhookAcknowledgesWhatItCannotApply is the split the whole design turns on: a
// 200 means the event is STORED, not that it was applied. The provider being unreachable
// must not make it retry something we already hold.
func TestBillingWebhookAcknowledgesWhatItCannotApply(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()

	var userID int64
	if err := pool.QueryRow(ctx, `INSERT INTO users (email) VALUES ('unreachable@example.test') RETURNING id`).Scan(&userID); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer stub.Close()

	iss := auth.NewIssuer("test-secret", time.Hour)
	app := billingApp(t, pool, enabledBillingConfig(), stub.URL, iss)

	body := []byte(fmt.Sprintf(`{"id":"evt_unreachable","type":"checkout.session.completed","data":{"object":{"customer":"cus_unreachable","client_reference_id":"%d"}}}`, userID))
	req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/api/v1/billing/stripe/webhook", strings.NewReader(string(body)))
	req.Header.Set(stripeSignatureHeader, signDelivery(body, time.Now()))

	resp, err := app.Test(req, 15000)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200 even though the apply failed, got %d", resp.StatusCode)
	}

	// Recorded, unapplied, and left where the reconciler will find it.
	var processedAt *time.Time
	if err := pool.QueryRow(ctx,
		`SELECT processed_at FROM billing_events WHERE event_id = 'evt_unreachable'`).Scan(&processedAt); err != nil {
		t.Fatalf("read event: %v", err)
	}
	if processedAt != nil {
		t.Fatal("an event that could not be applied must stay unprocessed")
	}
}

func TestBillingCheckout(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()

	var userID int64
	if err := pool.QueryRow(ctx, `INSERT INTO users (email) VALUES ('buyer@example.test') RETURNING id`).Scan(&userID); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	// The provider mints the checkout page; we only ever hand it the account id.
	var gotForm string
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		gotForm = r.Form.Encode()
		_, _ = w.Write([]byte(`{"url":"https://checkout.stripe.com/c/pay/cs_test_int"}`))
	}))
	defer stub.Close()

	iss := auth.NewIssuer("test-secret", time.Hour)
	app := billingApp(t, pool, enabledBillingConfig(), stub.URL, iss)

	t.Run("an anonymous caller gets no link", func(t *testing.T) {
		req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/api/v1/billing/checkout", nil)
		resp, err := app.Test(req, 15000)
		if err != nil {
			t.Fatalf("request: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("want 401, got %d", resp.StatusCode)
		}
	})

	t.Run("the link carries the caller's own id", func(t *testing.T) {
		token, _ := iss.Issue(userID, testTokenVersion)
		req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/api/v1/billing/checkout", nil)
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
				URL string `json:"url"`
			} `json:"data"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if out.Data.URL != "https://checkout.stripe.com/c/pay/cs_test_int" {
			t.Fatalf("want the provider's checkout page, got %q", out.Data.URL)
		}
		// The identifier the provider will echo back must be the SESSION's account, not
		// anything the request could have named. It is what decides who becomes Pro.
		if want := fmt.Sprintf("client_reference_id=%d", userID); !strings.Contains(gotForm, want) {
			t.Fatalf("form %q does not carry %q", gotForm, want)
		}
	})
}

// TestBillingRoutesAbsentWhenUnconfigured is the property that lets this ship in a public
// repository: a deployment that never sets the credentials cannot tell the endpoints are
// there. 404, not 401 and not 500.
func TestBillingRoutesAbsentWhenUnconfigured(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	iss := auth.NewIssuer("test-secret", time.Hour)
	app := billingApp(t, pool, billing.Config{}, "https://provider.invalid", iss)

	for _, tc := range []struct {
		method, path string
	}{
		{http.MethodPost, "/api/v1/billing/stripe/webhook"},
		{http.MethodGet, "/api/v1/billing/checkout"},
	} {
		req := httptest.NewRequestWithContext(ctx, tc.method, tc.path, strings.NewReader("{}"))
		resp, err := app.Test(req, 15000)
		if err != nil {
			t.Fatalf("%s %s: %v", tc.method, tc.path, err)
		}
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("%s %s: want 404, got %d", tc.method, tc.path, resp.StatusCode)
		}
		_ = resp.Body.Close()
	}
}

func countBillingEvents(t *testing.T, pool *pgxpool.Pool) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM billing_events`).Scan(&n); err != nil {
		t.Fatalf("count billing events: %v", err)
	}
	return n
}

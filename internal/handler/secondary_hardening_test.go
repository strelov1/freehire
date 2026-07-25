package handler

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/auth"
	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/telegramnotify"
)

// The webhook's secret is compared in constant time against the configured value. With an
// unset secret both sides are empty, and an equality check on two empty strings succeeds —
// so a misconfigured deployment would accept forged Bot API updates from anyone. The
// endpoint must refuse the request rather than trust an empty expectation.
func TestTelegramWebhook_RefusesWhenTheSecretIsUnset(t *testing.T) {
	h := &API{
		telegramLinks:         telegramnotify.NewLinkTokens("jwt-secret", time.Minute),
		telegramBot:           telegramnotify.NewClient("bot-token"),
		telegramWebhookSecret: "", // misconfigured deployment
	}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Post("/api/v1/telegram/webhook", h.TelegramWebhook)

	resp, err := app.Test(httptest.NewRequest(fiber.MethodPost, "/api/v1/telegram/webhook", nil))
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == fiber.StatusOK {
		t.Fatal("an unauthenticated update was accepted while the secret was unset (fail-open)")
	}
}

// requestOrigin must trust the request Host only when it names a host this deployment
// actually serves. Suffix-matching a cookie domain lets any subdomain — including a
// hijacked or third-party-hosted one — steer the OAuth redirect.
func TestRequestOrigin_OnlyTrustsAnExactServedHost(t *testing.T) {
	h := &API{
		frontendOrigin: "https://freehire.me",
		cookieDomains:  []string{"freehire.me"},
		servedHosts:    []string{"freehire.me", "apply.freehire.me"},
	}

	cases := []struct {
		host, want string
	}{
		{"apply.freehire.me", "https://apply.freehire.me"},
		{"freehire.me", "https://freehire.me"},
		{"takeover.freehire.me", "https://freehire.me"},
		{"evil.example", "https://freehire.me"},
	}
	for _, tc := range cases {
		if got := originForHost(t, h, tc.host); got != tc.want {
			t.Errorf("Host %q → origin %q, want %q", tc.host, got, tc.want)
		}
	}
}

// With no SERVED_HOSTS configured the canonical frontend origin's own host is still
// honoured, so a deployment that never sets the variable keeps working.
func TestRequestOrigin_DefaultsToTheFrontendHost(t *testing.T) {
	h := &API{
		frontendOrigin: "https://freehire.me",
		servedHosts:    servedHostsOrDefault(nil, "https://freehire.me"),
	}
	if got := originForHost(t, h, "freehire.me"); got != "https://freehire.me" {
		t.Errorf("origin = %q, want the frontend host to be served by default", got)
	}
}

// originForHost runs requestOrigin for one request Host.
func originForHost(t *testing.T, h *API, host string) string {
	t.Helper()
	app := fiber.New()
	app.Get("/origin", func(c *fiber.Ctx) error { return c.SendString(h.requestOrigin(c)) })

	req := httptest.NewRequest(fiber.MethodGet, "/origin", nil)
	req.Host = host
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	defer resp.Body.Close()
	buf := make([]byte, 256)
	n, _ := resp.Body.Read(buf)
	return string(buf[:n])
}

// An unrecognized link makes the server fetch an attacker-chosen URL, so the endpoint is
// an outbound-fetch amplifier unless it is bounded. The bound must be keyed on the
// authenticated user: keying on client IP would let a rotating proxy pool lift it.
func TestContributionLimiter_IsKeyedOnTheUser(t *testing.T) {
	mw := contributionLimiter()

	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	// Stand in for the auth middleware: the user id comes from a header so one test
	// can act as two users from the same address.
	app.Post("/contrib", func(c *fiber.Ctx) error {
		id := int64(1)
		if c.Get("X-Test-User") == "2" {
			id = 2
		}
		c.Locals("auth.userID", id)
		return c.Next()
	}, mw, func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusCreated) })

	post := func(user string) int {
		req := httptest.NewRequest(fiber.MethodPost, "/contrib", nil)
		req.Header.Set("X-Test-User", user)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("Test: %v", err)
		}
		defer resp.Body.Close()
		return resp.StatusCode
	}

	var refused int
	for i := 0; i < contributionsPerHour+5; i++ {
		if post("1") == fiber.StatusTooManyRequests {
			refused++
		}
	}
	if refused == 0 {
		t.Fatalf("user 1 sent %d submissions without being throttled", contributionsPerHour+5)
	}

	// A different user is unaffected by the first one's exhausted budget.
	if got := post("2"); got == fiber.StatusTooManyRequests {
		t.Error("a second user was throttled by the first user's budget — the key is not the user")
	}
}

// The tailoring credential is handed to an external agent, so it must be minted narrow.
func TestMintTailoringKey_IsScopedToTheCVSurface(t *testing.T) {
	var minted db.CreateAPIKeyParams
	recorder := apiKeyMinterFunc(func(_ context.Context, arg db.CreateAPIKeyParams) (db.CreateAPIKeyRow, error) {
		minted = arg
		return db.CreateAPIKeyRow{}, nil
	})

	if _, err := mintTailoringKey(context.Background(), recorder, 7, time.Now()); err != nil {
		t.Fatalf("mintTailoringKey: %v", err)
	}
	if minted.Scope != auth.ScopeCV {
		t.Errorf("minted scope = %q, want %q so a leaked tailoring key cannot reach the rest of the account",
			minted.Scope, auth.ScopeCV)
	}
}

// apiKeyMinterFunc adapts a function to the apiKeyMinter port.
type apiKeyMinterFunc func(context.Context, db.CreateAPIKeyParams) (db.CreateAPIKeyRow, error)

func (f apiKeyMinterFunc) CreateAPIKey(ctx context.Context, arg db.CreateAPIKeyParams) (db.CreateAPIKeyRow, error) {
	return f(ctx, arg)
}

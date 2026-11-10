//go:build integration

package handler

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/identity/accounts"
	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/identity/auth/mobileauth"
	"github.com/strelov1/freehire/internal/identity/auth/oauth"
	"github.com/strelov1/freehire/internal/platform/db"
)

// A banned account is refused by every OAuth sign-in leg, and a live account is not: each
// refusal is paired with the same request succeeding for an account in good standing, so a
// leg that simply failed for another reason cannot pass.
func TestAccountBanOAuth(t *testing.T) {
	ctx := context.Background()
	pool := startPostgres(t)
	queries := db.New(pool)
	svc := accounts.New(accounts.NewQueriesRepository(queries, pool), authHasher{})
	provider := &fakeProvider{name: "google"}
	store := mobileauth.NewStore(pool)
	h := &authHandlers{
		queries: queries, issuer: auth.NewIssuer("test-secret", time.Hour), accounts: svc,
		oauth: fakeRegistry{"google": provider}, oauthCodes: oauth.NewCodeStore(time.Minute),
		mobileAuth: store, mobileCallbacks: map[string]string{"ios": "freehire-test://cb"},
		frontendOrigin: "http://app.example",
	}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/api/v1/auth/oauth/:provider/callback", h.OAuthCallback)
	app.Post("/api/v1/auth/oauth/exchange", h.OAuthExchange)
	app.Get("/api/v1/auth/v2/oauth/:provider/callback", h.OAuthCallbackV2)
	app.Post("/api/v1/auth/v2/oauth/exchange", h.OAuthExchangeV2)

	var actorID int64
	if err := pool.QueryRow(ctx, `INSERT INTO users (email, role) VALUES ('oauth-mod@example.test', 'moderator') RETURNING id`).Scan(&actorID); err != nil {
		t.Fatal(err)
	}
	seed := func(email string, banned bool) int64 {
		var id int64
		if err := pool.QueryRow(ctx, `INSERT INTO users (email, email_verified) VALUES ($1, true) RETURNING id`, email).Scan(&id); err != nil {
			t.Fatalf("seed %s: %v", email, err)
		}
		if banned {
			if _, err := queries.BanUser(ctx, db.BanUserParams{ID: id, Reason: pgtype.Text{String: "spam", Valid: true}, ActorID: pgtype.Int8{Int64: actorID, Valid: true}}); err != nil {
				t.Fatalf("ban %s: %v", email, err)
			}
		}
		return id
	}
	bannedID := seed("oauth-banned@example.test", true)
	seed("oauth-live@example.test", false)

	type reply struct {
		StatusCode int
		Location   string
		Cookies    []*http.Cookie
	}
	send := func(method, path, body string, cookies ...string) reply {
		req := httptest.NewRequestWithContext(ctx, method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		for _, c := range cookies {
			req.Header.Add("Cookie", c)
		}
		resp, err := app.Test(req, 10_000)
		if err != nil {
			t.Fatalf("%s %s: %v", method, path, err)
		}
		_ = resp.Body.Close()
		return reply{StatusCode: resp.StatusCode, Location: resp.Header.Get("Location"), Cookies: resp.Cookies()}
	}
	hasSession := func(resp reply) bool {
		for _, c := range resp.Cookies {
			if c.Name == auth.CookieName && c.Value != "" {
				return true
			}
		}
		return false
	}
	as := func(email string) {
		provider.identity = oauth.Identity{ProviderUserID: "g-" + email, Email: email, EmailVerified: true}
	}

	t.Run("web callback", func(t *testing.T) {
		state := oauth.StateCookieName + "=s1"
		as("oauth-banned@example.test")
		resp := send(fiber.MethodGet, "/api/v1/auth/oauth/google/callback?state=s1&code=c", "", state)
		if loc := resp.Location; !strings.Contains(loc, "auth_error=oauth") || hasSession(resp) {
			t.Errorf("banned: Location %q, session %v; want auth_error and no session", loc, hasSession(resp))
		}
		as("oauth-live@example.test")
		resp = send(fiber.MethodGet, "/api/v1/auth/oauth/google/callback?state=s1&code=c", "", state)
		if !hasSession(resp) {
			t.Errorf("live account got no session (Location %q)", resp.Location)
		}
	})

	t.Run("mobile callback mints no code", func(t *testing.T) {
		cookies := []string{oauth.StateCookieName + "=s2", oauth.PlatformCookieName + "=" + oauth.PlatformMobile}
		as("oauth-banned@example.test")
		resp := send(fiber.MethodGet, "/api/v1/auth/oauth/google/callback?state=s2&code=c", "", cookies...)
		if loc := resp.Location; !strings.HasSuffix(loc, "?auth_error=oauth") {
			t.Errorf("banned: Location %q, want the generic auth_error", loc)
		}
		as("oauth-live@example.test")
		resp = send(fiber.MethodGet, "/api/v1/auth/oauth/google/callback?state=s2&code=c", "", cookies...)
		if loc := resp.Location; !strings.Contains(loc, "?code=") {
			t.Errorf("live: Location %q, want a one-time code", loc)
		}
	})

	t.Run("mobile exchange of a code already minted", func(t *testing.T) {
		code, err := h.oauthCodes.Mint(bannedID)
		if err != nil {
			t.Fatal(err)
		}
		resp := send(fiber.MethodPost, "/api/v1/auth/oauth/exchange", fmt.Sprintf(`{"code":%q}`, code))
		if resp.StatusCode != fiber.StatusForbidden || hasSession(resp) {
			t.Errorf("status %d, session %v; want 403 and no session", resp.StatusCode, hasSession(resp))
		}
	})

	const verifier = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	attempt := func() (*http.Cookie, string) {
		a, state, err := store.CreateBrowserAttempt(ctx, "google", "ios", "ios", "sign_in", mobileauth.Challenge(verifier), nil)
		if err != nil {
			t.Fatalf("CreateBrowserAttempt: %v", err)
		}
		_ = a
		return &http.Cookie{Name: oauthV2StateCookieName, Value: state}, state
	}

	t.Run("v2 callback mints no code", func(t *testing.T) {
		c, state := attempt()
		as("oauth-banned@example.test")
		resp := send(fiber.MethodGet, "/api/v1/auth/v2/oauth/google/callback?state="+state+"&code=c", "", c.String())
		if loc := resp.Location; !strings.Contains(loc, "auth_error=account_suspended") {
			t.Errorf("banned: Location %q, want auth_error=account_suspended", loc)
		}
		c, state = attempt()
		as("oauth-live@example.test")
		resp = send(fiber.MethodGet, "/api/v1/auth/v2/oauth/google/callback?state="+state+"&code=c", "", c.String())
		if loc := resp.Location; !strings.Contains(loc, "code=") {
			t.Errorf("live: Location %q, want a code", loc)
		}
	})

	t.Run("v2 exchange of a code already minted", func(t *testing.T) {
		a, _, err := store.CreateBrowserAttempt(ctx, "google", "ios", "ios", "sign_in", mobileauth.Challenge(verifier), nil)
		if err != nil {
			t.Fatal(err)
		}
		code, err := store.MintCode(ctx, a, bannedID)
		if err != nil {
			t.Fatal(err)
		}
		resp := send(fiber.MethodPost, "/api/v1/auth/v2/oauth/exchange", fmt.Sprintf(`{"code":%q,"code_verifier":%q}`, code, verifier))
		if resp.StatusCode != fiber.StatusForbidden || hasSession(resp) {
			t.Errorf("status %d, session %v; want 403 and no session", resp.StatusCode, hasSession(resp))
		}
	})
}

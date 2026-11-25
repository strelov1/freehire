//go:build integration

package handler

import (
	"context"
	"fmt"
	"io"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/identity/auth/mobileauth"
	"github.com/strelov1/freehire/internal/identity/auth/oauth"
	"github.com/strelov1/freehire/internal/identity/auth/recentauth"
	"github.com/strelov1/freehire/internal/platform/db"
)

// The "link" purpose end to end: callback, then exchange in the signed-in browser. The
// provider answers with an email belonging to nobody here, so any path that resolved the
// account by email (h.accounts is nil) would panic rather than pass.
func TestOAuthLinkV2(t *testing.T) {
	ctx := context.Background()
	pool := startPostgres(t)
	issuer := auth.NewIssuer("test-secret", time.Hour)
	provider := &fakeProvider{name: "github"}
	store := mobileauth.NewStore(pool)
	recent := recentauth.NewStore(pool, time.Minute)
	h := &authHandlers{
		queries: db.New(pool), issuer: issuer, oauth: fakeRegistry{"github": provider},
		mobileAuth: store, recentAuth: recent,
		mobileCallbacks: map[string]string{"web": "http://app.example/my/reauth"},
		frontendOrigin:  "http://app.example",
	}
	var signedIn int64
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Use(func(c *fiber.Ctx) error {
		if signedIn != 0 {
			c.Locals(auth.LocalsUserID, signedIn)
		}
		return c.Next()
	})
	app.Get("/cb/:provider", h.OAuthCallbackV2)
	app.Post("/exchange", h.OAuthExchangeV2)

	seed := func(email string) int64 {
		var id int64
		if err := pool.QueryRow(ctx, `INSERT INTO users (email, email_verified) VALUES ($1, true) RETURNING id`, email).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	alice := seed("link-alice@example.test")
	bob := seed("link-bob@example.test")

	send := func(method, path, body string, cookies ...string) (int, string, string) {
		req := httptest.NewRequestWithContext(ctx, method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		for _, c := range cookies {
			req.Header.Add("Cookie", c)
		}
		resp, err := app.Test(req, 10_000)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, resp.Header.Get("Location"), string(b)
	}
	session := func(uid int64) (string, []byte) {
		tok, err := issuer.Issue(uid, 1)
		if err != nil {
			t.Fatal(err)
		}
		hash, err := issuer.SessionFingerprint(tok)
		if err != nil {
			t.Fatal(err)
		}
		return auth.CookieName + "=" + tok, hash
	}
	const verifier = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	// link starts an attempt bound to uid's session, lets the provider answer subject, and
	// exchanges the code from exchangeCookie's session (uid's own unless overridden).
	link := func(uid int64, subject string, withProof bool, exchangeCookie string) (int, string) {
		t.Helper()
		signedIn = uid
		cookie, hash := session(uid)
		if exchangeCookie == "" {
			exchangeCookie = cookie
		}
		_, state, err := store.CreateBrowserAttempt(ctx, "github", "web", "web", "link",
			mobileauth.Challenge(verifier), &mobileauth.Binding{UserID: uid, TokenVersion: 1, SessionHash: hash})
		if err != nil {
			t.Fatal(err)
		}
		provider.identity = oauth.Identity{ProviderUserID: subject, Email: "stranger@example.test", EmailVerified: true}
		_, loc, _ := send(fiber.MethodGet, "/cb/github?state="+url.QueryEscape(state)+"&code=c", "", oauthV2StateCookieName+"="+state)
		u, err := url.Parse(loc)
		if err != nil || u.Query().Get("code") == "" {
			t.Fatalf("callback Location %q, want a code", loc)
		}
		cookies := []string{exchangeCookie}
		if withProof {
			raw, _, err := recent.Issue(ctx, uid, 1, hash)
			if err != nil {
				t.Fatal(err)
			}
			cookies = append(cookies, recentauth.CookieName+"="+raw)
		}
		status, _, body := send(fiber.MethodPost, "/exchange",
			fmt.Sprintf(`{"code":%q,"code_verifier":%q}`, u.Query().Get("code"), verifier), cookies...)
		return status, body
	}
	owner := func(subject string) int64 {
		var id int64
		_ = pool.QueryRow(ctx, `SELECT user_id FROM user_identities WHERE provider='github' AND provider_user_id=$1`, subject).Scan(&id)
		return id
	}
	var usersBefore int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM users`).Scan(&usersBefore)

	t.Run("links to the signed-in account whatever the provider email", func(t *testing.T) {
		status, body := link(alice, "gh-1", true, "")
		if status != fiber.StatusOK || !strings.Contains(body, `"provider":"github"`) {
			t.Fatalf("status %d body %s", status, body)
		}
		if owner("gh-1") != alice {
			t.Fatal("identity not attached to the signed-in account")
		}
		var usersAfter int
		_ = pool.QueryRow(ctx, `SELECT count(*) FROM users`).Scan(&usersAfter)
		if usersAfter != usersBefore {
			t.Fatalf("linking created an account: %d -> %d", usersBefore, usersAfter)
		}
	})

	t.Run("without a recent-auth proof", func(t *testing.T) {
		if status, body := link(bob, "gh-3", false, ""); status != 428 || !strings.Contains(body, "recent_auth_required") {
			t.Fatalf("status %d body %s", status, body)
		}
		if owner("gh-3") != 0 {
			t.Fatal("refused link wrote an identity")
		}
	})

	t.Run("identity of another account", func(t *testing.T) {
		if status, body := link(bob, "gh-1", true, ""); status != fiber.StatusConflict || !strings.Contains(body, "identity_in_use") {
			t.Fatalf("status %d body %s", status, body)
		}
		if owner("gh-1") != alice {
			t.Fatal("identity moved")
		}
	})

	t.Run("second identity of the provider", func(t *testing.T) {
		if status, body := link(alice, "gh-2", true, ""); status != fiber.StatusConflict || !strings.Contains(body, "provider_already_linked") {
			t.Fatalf("status %d body %s", status, body)
		}
	})

	t.Run("session changed before the exchange", func(t *testing.T) {
		other, _ := session(bob)
		if status, _ := link(bob, "gh-4", true, other); status != fiber.StatusUnauthorized {
			t.Fatalf("status %d, want 401", status)
		}
		if owner("gh-4") != 0 {
			t.Fatal("link from another session wrote an identity")
		}
	})
}

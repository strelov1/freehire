//go:build integration

// Integration tests for the push-token HTTP flow against a real Postgres:
// register upserts and reassigns by token value, unregister is owner-scoped,
// and the self-test endpoint reports per-device outcomes. Run with:
// go test -tags=integration ./internal/handler/
package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/auth"
	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/pushnotify"
)

// fakeSendNotifier stands in for a real Expo round trip: it returns a
// scripted outcome per token, so the self-test endpoint's per-outcome
// counting can be verified without a network call.
type fakeSendNotifier struct {
	outcomes map[string]error // token -> error to return (nil = "sent")
}

func (n *fakeSendNotifier) Send(ctx context.Context, token, title, body string, data map[string]string) error {
	return n.outcomes[token]
}

var errUnreachableTestExpo = errors.New("expo unreachable (test)")

func TestPushTokensEndToEnd(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()

	var aliceID, bobID int64
	if err := pool.QueryRow(ctx, `INSERT INTO users (email, email_verified) VALUES ('alice-pt@example.test', true) RETURNING id`).Scan(&aliceID); err != nil {
		t.Fatalf("seed alice: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO users (email, email_verified) VALUES ('bob-pt@example.test', true) RETURNING id`).Scan(&bobID); err != nil {
		t.Fatalf("seed bob: %v", err)
	}

	iss := auth.NewIssuer("test-secret", time.Hour)
	aliceCookie, _ := iss.Issue(aliceID, testTokenVersion)
	bobCookie, _ := iss.Issue(bobID, testTokenVersion)
	queries := db.New(pool)
	h := &authHandlers{queries: queries}

	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Post("/api/v1/me/push-tokens", auth.RequireAuth(iss, testVersions), h.RegisterPushToken)
	app.Get("/api/v1/me/push-tokens", auth.RequireAuth(iss, testVersions), h.ListPushTokens)
	app.Delete("/api/v1/me/push-tokens", auth.RequireAuth(iss, testVersions), h.UnregisterPushToken)
	app.Post("/api/v1/me/push-tokens/test", auth.RequireAuth(iss, testVersions), h.TestPushToken)

	cookieReq := func(method, path, cookie string, body []byte) *http.Request {
		var r *http.Request
		if body != nil {
			r = httptest.NewRequest(method, path, bytes.NewReader(body))
			r.Header.Set("Content-Type", "application/json")
		} else {
			r = httptest.NewRequest(method, path, nil)
		}
		r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: cookie})
		return r
	}

	const token = "ExponentPushToken[http-flow]"

	t.Run("register creates a row for the caller", func(t *testing.T) {
		resp, err := app.Test(cookieReq(fiber.MethodPost, "/api/v1/me/push-tokens", aliceCookie,
			[]byte(`{"token":"`+token+`","platform":"ios"}`)))
		if err != nil {
			t.Fatalf("register: %v", err)
		}
		if resp.StatusCode != fiber.StatusNoContent {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("register status = %d, want 204 (body %s)", resp.StatusCode, body)
		}

		var owner int64
		if err := pool.QueryRow(ctx, `SELECT user_id FROM user_push_tokens WHERE token = $1`, token).Scan(&owner); err != nil {
			t.Fatalf("query token: %v", err)
		}
		if owner != aliceID {
			t.Errorf("owner = %d, want alice (%d)", owner, aliceID)
		}
	})

	t.Run("registering the same token under bob reassigns it", func(t *testing.T) {
		resp, err := app.Test(cookieReq(fiber.MethodPost, "/api/v1/me/push-tokens", bobCookie,
			[]byte(`{"token":"`+token+`","platform":"ios"}`)))
		if err != nil {
			t.Fatalf("register(bob): %v", err)
		}
		if resp.StatusCode != fiber.StatusNoContent {
			t.Fatalf("register(bob) status = %d, want 204", resp.StatusCode)
		}

		var owner int64
		if err := pool.QueryRow(ctx, `SELECT user_id FROM user_push_tokens WHERE token = $1`, token).Scan(&owner); err != nil {
			t.Fatalf("query token: %v", err)
		}
		if owner != bobID {
			t.Errorf("owner = %d, want bob (%d) after reassignment", owner, bobID)
		}
	})

	// Runs after the reassignment above, which is exactly the state worth
	// asserting: one token, one owner. A list that still showed it to alice
	// would mean the app's toggle reads "on" for a device she no longer has.
	t.Run("list is owner-scoped", func(t *testing.T) {
		// Decoded into a local shape rather than pushTokenResponse: this test is
		// about who sees which device, not about how a timestamp round-trips.
		type device struct {
			Token    string `json:"token"`
			Platform string `json:"platform"`
		}
		listFor := func(cookie string) []device {
			t.Helper()
			resp, err := app.Test(cookieReq(fiber.MethodGet, "/api/v1/me/push-tokens", cookie, nil))
			if err != nil {
				t.Fatalf("list: %v", err)
			}
			if resp.StatusCode != fiber.StatusOK {
				body, _ := io.ReadAll(resp.Body)
				t.Fatalf("list status = %d, want 200 (body %s)", resp.StatusCode, body)
			}
			var out struct {
				Data []device `json:"data"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
				t.Fatalf("decode list: %v", err)
			}
			return out.Data
		}

		if got := listFor(aliceCookie); len(got) != 0 {
			t.Errorf("alice's devices = %d, want 0 — the token was reassigned to bob", len(got))
		}

		bobDevices := listFor(bobCookie)
		if len(bobDevices) != 1 {
			t.Fatalf("bob's devices = %d, want 1", len(bobDevices))
		}
		if bobDevices[0].Token != token {
			t.Errorf("token = %q, want %q", bobDevices[0].Token, token)
		}
		if bobDevices[0].Platform != "ios" {
			t.Errorf("platform = %q, want ios", bobDevices[0].Platform)
		}
	})

	t.Run("unregister is owner-scoped", func(t *testing.T) {
		aliceResp, err := app.Test(cookieReq(fiber.MethodDelete, "/api/v1/me/push-tokens", aliceCookie,
			[]byte(`{"token":"`+token+`"}`)))
		if err != nil {
			t.Fatalf("unregister(alice): %v", err)
		}
		if aliceResp.StatusCode != fiber.StatusNotFound {
			t.Errorf("unregister(alice) status = %d, want 404 — alice no longer owns this token", aliceResp.StatusCode)
		}

		bobResp, err := app.Test(cookieReq(fiber.MethodDelete, "/api/v1/me/push-tokens", bobCookie,
			[]byte(`{"token":"`+token+`"}`)))
		if err != nil {
			t.Fatalf("unregister(bob): %v", err)
		}
		if bobResp.StatusCode != fiber.StatusNoContent {
			t.Errorf("unregister(bob) status = %d, want 204", bobResp.StatusCode)
		}
	})

	t.Run("self-test reports per-device outcomes", func(t *testing.T) {
		ok := "ExponentPushToken[ok]"
		dead := "ExponentPushToken[dead]"
		broken := "ExponentPushToken[broken]"
		for _, tok := range []string{ok, dead, broken} {
			if _, err := queries.UpsertPushToken(ctx, db.UpsertPushTokenParams{
				UserID: aliceID, Token: tok, Platform: "ios",
			}); err != nil {
				t.Fatalf("seed token %s: %v", tok, err)
			}
		}

		h.pushNotifier = &fakeSendNotifier{outcomes: map[string]error{
			ok:     nil,
			dead:   pushnotify.ErrTokenPruned,
			broken: errUnreachableTestExpo,
		}}

		resp, err := app.Test(cookieReq(fiber.MethodPost, "/api/v1/me/push-tokens/test", aliceCookie, nil))
		if err != nil {
			t.Fatalf("test-send: %v", err)
		}
		if resp.StatusCode != fiber.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("test-send status = %d, want 200 (body %s)", resp.StatusCode, body)
		}
		var out struct {
			Data struct {
				Devices int `json:"devices"`
				Sent    int `json:"sent"`
				Pruned  int `json:"pruned"`
				Failed  int `json:"failed"`
			} `json:"data"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if out.Data.Devices != 3 || out.Data.Sent != 1 || out.Data.Pruned != 1 || out.Data.Failed != 1 {
			t.Errorf("outcome = %+v, want devices=3 sent=1 pruned=1 failed=1", out.Data)
		}
	})

	t.Run("self-test with no registered tokens reports zero without error", func(t *testing.T) {
		if _, err := pool.Exec(ctx, `DELETE FROM user_push_tokens WHERE user_id = $1`, bobID); err != nil {
			t.Fatalf("clear bob's tokens: %v", err)
		}
		resp, err := app.Test(cookieReq(fiber.MethodPost, "/api/v1/me/push-tokens/test", bobCookie, nil))
		if err != nil {
			t.Fatalf("test-send: %v", err)
		}
		if resp.StatusCode != fiber.StatusOK {
			t.Fatalf("test-send status = %d, want 200", resp.StatusCode)
		}
		var out struct {
			Data struct {
				Devices int `json:"devices"`
			} `json:"data"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if out.Data.Devices != 0 {
			t.Errorf("devices = %d, want 0", out.Data.Devices)
		}
	})
}

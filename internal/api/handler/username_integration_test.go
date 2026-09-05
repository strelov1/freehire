//go:build integration

// Integration tests for the account-username HTTP flow against a real Postgres:
// availability check, reading the caller's own state, claiming, rejecting a taken
// or reserved name, and the 30-day change cooldown. Run with:
// go test -tags=integration ./internal/api/handler/
package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/identity/accounts"
	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/platform/db"
)

func newUsernameTestApp(t *testing.T) (*fiber.App, *auth.Issuer, func() (int64, string)) {
	t.Helper()
	pool := startPostgres(t)
	h := &authHandlers{
		accounts: accounts.New(accounts.NewQueriesRepository(db.New(pool), pool), authHasher{}),
	}
	iss := auth.NewIssuer("test-secret-that-is-long-enough-0002", time.Hour)
	ra := auth.RequireAuth(iss, testVersions)

	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/api/v1/username/check", h.CheckUsername)
	app.Get("/api/v1/me/username", ra, h.GetUsername)
	app.Put("/api/v1/me/username", ra, h.ClaimUsername)

	var nextEmail int
	seedUser := func() (int64, string) {
		t.Helper()
		nextEmail++
		var userID int64
		if err := pool.QueryRow(context.Background(),
			`INSERT INTO users (email) VALUES ($1) RETURNING id`,
			"u"+strconv.Itoa(nextEmail)+"@example.test").Scan(&userID); err != nil {
			t.Fatalf("seed user: %v", err)
		}
		cookie, _ := iss.Issue(userID, testTokenVersion)
		return userID, cookie
	}
	return app, iss, seedUser
}

func TestUsernameHTTPFlow(t *testing.T) {
	app, _, seedUser := newUsernameTestApp(t)
	_, cookie := seedUser()

	do := func(method, path, cookieVal, body string) (int, map[string]any) {
		var r *http.Request
		if body != "" {
			r = httptest.NewRequestWithContext(context.Background(), method, path, strings.NewReader(body))
			r.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
		} else {
			r = httptest.NewRequestWithContext(context.Background(), method, path, nil)
		}
		if cookieVal != "" {
			r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: cookieVal})
		}
		resp, err := app.Test(r, -1)
		if err != nil {
			t.Fatalf("%s %s: %v", method, path, err)
		}
		var respBody map[string]any
		return decodeJSON(t, resp, &respBody), respBody
	}

	// Nobody has claimed "ivan-petrov" yet.
	if code, body := do("GET", "/api/v1/username/check?value=ivan-petrov", "", ""); code != 200 {
		t.Fatalf("check status: %d", code)
	} else if d, _ := body["data"].(map[string]any); d["available"] != true {
		t.Errorf("check available = %v, want true", body["data"])
	}

	// Before claiming, the caller has no username.
	if code, body := do("GET", "/api/v1/me/username", cookie, ""); code != 200 {
		t.Fatalf("get status: %d", code)
	} else if d, _ := body["data"].(map[string]any); d["username"] != nil {
		t.Errorf("initial username = %v, want null", body["data"])
	}

	// Claim it.
	if code, body := do("PUT", "/api/v1/me/username", cookie, `{"username":"ivan-petrov"}`); code != 200 {
		t.Fatalf("claim status: %d, body: %v", code, body)
	} else if d, _ := body["data"].(map[string]any); d["username"] != "ivan-petrov" {
		t.Errorf("claim response username = %v, want %q", body["data"], "ivan-petrov")
	}

	// Now the name is taken.
	if code, body := do("GET", "/api/v1/username/check?value=ivan-petrov", "", ""); code != 200 {
		t.Fatalf("check status: %d", code)
	} else if d, _ := body["data"].(map[string]any); d["available"] != false {
		t.Errorf("check available = %v, want false after claim", body["data"])
	}

	// A second account cannot claim the same name.
	_, cookie2 := seedUser()
	if code, _ := do("PUT", "/api/v1/me/username", cookie2, `{"username":"ivan-petrov"}`); code != fiber.StatusConflict {
		t.Errorf("claiming a taken name = %d, want 409", code)
	}

	// A reserved name is rejected.
	if code, _ := do("PUT", "/api/v1/me/username", cookie2, `{"username":"admin"}`); code != fiber.StatusConflict {
		t.Errorf("claiming a reserved name = %d, want 409", code)
	}

	// An invalid format is rejected.
	if code, _ := do("PUT", "/api/v1/me/username", cookie2, `{"username":"AB"}`); code != fiber.StatusBadRequest {
		t.Errorf("claiming an invalid name = %d, want 400", code)
	}

	// Changing again immediately hits the cooldown.
	if code, body := do("PUT", "/api/v1/me/username", cookie2, `{"username":"petr"}`); code != 200 {
		t.Fatalf("first claim for second account: %d, body: %v", code, body)
	}
	if code, _ := do("PUT", "/api/v1/me/username", cookie2, `{"username":"petr-2"}`); code != fiber.StatusTooManyRequests {
		t.Errorf("changing within cooldown = %d, want 429", code)
	}
}

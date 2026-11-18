//go:build integration

package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/engage/ga4mp"
	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/platform/db"
)

type fakeGA4 struct {
	mu    sync.Mutex
	calls [][3]string
	err   error
}

func (f *fakeGA4) SignUp(_ context.Context, clientID, sessionID, method string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, [3]string{clientID, sessionID, method})
	return f.err
}

func TestSignupAttributionEndToEnd(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()

	var freshID, oldID int64
	if err := pool.QueryRow(ctx, `INSERT INTO users (email, email_verified) VALUES ('fresh-attr@example.test', true) RETURNING id`).Scan(&freshID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO users (email, email_verified, created_at) VALUES ('old-attr@example.test', true, now() - interval '3 days') RETURNING id`).Scan(&oldID); err != nil {
		t.Fatal(err)
	}

	iss := auth.NewIssuer("test-secret", time.Hour)
	freshCookie, _ := iss.Issue(freshID, testTokenVersion)
	oldCookie, _ := iss.Issue(oldID, testTokenVersion)
	ga := &fakeGA4{}
	h := &authHandlers{queries: db.New(pool), ga4: ga}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Post("/api/v1/me/attribution", auth.RequireAuth(iss, testVersions), newContract(contractHandlers{authHandlers: h}))

	var lastSent bool
	post := func(cookie, body string) int {
		req := httptest.NewRequestWithContext(ctx, fiber.MethodPost, "/api/v1/me/attribution", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: cookie})
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var out struct {
			Data struct {
				GA4Sent bool `json:"ga4_sent"`
			} `json:"data"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&out)
		lastSent = out.Data.GA4Sent
		return resp.StatusCode
	}

	body := `{"utm_source":"LinkedIn","utm_medium":"social","referrer_host":"https://www.linkedin.com/feed","landing_path":"/jobs?x=1","method":"oauth","ga_client_id":"123.456","ga_session_id":"1700000000"}`
	if got := post(freshCookie, body); got != fiber.StatusOK {
		t.Fatalf("first report = %d", got)
	}
	if !lastSent {
		t.Fatal("the server sent GA4, so it must say so")
	}
	// A replay must neither overwrite the row nor send a second GA event.
	if got := post(freshCookie, `{"utm_source":"other","ga_client_id":"9.9"}`); got != fiber.StatusOK {
		t.Fatalf("replay = %d", got)
	}
	if !lastSent {
		t.Fatal("a replay must tell the browser to stay out of it: the first report owns the event")
	}

	var source, host, path string
	if err := pool.QueryRow(ctx, `SELECT utm_source, referrer_host, landing_path FROM user_signup_attribution WHERE user_id=$1`, freshID).Scan(&source, &host, &path); err != nil {
		t.Fatal(err)
	}
	if source != "linkedin" || host != "linkedin.com" || path != "/jobs" {
		t.Fatalf("stored = %q %q %q", source, host, path)
	}
	if len(ga.calls) != 1 || ga.calls[0] != [3]string{"123.456", "1700000000", "oauth"} {
		t.Fatalf("ga calls = %v", ga.calls)
	}

	// An account that is not new has no arrival to record.
	if got := post(oldCookie, body); got != fiber.StatusOK {
		t.Fatalf("old account = %d", got)
	}
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM user_signup_attribution WHERE user_id=$1`, oldID).Scan(&n); err != nil || n != 0 {
		t.Fatalf("old account row count = %d err=%v", n, err)
	}
	if len(ga.calls) != 1 {
		t.Fatalf("old account must not send to GA, calls = %v", ga.calls)
	}

	// The browser's fallback depends on what the answer says in each failure mode.
	for _, tc := range []struct {
		name     string
		err      error
		wantSent bool
	}{
		{"collector refused", errors.New("collect answered 400"), false},
		{"outcome unknown", errors.Join(ga4mp.ErrUncertain, errors.New("timeout")), true},
	} {
		var id int64
		if err := pool.QueryRow(ctx, `INSERT INTO users (email, email_verified) VALUES ($1, true) RETURNING id`, tc.name+"@example.test").Scan(&id); err != nil {
			t.Fatal(err)
		}
		cookie, _ := iss.Issue(id, testTokenVersion)
		failing := &authHandlers{queries: db.New(pool), ga4: &fakeGA4{err: tc.err}}
		app2 := fiber.New(fiber.Config{ErrorHandler: RenderError})
		app2.Post("/api/v1/me/attribution", auth.RequireAuth(iss, testVersions), newContract(contractHandlers{authHandlers: failing}))
		req := httptest.NewRequestWithContext(ctx, fiber.MethodPost, "/api/v1/me/attribution", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: cookie})
		resp, err := app2.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		var out struct {
			Data struct {
				GA4Sent bool `json:"ga4_sent"`
			} `json:"data"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&out)
		resp.Body.Close()
		if out.Data.GA4Sent != tc.wantSent {
			t.Errorf("%s: ga4_sent = %v, want %v", tc.name, out.Data.GA4Sent, tc.wantSent)
		}
	}
}

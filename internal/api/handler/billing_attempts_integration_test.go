//go:build integration

package handler

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/identity/auth"
)

func TestBillingAttemptsArePrivateAndIndependentOfSubscription(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	var owner, stranger int64
	if err := pool.QueryRow(ctx, `INSERT INTO users (email, stripe_customer_id) VALUES ('attempt-owner@example.test', 'cus_owner') RETURNING id`).Scan(&owner); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO users (email) VALUES ('attempt-stranger@example.test') RETURNING id`).Scan(&stranger); err != nil {
		t.Fatal(err)
	}
	providerCalls := 0
	providerFails := false
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		providerCalls++
		if r.URL.Path != "/charges" || r.URL.Query().Get("customer") != "cus_owner" {
			t.Errorf("unexpected provider request: %s", r.URL)
		}
		if providerFails {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = fmt.Fprint(w, `{"data":[{"id":"ch_owned","status":"failed","amount":290,"currency":"brl"}]}`)
	}))
	defer stub.Close()
	iss := auth.NewIssuer("test-secret", time.Hour)
	app := billingApp(t, pool, enabledBillingConfig(), stub.URL, iss)
	request := func(userID int64) (int, string, string) {
		t.Helper()
		// A caller-supplied customer must never override the session's binding.
		req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/api/v1/billing/attempts?customer=cus_victim", nil)
		if userID != 0 {
			token, _ := iss.Issue(userID, testTokenVersion)
			req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
		}
		resp, err := app.Test(req, 15000)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		return resp.StatusCode, string(body), resp.Header.Get("Cache-Control")
	}
	if status, _, _ := request(0); status != http.StatusUnauthorized {
		t.Fatalf("anonymous status = %d", status)
	}
	if status, body, _ := request(stranger); status != http.StatusOK || body != `{"data":[]}` || providerCalls != 0 {
		t.Fatalf("unbound account: %d %s, calls=%d", status, body, providerCalls)
	}
	if status, body, cache := request(owner); status != http.StatusOK || !strings.Contains(body, "ch_owned") || cache != "no-store" {
		t.Fatalf("owner: %d %s, cache=%s", status, body, cache)
	}
	providerFails = true
	if status, _, _ := request(owner); status != http.StatusServiceUnavailable {
		t.Fatalf("provider failure status = %d", status)
	}
}

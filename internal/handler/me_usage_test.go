package handler

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/auth"
	"github.com/strelov1/freehire/internal/llmkey"
)

// usageGateway answers /key/info per credential, so a test can give two accounts
// different spend and prove neither can see the other's.
func usageGateway(t *testing.T, byCredential map[string]string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/key/info" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		body, ok := byCredential[r.Header.Get("Authorization")]
		if !ok {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, `{"error":"invalid key"}`)
			return
		}
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func usageApp(t *testing.T, store *stubKeyQueries, gatewayURL string, userID int64) (*fiber.App, string) {
	t.Helper()
	iss := auth.NewIssuer("test-secret", time.Hour)
	token, err := iss.Issue(userID, testTokenVersion)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	gateway := llmkey.New(llmkey.Config{BaseURL: gatewayURL, AdminKey: "sk-admin"})
	h := &usageHandlers{keys: llmkey.NewResolver(store, gateway), gateway: gateway}

	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/api/v1/me/usage", auth.RequireAuth(iss, testVersions), h.GetMyUsage)
	return app, token
}

func readUsage(t *testing.T, app *fiber.App, token string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(fiber.MethodGet, "/api/v1/me/usage", nil)
	if token != "" {
		req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	}
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	var body struct {
		Data map[string]any `json:"data"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&body)
	return resp.StatusCode, body.Data
}

func TestUsageReportsTheCallersSpend(t *testing.T) {
	store := newStubKeyQueries()
	store.stored[7] = "sk-seven"
	gw := usageGateway(t, map[string]string{
		"Bearer sk-seven": `{"info":{"spend":1.25,"max_budget":10,"budget_reset_at":"2026-09-01T00:00:00Z"}}`,
	})
	app, token := usageApp(t, store, gw.URL, 7)

	status, data := readUsage(t, app, token)
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if data["spend"] != 1.25 {
		t.Errorf("spend = %v, want 1.25", data["spend"])
	}
	if data["limit"] != 10.0 {
		t.Errorf("limit = %v, want 10", data["limit"])
	}
	if data["resets_at"] == nil {
		t.Error("resets_at missing; a period with no end is not a period")
	}
}

// The credential is infrastructure bookkeeping, not something the account holder has any
// use for — and the endpoint that reads their spend is the one place it could plausibly
// slip out. Asserted on the raw body rather than a decoded field, because a field nobody
// declared is exactly how it would escape.
func TestUsageNeverReturnsTheCredential(t *testing.T) {
	store := newStubKeyQueries()
	store.stored[7] = "sk-secret-value"
	gw := usageGateway(t, map[string]string{
		"Bearer sk-secret-value": `{"info":{"spend":1.25,"max_budget":null,"budget_reset_at":null}}`,
	})
	app, token := usageApp(t, store, gw.URL, 7)

	req := httptest.NewRequest(fiber.MethodGet, "/api/v1/me/usage", nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if strings.Contains(string(raw), "sk-secret-value") {
		t.Errorf("the response carried the caller's gateway credential: %s", raw)
	}
}

// Never used AI is a real and common state, and it is zero — not an error, and not a 404
// that a client has to special-case before it can render a number.
func TestUsageAnswersACallerWithNoneAsZeroes(t *testing.T) {
	store := newStubKeyQueries() // nothing stored: this account has never spent
	gw := usageGateway(t, map[string]string{})
	app, token := usageApp(t, store, gw.URL, 7)

	status, data := readUsage(t, app, token)
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if data["spend"] != 0.0 {
		t.Errorf("spend = %v, want 0", data["spend"])
	}
}

// Reading a usage page must not mint anything. Minting on read would create a credential
// for every curious visitor and make "accounts with a key" a meaningless number.
func TestUsageDoesNotMintACredential(t *testing.T) {
	store := newStubKeyQueries()
	gw := usageGateway(t, map[string]string{})
	app, token := usageApp(t, store, gw.URL, 7)

	if _, _ = readUsage(t, app, token); true {
		store.mu.Lock()
		defer store.mu.Unlock()
		if _, present := store.stored[7]; present {
			t.Error("reading usage minted a credential; only spending should")
		}
	}
}

func TestUsageIsOwnerScoped(t *testing.T) {
	store := newStubKeyQueries()
	store.stored[7] = "sk-seven"
	store.stored[8] = "sk-eight"
	gw := usageGateway(t, map[string]string{
		"Bearer sk-seven": `{"info":{"spend":1.25,"max_budget":null,"budget_reset_at":null}}`,
		"Bearer sk-eight": `{"info":{"spend":99,"max_budget":null,"budget_reset_at":null}}`,
	})

	seven, tokenSeven := usageApp(t, store, gw.URL, 7)
	if _, data := readUsage(t, seven, tokenSeven); data["spend"] != 1.25 {
		t.Errorf("account 7 saw %v, want its own 1.25", data["spend"])
	}
	eight, tokenEight := usageApp(t, store, gw.URL, 8)
	if _, data := readUsage(t, eight, tokenEight); data["spend"] != 99.0 {
		t.Errorf("account 8 saw %v, want its own 99", data["spend"])
	}
}

func TestUsageRequiresACredential(t *testing.T) {
	store := newStubKeyQueries()
	gw := usageGateway(t, map[string]string{})
	app, _ := usageApp(t, store, gw.URL, 7)

	if status, _ := readUsage(t, app, ""); status != fiber.StatusUnauthorized {
		t.Errorf("status = %d, want 401", status)
	}
}

// A usage readout is informational. Rendering a proxy blip as a fault would make an
// unrelated outage look like a billing problem to the person reading it.
func TestUsageAnswersZeroesWhenTheGatewayIsUnreachable(t *testing.T) {
	store := newStubKeyQueries()
	store.stored[7] = "sk-seven"
	dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(dead.Close)
	app, token := usageApp(t, store, dead.URL, 7)

	status, data := readUsage(t, app, token)
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200 even with the gateway down", status)
	}
	if data["spend"] != 0.0 {
		t.Errorf("spend = %v, want 0", data["spend"])
	}
}

// A credential the gateway has forgotten is cleared on the spot, so the account's next
// model call mints a working replacement instead of failing over and over.
func TestUsageForgetsACredentialTheGatewayRejects(t *testing.T) {
	store := newStubKeyQueries()
	store.stored[7] = "sk-stale"
	gw := usageGateway(t, map[string]string{}) // knows nothing: every key is refused
	app, token := usageApp(t, store, gw.URL, 7)

	if status, _ := readUsage(t, app, token); status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.stored[7] != "" {
		t.Errorf("stored %q, want the rejected credential cleared", store.stored[7])
	}
}

// An unconfigured deployment has no gateway at all, and that is ordinary: the endpoint
// exists and reports nothing rather than 501-ing a surface the SPA would have to hide.
func TestUsageWithNoGatewayConfiguredIsStillZeroes(t *testing.T) {
	iss := auth.NewIssuer("test-secret", time.Hour)
	token, _ := iss.Issue(7, testTokenVersion)
	h := &usageHandlers{keys: llmkey.NewResolver(newStubKeyQueries(), nil)}

	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/api/v1/me/usage", auth.RequireAuth(iss, testVersions), h.GetMyUsage)

	status, data := readUsage(t, app, token)
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if data["spend"] != 0.0 {
		t.Errorf("spend = %v, want 0", data["spend"])
	}
}

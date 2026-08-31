package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/ai/llmkey"
	"github.com/strelov1/freehire/internal/ai/plan"
	"github.com/strelov1/freehire/internal/identity/auth"
)

// activity is one credential's month, as the fake gateway will report it.
type activity struct{ requests, failed, tokens int }

// activityGateway answers the usage read per CREDENTIAL id, so a test can give two
// accounts different numbers and prove neither can see the other's.
//
// It serves the read twice over, because that is what the client does: once for the
// totals and once filtered to failures, where total_requests IS the failure count. A fake
// that ignored the filter would report every call as failed.
//
// The totals always carry a cost the handler must drop; see the wire-shape test below.
func activityGateway(t *testing.T, byKey map[string]activity) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/logs/stats" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		act := byKey[r.URL.Query().Get("virtual_key_ids")]
		if r.URL.Query().Get("status") == "error" {
			_, _ = fmt.Fprintf(w, `{"total_requests":%d}`, act.failed)
			return
		}
		_, _ = fmt.Fprintf(w, `{"total_requests":%d,"total_tokens":%d,"total_cost":9.99}`,
			act.requests, act.tokens)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// usageKeyID is the credential id this account's usage is filed under. The handler reads
// it from the store rather than deriving it, so the store has to be seeded with it — an
// account whose row holds no id reports zeroes, which is a real state and not a fault.
func usageKeyID(userID int64) string { return fmt.Sprintf("vk-%d", userID) }

func usageApp(t *testing.T, gatewayURL string, userID int64) (*fiber.App, string) {
	t.Helper()
	iss := auth.NewIssuer("test-secret", time.Hour)
	token, err := iss.Issue(userID, testTokenVersion)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	store := newStubKeyQueries()
	store.stored[userID] = fmt.Sprintf("sk-user-%d", userID)
	store.storedIDs[userID] = usageKeyID(userID)
	gateway := llmkey.New(testGatewayConfig(gatewayURL))
	h := &usageHandlers{gateway: gateway, keys: llmkey.NewResolver(store, gateway)}

	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/api/v1/me/usage", auth.RequireAuth(iss, testVersions), h.GetMyUsage)
	return app, token
}

func readUsage(t *testing.T, app *fiber.App, token string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, "/api/v1/me/usage", nil)
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

func TestUsageReportsWhatTheCallerDid(t *testing.T) {
	gw := activityGateway(t, map[string]activity{
		"vk-7": {requests: 128, failed: 2, tokens: 450000},
	})
	app, token := usageApp(t, gw.URL, 7)

	status, data := readUsage(t, app, token)
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if data["requests"] != 128.0 || data["failed"] != 2.0 || data["tokens"] != 450000.0 {
		t.Errorf("data = %v, want 128 calls, 2 failed, 450000 tokens", data)
	}
	if data["period"] == "" || data["resets_at"] == nil {
		t.Errorf("data = %v, want the period named and its end", data)
	}
}

// No money, ever. The gateway's figure is a list price on a mixed pool — not our cost, and
// not what the caller spends, which is a plan allowance. Two currencies for one thing, one
// of them fictional.
func TestUsageReportsNoMoney(t *testing.T) {
	gw := activityGateway(t, map[string]activity{
		"vk-7": {requests: 5, tokens: 10},
	})
	app, token := usageApp(t, gw.URL, 7)

	_, data := readUsage(t, app, token)
	for _, field := range []string{"spend", "cost", "limit", "usd"} {
		if _, present := data[field]; present {
			t.Errorf("the response carried %q = %v; usage is reported in calls, not currency", field, data[field])
		}
	}
}

// The activity figure and the plan allowances must describe the same period, or a reader
// comparing "what I used" against "what I have left" is comparing two calendars. It
// narrowed from a month to a day when the monthly balance was withdrawn.
func TestUsagePeriodMatchesThePlanCalendar(t *testing.T) {
	gw := activityGateway(t, map[string]activity{})
	app, token := usageApp(t, gw.URL, 7)

	_, data := readUsage(t, app, token)
	want := plan.Day(time.Now().UTC()).Format(time.DateOnly)
	if data["period"] != want {
		t.Errorf("period = %v, want the UTC day %q the allowances reset on", data["period"], want)
	}
}

// Never used AI is a real and common state, and it is zero — not an error, and not a 404
// that a client has to special-case before it can render a number.
func TestUsageAnswersACallerWithNoneAsZeroes(t *testing.T) {
	gw := activityGateway(t, map[string]activity{})
	app, token := usageApp(t, gw.URL, 7)

	status, data := readUsage(t, app, token)
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if data["requests"] != 0.0 {
		t.Errorf("requests = %v, want 0", data["requests"])
	}
}

func TestUsageIsOwnerScoped(t *testing.T) {
	gw := activityGateway(t, map[string]activity{
		"vk-7": {requests: 12, tokens: 1},
		"vk-8": {requests: 99, tokens: 1},
	})

	seven, tokenSeven := usageApp(t, gw.URL, 7)
	if _, data := readUsage(t, seven, tokenSeven); data["requests"] != 12.0 {
		t.Errorf("account 7 saw %v, want its own 12", data["requests"])
	}
	eight, tokenEight := usageApp(t, gw.URL, 8)
	if _, data := readUsage(t, eight, tokenEight); data["requests"] != 99.0 {
		t.Errorf("account 8 saw %v, want its own 99", data["requests"])
	}
}

func TestUsageRequiresACredential(t *testing.T) {
	gw := activityGateway(t, map[string]activity{})
	app, _ := usageApp(t, gw.URL, 7)

	if status, _ := readUsage(t, app, ""); status != fiber.StatusUnauthorized {
		t.Errorf("status = %d, want 401", status)
	}
}

// A usage readout is informational. Rendering a proxy blip as a fault would make an
// unrelated outage look to the reader like a problem with their own account.
func TestUsageAnswersZeroesWhenTheGatewayIsUnreachable(t *testing.T) {
	dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(dead.Close)
	app, token := usageApp(t, dead.URL, 7)

	status, data := readUsage(t, app, token)
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200 even with the gateway down", status)
	}
	if data["requests"] != 0.0 {
		t.Errorf("requests = %v, want 0", data["requests"])
	}
	if data["period"] == nil {
		t.Error("the period should still be reported; it is ours to compute, not the gateway's")
	}
}

// An unconfigured deployment has no gateway at all, and that is ordinary: the endpoint
// exists and reports nothing rather than 501-ing a surface the SPA would have to hide.
func TestUsageWithNoGatewayConfiguredIsStillZeroes(t *testing.T) {
	iss := auth.NewIssuer("test-secret", time.Hour)
	token, _ := iss.Issue(7, testTokenVersion)
	h := &usageHandlers{}

	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/api/v1/me/usage", auth.RequireAuth(iss, testVersions), h.GetMyUsage)

	status, data := readUsage(t, app, token)
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if data["requests"] != 0.0 {
		t.Errorf("requests = %v, want 0", data["requests"])
	}
}

// The response is about what an account DID, never about how it is known: no credential,
// minted or stored, may appear on the wire. The figures are now scoped to the account's
// current credential — see the design note on that narrowing — but the secret behind it
// stays entirely server-side.
func TestUsageNeitherNeedsNorReturnsACredential(t *testing.T) {
	gw := activityGateway(t, map[string]activity{
		"vk-7": {requests: 3, tokens: 9},
	})
	app, token := usageApp(t, gw.URL, 7)

	req := httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, "/api/v1/me/usage", nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if strings.Contains(string(raw), "sk-") {
		t.Errorf("the response looks like it carried a credential: %s", raw)
	}
}

// Reading a usage page must never mint. Attribution exists to say what somebody spent, and
// a credential issued to every visitor who opened the page out of curiosity would turn
// "accounts with a key" from a count of who has spent into a count of who has looked.
func TestUsageOfAnUncredentialledAccountMintsNothing(t *testing.T) {
	gw := activityGateway(t, map[string]activity{})

	iss := auth.NewIssuer("test-secret", time.Hour)
	token, err := iss.Issue(7, testTokenVersion)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	// Deliberately unseeded: this account has never made an AI call.
	store := newStubKeyQueries()
	gateway := llmkey.New(testGatewayConfig(gw.URL))
	h := &usageHandlers{gateway: gateway, keys: llmkey.NewResolver(store, gateway)}

	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/api/v1/me/usage", auth.RequireAuth(iss, testVersions), h.GetMyUsage)

	status, data := readUsage(t, app, token)
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200 — never having used AI is an answer, not a fault", status)
	}
	if data["requests"] != float64(0) || data["tokens"] != float64(0) {
		t.Errorf("data = %v, want zeroes", data)
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.stored) != 0 {
		t.Errorf("store holds %v, want nothing minted by a read", store.stored)
	}
}

// A credential minted before the id column existed has a secret and nothing to ask the
// gateway with. Zeroes are the honest answer — the account self-heals into a complete pair
// on the first refusal — but the read must not go out at all, because an id-less query
// asks about every credential rather than none.
func TestUsageOfAnIdlessCredentialAsksNothing(t *testing.T) {
	asked := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked = true
		_, _ = io.WriteString(w, `{"total_requests":999,"total_tokens":999}`)
	}))
	t.Cleanup(srv.Close)

	iss := auth.NewIssuer("test-secret", time.Hour)
	token, err := iss.Issue(7, testTokenVersion)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	store := newStubKeyQueries()
	store.stored[7] = "sk-pre-0119" // secret, deliberately no id
	gateway := llmkey.New(testGatewayConfig(srv.URL))
	h := &usageHandlers{gateway: gateway, keys: llmkey.NewResolver(store, gateway)}

	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/api/v1/me/usage", auth.RequireAuth(iss, testVersions), h.GetMyUsage)

	status, data := readUsage(t, app, token)
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if data["requests"] != float64(0) {
		t.Errorf("requests = %v, want zero — there is no credential to report under", data["requests"])
	}
	if asked {
		t.Error("the gateway was asked with no credential id, which is a question about everybody's usage")
	}
}

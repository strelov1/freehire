package handler

import (
	"context"
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

// activityGateway answers the daily-activity read per account id, so a test can give two
// accounts different numbers and prove neither can see the other's.
func activityGateway(t *testing.T, byAccount map[string]string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/user/daily/activity" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		body, ok := byAccount[r.URL.Query().Get("user_id")]
		if !ok {
			body = `{"results":[],"metadata":{"total_api_requests":0,"total_failed_requests":0,"total_tokens":0}}`
		}
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func usageApp(t *testing.T, gatewayURL string, userID int64) (*fiber.App, string) {
	t.Helper()
	iss := auth.NewIssuer("test-secret", time.Hour)
	token, err := iss.Issue(userID, testTokenVersion)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	h := &usageHandlers{gateway: llmkey.New(llmkey.Config{BaseURL: gatewayURL, AdminKey: "sk-admin"})}

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
	gw := activityGateway(t, map[string]string{
		"freehire-7": `{"results":[],"metadata":{"total_api_requests":128,"total_failed_requests":2,"total_tokens":450000}}`,
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

// No money, ever. The gateway's figure is a list price on a mixed pool — not our cost and
// not the caller's price, which is credits. Two currencies for one thing, one fictional.
func TestUsageReportsNoMoney(t *testing.T) {
	gw := activityGateway(t, map[string]string{
		"freehire-7": `{"results":[],"metadata":{"total_api_requests":5,"total_spend":9.99,"total_tokens":10}}`,
	})
	app, token := usageApp(t, gw.URL, 7)

	_, data := readUsage(t, app, token)
	for _, field := range []string{"spend", "cost", "limit", "usd"} {
		if _, present := data[field]; present {
			t.Errorf("the response carried %q = %v; usage is reported in calls, not currency", field, data[field])
		}
	}
}

// The period must be the one credits already reset on, or a balance and a usage count sit
// on different months and both look correct.
func TestUsagePeriodMatchesTheCreditsCalendar(t *testing.T) {
	gw := activityGateway(t, map[string]string{})
	app, token := usageApp(t, gw.URL, 7)

	_, data := readUsage(t, app, token)
	want := time.Now().UTC().Format("2006-01")
	if data["period"] != want {
		t.Errorf("period = %v, want the calendar month %q", data["period"], want)
	}
}

// Never used AI is a real and common state, and it is zero — not an error, and not a 404
// that a client has to special-case before it can render a number.
func TestUsageAnswersACallerWithNoneAsZeroes(t *testing.T) {
	gw := activityGateway(t, map[string]string{})
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
	gw := activityGateway(t, map[string]string{
		"freehire-7": `{"results":[],"metadata":{"total_api_requests":12,"total_failed_requests":0,"total_tokens":1}}`,
		"freehire-8": `{"results":[],"metadata":{"total_api_requests":99,"total_failed_requests":0,"total_tokens":1}}`,
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
	gw := activityGateway(t, map[string]string{})
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

// The read is scoped by the account id, never by a credential — so it needs no key, mints
// nothing, and still reports a month during which the key was re-minted.
func TestUsageNeitherNeedsNorReturnsACredential(t *testing.T) {
	gw := activityGateway(t, map[string]string{
		"freehire-7": `{"results":[],"metadata":{"total_api_requests":3,"total_failed_requests":0,"total_tokens":9}}`,
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

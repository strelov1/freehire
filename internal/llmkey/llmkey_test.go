package llmkey

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// captured is what the fake gateway saw, so a test can assert on the request the client
// built rather than only on what it returned.
type captured struct {
	path   string
	authz  string
	method string
	body   map[string]any
	query  string
}

// gateway stands in for the admin API. It records the request and answers with status
// and body.
func gateway(t *testing.T, status int, body string) (*httptest.Server, *captured) {
	t.Helper()
	got := &captured{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.path = r.URL.Path
		got.authz = r.Header.Get("Authorization")
		got.method = r.Method
		got.query = r.URL.RawQuery
		if raw, err := io.ReadAll(r.Body); err == nil && len(raw) > 0 {
			_ = json.Unmarshal(raw, &got.body)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	return srv, got
}

func TestNewReportsUnconfiguredAsNil(t *testing.T) {
	tests := []struct {
		name              string
		baseURL, adminKey string
	}{
		{"both empty", "", ""},
		{"no base url", "", "sk-admin"},
		{"no admin key", "https://gw.example", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if c := New(Config{BaseURL: tc.baseURL, AdminKey: tc.adminKey}); c != nil {
				t.Errorf("New(%q, %q) = %v, want nil — an unconfigured gateway must be absent, not broken", tc.baseURL, tc.adminKey, c)
			}
		})
	}
}

func TestMintNamesTheAccountOnTheGateway(t *testing.T) {
	srv, got := gateway(t, http.StatusOK, `{"key":"sk-minted","token_id":"hash-abc"}`)
	c := New(Config{BaseURL: srv.URL, AdminKey: "sk-admin"})

	key, err := c.Mint(context.Background(), 42)
	if err != nil {
		t.Fatalf("Mint: %v", err)
	}
	if key != "sk-minted" {
		t.Errorf("Mint = %q, want the minted secret", key)
	}
	if got.path != "/key/generate" || got.method != http.MethodPost {
		t.Errorf("called %s %s, want POST /key/generate", got.method, got.path)
	}
	if got.authz != "Bearer sk-admin" {
		t.Errorf("authorization = %q, want the admin credential", got.authz)
	}
	// The account has to be named on the gateway's side or DailyUserSpend cannot
	// attribute anything — this is the entire point of the change.
	if uid, _ := got.body["user_id"].(string); uid != "freehire-42" {
		t.Errorf("user_id = %v, want the namespaced account id", got.body["user_id"])
	}
	if alias, _ := got.body["key_alias"].(string); alias != "freehire-user-42" {
		t.Errorf("key_alias = %v, want a recognisable per-user alias", got.body["key_alias"])
	}
}

// A ceiling is policy, and this change deliberately ships without one. Sending a zero
// would be read by the gateway as "no budget at all is allowed" rather than "no limit".
func TestMintSendsNoCeilingWhenNoneIsConfigured(t *testing.T) {
	srv, got := gateway(t, http.StatusOK, `{"key":"sk-minted","token_id":"hash-abc"}`)
	c := New(Config{BaseURL: srv.URL, AdminKey: "sk-admin"})

	if _, err := c.Mint(context.Background(), 1); err != nil {
		t.Fatalf("Mint: %v", err)
	}
	for _, field := range []string{"max_budget", "rpm_limit", "budget_duration"} {
		if _, present := got.body[field]; present {
			t.Errorf("%s was sent as %v, want it omitted entirely when unconfigured", field, got.body[field])
		}
	}
}

func TestMintPassesAConfiguredCeilingThrough(t *testing.T) {
	srv, got := gateway(t, http.StatusOK, `{"key":"sk-minted","token_id":"hash-abc"}`)
	c := New(Config{
		BaseURL: srv.URL, AdminKey: "sk-admin",
		MaxBudget: 2.5, RPMLimit: 60, BudgetWindow: "30d",
	})

	if _, err := c.Mint(context.Background(), 1); err != nil {
		t.Fatalf("Mint: %v", err)
	}
	if budget, _ := got.body["max_budget"].(float64); budget != 2.5 {
		t.Errorf("max_budget = %v, want 2.5", got.body["max_budget"])
	}
	if rpm, _ := got.body["rpm_limit"].(float64); rpm != 60 {
		t.Errorf("rpm_limit = %v, want 60", got.body["rpm_limit"])
	}
	if window, _ := got.body["budget_duration"].(string); window != "30d" {
		t.Errorf("budget_duration = %v, want 30d", got.body["budget_duration"])
	}
}

// A refusal must not read as a successful mint of the empty string: storing "" would
// mark the account as credentialled forever and send every later call out unattributed.
func TestMintReportsARefusalAsAnError(t *testing.T) {
	srv, _ := gateway(t, http.StatusUnauthorized, `{"error":"bad admin key"}`)
	c := New(Config{BaseURL: srv.URL, AdminKey: "sk-wrong"})

	key, err := c.Mint(context.Background(), 1)
	if err == nil {
		t.Fatal("Mint on a refusing gateway must fail")
	}
	if !errors.Is(err, ErrUpstream) {
		t.Errorf("error = %v, want it to wrap ErrUpstream so callers can tell whose fault it is", err)
	}
	if key != "" {
		t.Errorf("Mint returned secret %q alongside an error, want nothing usable", key)
	}
}

// A 200 carrying no key is the same hazard as a refusal, and gateways do answer this way
// when a proxy in front of them rewrites a response.
func TestMintReportsAnEmptyKeyAsAnError(t *testing.T) {
	srv, _ := gateway(t, http.StatusOK, `{"token_id":"hash-abc"}`)
	c := New(Config{BaseURL: srv.URL, AdminKey: "sk-admin"})

	if _, err := c.Mint(context.Background(), 1); err == nil {
		t.Fatal("a 200 with no key must fail — storing an empty credential is worse than not minting")
	}
}

func TestSpendReadsThePeriodBack(t *testing.T) {
	srv, got := gateway(t, http.StatusOK, `{"info":{"spend":1.25,"max_budget":10,"budget_reset_at":"2026-09-01T00:00:00Z"}}`)
	c := New(Config{BaseURL: srv.URL, AdminKey: "sk-admin"})

	spend, err := c.Spend(context.Background(), "sk-user")
	if err != nil {
		t.Fatalf("Spend: %v", err)
	}
	if spend.Amount != 1.25 || spend.Limit != 10 {
		t.Errorf("Spend = %+v, want the amount and the ceiling", spend)
	}
	if spend.ResetsAt.IsZero() || spend.ResetsAt.Year() != 2026 || spend.ResetsAt.Month() != 9 {
		t.Errorf("ResetsAt = %v, want the parsed reset instant", spend.ResetsAt)
	}
	if got.path != "/key/info" || got.method != http.MethodGet {
		t.Errorf("called %s %s, want GET /key/info", got.method, got.path)
	}
	// The read authenticates AS the key, and the key must never travel in the URL: the
	// gateway logs the request URI, so a query parameter would file a live credential
	// into an access log in plaintext on every usage read.
	if got.query != "" {
		t.Errorf("query = %q, want none — a credential in a URL lands in access logs", got.query)
	}
	if got.authz != "Bearer sk-user" {
		t.Errorf("authorization = %q, want the caller's own key, not the administrator's", got.authz)
	}
}

// A self-read is authenticated by the very key being asked about, so a refusal means the
// gateway has forgotten it — the signal to mint a replacement.
func TestSpendReportsARefusedSelfReadAsAnUnknownKey(t *testing.T) {
	srv, _ := gateway(t, http.StatusUnauthorized, `{"error":"invalid key"}`)
	c := New(Config{BaseURL: srv.URL, AdminKey: "sk-admin"})

	if _, err := c.Spend(context.Background(), "sk-forgotten"); !errors.Is(err, ErrUnknownKey) {
		t.Errorf("error = %v, want ErrUnknownKey so the caller can re-mint", err)
	}
}

// The same status on an ADMINISTRATIVE call means our own admin credential is wrong.
// Reading that as a stale user key would turn one bad environment variable into a
// re-minting storm across every account — so the two must never be conflated.
func TestARefusedAdminCallIsNotAnUnknownUserKey(t *testing.T) {
	srv, _ := gateway(t, http.StatusUnauthorized, `{"error":"bad admin key"}`)
	c := New(Config{BaseURL: srv.URL, AdminKey: "sk-wrong"})

	_, err := c.Mint(context.Background(), 1)
	if errors.Is(err, ErrUnknownKey) {
		t.Errorf("error = %v, want a plain upstream failure — a misconfigured admin key is not a stale user key", err)
	}
	if !errors.Is(err, ErrUpstream) {
		t.Errorf("error = %v, want it to wrap ErrUpstream", err)
	}
}

// A gateway that reports no ceiling and no reset is the default deployment, not an error.
func TestSpendToleratesAnAbsentCeiling(t *testing.T) {
	srv, _ := gateway(t, http.StatusOK, `{"info":{"spend":0.5,"max_budget":null,"budget_reset_at":null}}`)
	c := New(Config{BaseURL: srv.URL, AdminKey: "sk-admin"})

	spend, err := c.Spend(context.Background(), "sk-user")
	if err != nil {
		t.Fatalf("Spend: %v", err)
	}
	if spend.Amount != 0.5 || spend.Limit != 0 || !spend.ResetsAt.IsZero() {
		t.Errorf("Spend = %+v, want the amount with an absent ceiling and reset", spend)
	}
}

// The gateway answers 404 for a key it does not know. That is the signal to re-mint, and
// it has to be distinguishable from "the gateway is down" — one heals itself, the other
// must not throw away a good credential.
func TestSpendReportsAnUnknownKeyDistinctly(t *testing.T) {
	srv, _ := gateway(t, http.StatusNotFound, `{"error":"key not found"}`)
	c := New(Config{BaseURL: srv.URL, AdminKey: "sk-admin"})

	_, err := c.Spend(context.Background(), "sk-forgotten")
	if !errors.Is(err, ErrUnknownKey) {
		t.Errorf("error = %v, want ErrUnknownKey so the caller can re-mint", err)
	}
}

func TestActivityCountsWhatTheAccountDid(t *testing.T) {
	srv, got := gateway(t, http.StatusOK, `{"results":[],"metadata":{
		"total_api_requests":128,"total_successful_requests":126,"total_failed_requests":2,
		"total_tokens":450000,"total_spend":1.25}}`)
	c := New(Config{BaseURL: srv.URL, AdminKey: "sk-admin"})

	from := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC)
	act, err := c.Activity(context.Background(), 42, from, to)
	if err != nil {
		t.Fatalf("Activity: %v", err)
	}
	if act.Requests != 128 || act.Failed != 2 || act.Tokens != 450000 {
		t.Errorf("Activity = %+v, want 128 calls, 2 failed, 450000 tokens", act)
	}
	if got.path != "/user/daily/activity" || got.method != http.MethodGet {
		t.Errorf("called %s %s, want GET /user/daily/activity", got.method, got.path)
	}
	// The account is named by the SAME namespaced id minting writes, or the read
	// silently reports zero for an account that has been busy all month.
	if !strings.Contains(got.query, "user_id=freehire-42") {
		t.Errorf("query = %q, want the namespaced account id", got.query)
	}
	if !strings.Contains(got.query, "start_date=2026-08-01") || !strings.Contains(got.query, "end_date=2026-08-31") {
		t.Errorf("query = %q, want the window as plain dates", got.query)
	}
}

// An account the gateway has never seen is the ordinary "never used AI" case, and the
// endpoint answers it with zeroes rather than an error.
func TestActivityOfAnUnknownAccountIsZero(t *testing.T) {
	srv, _ := gateway(t, http.StatusOK, `{"results":[],"metadata":{"total_api_requests":0,"total_failed_requests":0,"total_tokens":0}}`)
	c := New(Config{BaseURL: srv.URL, AdminKey: "sk-admin"})

	act, err := c.Activity(context.Background(), 42, time.Now(), time.Now())
	if err != nil {
		t.Fatalf("Activity: %v", err)
	}
	if act.Requests != 0 || act.Tokens != 0 {
		t.Errorf("Activity = %+v, want zeroes", act)
	}
}

func TestActivityReportsAFailure(t *testing.T) {
	srv, _ := gateway(t, http.StatusInternalServerError, `{"error":"boom"}`)
	c := New(Config{BaseURL: srv.URL, AdminKey: "sk-admin"})

	if _, err := c.Activity(context.Background(), 42, time.Now(), time.Now()); err == nil {
		t.Error("Activity must surface a gateway fault rather than report a false zero")
	}
}

// Blocking is how a credential is retired when its spend history matters. Deleting takes
// the gateway's record of what was spent with it; blocking stops the spending and keeps
// the record.
func TestBlockNamesTheKey(t *testing.T) {
	srv, got := gateway(t, http.StatusOK, `{"blocked":true}`)
	c := New(Config{BaseURL: srv.URL, AdminKey: "sk-admin"})

	if err := c.Block(context.Background(), "sk-user"); err != nil {
		t.Fatalf("Block: %v", err)
	}
	if got.path != "/key/block" || got.method != http.MethodPost {
		t.Errorf("called %s %s, want POST /key/block", got.method, got.path)
	}
	if key, _ := got.body["key"].(string); key != "sk-user" {
		t.Errorf("key = %v, want the one credential", got.body["key"])
	}
	if got.authz != "Bearer sk-admin" {
		t.Errorf("authorization = %q, want the admin credential — a key cannot block itself", got.authz)
	}
}

// A key the gateway has already forgotten is in the state asked for.
func TestBlockTreatsAnUnknownKeyAsDone(t *testing.T) {
	srv, _ := gateway(t, http.StatusNotFound, `{"error":"key not found"}`)
	c := New(Config{BaseURL: srv.URL, AdminKey: "sk-admin"})

	if err := c.Block(context.Background(), "sk-gone"); err != nil {
		t.Errorf("Block of an unknown key = %v, want nil", err)
	}
}

func TestBlockReportsARealFailure(t *testing.T) {
	srv, _ := gateway(t, http.StatusInternalServerError, `{"error":"boom"}`)
	c := New(Config{BaseURL: srv.URL, AdminKey: "sk-admin"})

	if err := c.Block(context.Background(), "sk-user"); err == nil {
		t.Error("Block must surface a gateway fault; the caller decides whether to care")
	}
}

func TestDeleteNamesTheKey(t *testing.T) {
	srv, got := gateway(t, http.StatusOK, `{"deleted_keys":["sk-user"]}`)
	c := New(Config{BaseURL: srv.URL, AdminKey: "sk-admin"})

	if err := c.Delete(context.Background(), "sk-user"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if got.path != "/key/delete" || got.method != http.MethodPost {
		t.Errorf("called %s %s, want POST /key/delete", got.method, got.path)
	}
	keys, _ := got.body["keys"].([]any)
	if len(keys) != 1 || keys[0] != "sk-user" {
		t.Errorf("keys = %v, want exactly the one key", got.body["keys"])
	}
}

// Deleting a key the gateway has already forgotten is the outcome the caller wanted.
// Reporting it as a failure would make account deletion log a fault on every retry.
func TestDeleteTreatsAnUnknownKeyAsDone(t *testing.T) {
	srv, _ := gateway(t, http.StatusNotFound, `{"error":"key not found"}`)
	c := New(Config{BaseURL: srv.URL, AdminKey: "sk-admin"})

	if err := c.Delete(context.Background(), "sk-gone"); err != nil {
		t.Errorf("Delete of an unknown key = %v, want nil — it is already in the state asked for", err)
	}
}

func TestDeleteReportsARealFailure(t *testing.T) {
	srv, _ := gateway(t, http.StatusInternalServerError, `{"error":"boom"}`)
	c := New(Config{BaseURL: srv.URL, AdminKey: "sk-admin"})

	if err := c.Delete(context.Background(), "sk-user"); err == nil {
		t.Error("Delete must surface a gateway fault; the caller decides whether to care")
	}
}

// A nil client is the unconfigured deployment. Every method must be safe on it, because
// the whole feature degrades by being absent rather than by being checked for everywhere.
func TestNilClientIsSafe(t *testing.T) {
	var c *Client
	if _, err := c.Mint(context.Background(), 1); err == nil {
		t.Error("Mint on an unconfigured gateway must fail rather than return a usable key")
	}
	if _, err := c.Spend(context.Background(), "sk-user"); err == nil {
		t.Error("Spend on an unconfigured gateway must fail rather than report a false zero")
	}
	if err := c.Delete(context.Background(), "sk-user"); err != nil {
		t.Errorf("Delete on an unconfigured gateway = %v, want nil — there is nothing to erase", err)
	}
}

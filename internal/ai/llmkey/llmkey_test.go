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

// templateKey is the virtual key every minted credential copies its provider policy from.
// Naming it once here keeps the fake and the assertions from drifting apart.
const templateKey = "vk-freehire-service"

// templatePath is where the fake serves that key.
const templatePath = "/api/governance/virtual-keys/" + templateKey

// templateBody is a plausible answer for a policy read, including the one asymmetry that
// matters: the gateway returns key_ids as null where a write requires ["*"].
const templateBody = `{"virtual_key":{"id":"vk-freehire-service","value":"sk-bf-svc","provider_configs":[
	{"provider":"zai","weight":0.7,"allowed_models":["flagship","mid"],"key_ids":null},
	{"provider":"gemini","weight":0.2,"allowed_models":["flagship","mid"],"key_ids":null}]}}`

// captured is what the fake gateway saw, so a test can assert on the request the client
// built rather than only on what it returned.
type captured struct {
	path   string
	method string
	query  string
	user   string
	pass   string
	body   map[string]any

	// reads counts template lookups, which is how a test tells "minting read the
	// policy" from "minting invented one".
	reads int
}

// gateway stands in for the admin API. It answers a template read from templateBody and
// everything else with status and body, recording that second request.
//
// Two behaviours in one fake because Mint is two calls: a test that stubbed only the
// second would see the first fail and never reach what it meant to assert.
func gateway(t *testing.T, status int, body string) (*httptest.Server, *captured) {
	t.Helper()
	return gatewayWithTemplate(t, templateBody, status, body)
}

func gatewayWithTemplate(t *testing.T, template string, status int, body string) (*httptest.Server, *captured) {
	t.Helper()
	got := &captured{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet && r.URL.Path == templatePath {
			got.reads++
			_, _ = io.WriteString(w, template)
			return
		}
		got.path = r.URL.Path
		got.method = r.Method
		got.query = r.URL.RawQuery
		got.user, got.pass, _ = r.BasicAuth()
		if raw, err := io.ReadAll(r.Body); err == nil && len(raw) > 0 {
			_ = json.Unmarshal(raw, &got.body)
		}
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	return srv, got
}

// configured is the client every test that is not about configuration uses.
func configured(url string) *Client {
	return New(Config{
		BaseURL: url, AdminUsername: "admin", AdminPassword: "secret", TemplateKey: templateKey,
	})
}

// mintedBody is what the gateway answers a create with.
const mintedBody = `{"virtual_key":{"id":"vk-42","value":"sk-bf-minted"}}`

func TestNewReportsUnconfiguredAsNil(t *testing.T) {
	full := Config{BaseURL: "https://gw.example", AdminUsername: "admin", AdminPassword: "secret", TemplateKey: templateKey}
	tests := []struct {
		name  string
		strip func(*Config)
	}{
		{"no base url", func(c *Config) { c.BaseURL = "" }},
		{"no username", func(c *Config) { c.AdminUsername = "" }},
		{"no password", func(c *Config) { c.AdminPassword = "" }},
		// A client without a template would mint credentials the gateway then refuses
		// every provider — an outage that reads as a model fault and is really a missing
		// environment variable. Absent is a better answer than subtly broken.
		{"no policy template", func(c *Config) { c.TemplateKey = "" }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := full
			tc.strip(&cfg)
			if c := New(cfg); c != nil {
				t.Errorf("New(%+v) = %v, want nil — an unconfigured gateway must be absent, not broken", cfg, c)
			}
		})
	}
}

func TestMintNamesTheAccountOnTheGateway(t *testing.T) {
	srv, got := gateway(t, http.StatusOK, mintedBody)

	cred, err := configured(srv.URL).Mint(context.Background(), 42)
	if err != nil {
		t.Fatalf("Mint: %v", err)
	}
	// Both halves or nothing: the secret spends, the id revokes, and a credential
	// carrying only one of them is unusable in one direction without saying so.
	if cred.Secret != "sk-bf-minted" || cred.ID != "vk-42" {
		t.Errorf("Mint = %+v, want the minted secret and the gateway's own id", cred)
	}
	if got.path != "/api/governance/virtual-keys" || got.method != http.MethodPost {
		t.Errorf("called %s %s, want POST /api/governance/virtual-keys", got.method, got.path)
	}
	// Basic, not bearer: this gateway's management surface admits one identity and
	// authenticates it with a username and password.
	if got.user != "admin" || got.pass != "secret" {
		t.Errorf("basic auth = %q/%q, want the administrator", got.user, got.pass)
	}
	// The account has to be legible in the gateway's own listings, or a key found there
	// belongs to nobody anybody can name.
	if name, _ := got.body["name"].(string); name != "freehire-user-42" {
		t.Errorf("name = %v, want a recognisable per-user name", got.body["name"])
	}
}

// The policy is read, never invented here. It is what keeps the provider vocabulary in
// the gateway's configuration: adding a provider must not become a deployment of this
// service.
func TestMintCopiesThePolicyFromTheTemplate(t *testing.T) {
	srv, got := gateway(t, http.StatusOK, mintedBody)

	if _, err := configured(srv.URL).Mint(context.Background(), 1); err != nil {
		t.Fatalf("Mint: %v", err)
	}
	if got.reads != 1 {
		t.Errorf("template reads = %d, want exactly 1 — the policy is read, not assumed", got.reads)
	}
	configs, _ := got.body["provider_configs"].([]any)
	if len(configs) != 2 {
		t.Fatalf("provider_configs = %v, want both entries the template carried", got.body["provider_configs"])
	}
	first, _ := configs[0].(map[string]any)
	if first["provider"] != "zai" {
		t.Errorf("first provider = %v, want the template's own order preserved", first["provider"])
	}
	if weight, _ := first["weight"].(float64); weight != 0.7 {
		t.Errorf("weight = %v, want the template's 0.7 — the weights are the fallback order", first["weight"])
	}
}

// A read answers key_ids as null; a write reads null as "no provider key at all" and
// mints a credential that can reach none of them. Echoing the read back would produce a
// key that authenticates and then fails on every call.
func TestMintNormalisesTheKeyAllowlistTheTemplateOmits(t *testing.T) {
	srv, got := gateway(t, http.StatusOK, mintedBody)

	if _, err := configured(srv.URL).Mint(context.Background(), 1); err != nil {
		t.Fatalf("Mint: %v", err)
	}
	configs, _ := got.body["provider_configs"].([]any)
	for i, raw := range configs {
		entry, _ := raw.(map[string]any)
		ids, _ := entry["key_ids"].([]any)
		if len(ids) != 1 || ids[0] != "*" {
			t.Errorf("provider_configs[%d].key_ids = %v, want [\"*\"]", i, entry["key_ids"])
		}
	}
}

// A template that allows nothing mints credentials that are refused everything. Failing
// here surfaces the misconfiguration at the gateway; succeeding would surface it as every
// user's AI quietly breaking.
func TestMintRefusesAPolicylessTemplate(t *testing.T) {
	srv, _ := gatewayWithTemplate(t, `{"virtual_key":{"id":"vk-x","provider_configs":[]}}`,
		http.StatusOK, mintedBody)

	if _, err := configured(srv.URL).Mint(context.Background(), 1); err == nil {
		t.Fatal("a template allowing no provider must fail the mint, not produce a dead key")
	}
}

// A ceiling is policy, and this change deliberately ships without one. Sending a zero
// would be read by the gateway as "no budget at all is allowed" rather than "no limit".
func TestMintSendsNoCeilingWhenNoneIsConfigured(t *testing.T) {
	srv, got := gateway(t, http.StatusOK, mintedBody)

	if _, err := configured(srv.URL).Mint(context.Background(), 1); err != nil {
		t.Fatalf("Mint: %v", err)
	}
	for _, field := range []string{"budgets", "rate_limit"} {
		if _, present := got.body[field]; present {
			t.Errorf("%s was sent as %v, want it omitted entirely when unconfigured", field, got.body[field])
		}
	}
}

func TestMintPassesAConfiguredCeilingThrough(t *testing.T) {
	srv, got := gateway(t, http.StatusOK, mintedBody)
	c := New(Config{
		BaseURL: srv.URL, AdminUsername: "admin", AdminPassword: "secret", TemplateKey: templateKey,
		MaxBudget: 2.5, RPMLimit: 60, BudgetWindow: "30d",
	})

	if _, err := c.Mint(context.Background(), 1); err != nil {
		t.Fatalf("Mint: %v", err)
	}
	budgets, _ := got.body["budgets"].([]any)
	if len(budgets) != 1 {
		t.Fatalf("budgets = %v, want the one configured ceiling", got.body["budgets"])
	}
	budget, _ := budgets[0].(map[string]any)
	if limit, _ := budget["max_limit"].(float64); limit != 2.5 {
		t.Errorf("max_limit = %v, want 2.5", budget["max_limit"])
	}
	if window, _ := budget["reset_duration"].(string); window != "30d" {
		t.Errorf("reset_duration = %v, want 30d", budget["reset_duration"])
	}
	rate, _ := got.body["rate_limit"].(map[string]any)
	if requests, _ := rate["request_max_limit"].(float64); requests != 60 {
		t.Errorf("request_max_limit = %v, want 60", rate["request_max_limit"])
	}
}

// A refusal must not read as a successful mint of the empty string: storing "" would
// mark the account as credentialled forever and send every later call out unattributed.
func TestMintReportsARefusalAsAnError(t *testing.T) {
	srv, _ := gateway(t, http.StatusUnauthorized, `{"error":"bad admin credentials"}`)

	cred, err := configured(srv.URL).Mint(context.Background(), 1)
	if err == nil {
		t.Fatal("Mint on a refusing gateway must fail")
	}
	if !errors.Is(err, ErrUpstream) {
		t.Errorf("error = %v, want it to wrap ErrUpstream so callers can tell whose fault it is", err)
	}
	if cred.Secret != "" || cred.ID != "" {
		t.Errorf("Mint returned %+v alongside an error, want nothing usable", cred)
	}
}

// A 2xx carrying half a credential is the same hazard as a refusal, and gateways do
// answer this way when a proxy in front of them rewrites a response. Either half missing
// is fatal: a secret with no id can never be revoked, an id with no secret can never
// spend, and both would be stored as though the account were credentialled.
func TestMintReportsAnIncompleteCredentialAsAnError(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"no secret", `{"virtual_key":{"id":"vk-42"}}`},
		{"no id", `{"virtual_key":{"value":"sk-bf-minted"}}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv, _ := gateway(t, http.StatusOK, tc.body)
			if _, err := configured(srv.URL).Mint(context.Background(), 1); err == nil {
				t.Fatal("a 2xx with half a credential must fail — storing it is worse than not minting")
			}
		})
	}
}

// Some governance routes answer with the key at the top level rather than wrapped.
// Depending on which would make the client fragile to a shape that carries no meaning.
func TestMintReadsAnUnwrappedAnswer(t *testing.T) {
	srv, _ := gateway(t, http.StatusOK, `{"id":"vk-42","value":"sk-bf-minted"}`)

	cred, err := configured(srv.URL).Mint(context.Background(), 1)
	if err != nil {
		t.Fatalf("Mint: %v", err)
	}
	if cred.ID != "vk-42" || cred.Secret != "sk-bf-minted" {
		t.Errorf("Mint = %+v, want the credential read from an unwrapped answer", cred)
	}
}

// The same status on an ADMINISTRATIVE call means our own admin credential is wrong.
// Reading that as a stale user key would turn one bad environment variable into a
// re-minting storm across every account — so the two must never be conflated.
func TestARefusedAdminCallIsNotAnUnknownUserKey(t *testing.T) {
	srv, _ := gateway(t, http.StatusUnauthorized, `{"error":"bad admin credentials"}`)

	_, err := configured(srv.URL).Mint(context.Background(), 1)
	if errors.Is(err, ErrUnknownKey) {
		t.Errorf("error = %v, want a plain upstream failure — a misconfigured administrator is not a stale user key", err)
	}
	if !errors.Is(err, ErrUpstream) {
		t.Errorf("error = %v, want it to wrap ErrUpstream", err)
	}
}

func TestActivityCountsWhatTheCredentialDid(t *testing.T) {
	// The two reads must answer DIFFERENTLY, or this test passes with the second call
	// deleted: the failure count comes from a read filtered to errors, where
	// total_requests IS the failures.
	got := &captured{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.path, got.method, got.query = r.URL.Path, r.Method, r.URL.RawQuery
		if r.URL.Query().Get("status") == "error" {
			_, _ = io.WriteString(w, `{"total_requests":2}`)
			return
		}
		_, _ = io.WriteString(w, `{"total_requests":128,"total_tokens":450000,"total_cost":1.25,"success_rate":98.4}`)
	}))
	t.Cleanup(srv.Close)

	from := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC)
	act, err := configured(srv.URL).Activity(context.Background(), "vk-42", from, to)
	if err != nil {
		t.Fatalf("Activity: %v", err)
	}
	if act.Requests != 128 || act.Tokens != 450000 {
		t.Errorf("Activity = %+v, want 128 calls and 450000 tokens", act)
	}
	// Read from the filtered call, not derived from success_rate: 126/128 is 98.4375%,
	// and turning a rounded percentage back into a count is arithmetic on a display value.
	if act.Failed != 2 {
		t.Errorf("Failed = %d, want 2 read from the error-filtered call", act.Failed)
	}
	if got.path != "/api/logs/stats" || got.method != http.MethodGet {
		t.Errorf("called %s %s, want GET /api/logs/stats", got.method, got.path)
	}
	// Scoped by the credential's own id. Asking by anything else reports somebody
	// else's month, or nobody's.
	if !strings.Contains(got.query, "virtual_key_ids=vk-42") {
		t.Errorf("query = %q, want the credential id", got.query)
	}
	// The last recorded call is the failure one, and it must carry the same window as
	// the total or the two numbers describe different months.
	if !strings.Contains(got.query, "status=error") {
		t.Errorf("query = %q, want the failure read filtered to errors", got.query)
	}
}

// The caller works in dates and the gateway in instants. A window taken literally would
// drop everything that happened earlier on the first day and later on the last.
func TestActivityWidensTheWindowToWholeDays(t *testing.T) {
	srv, got := gateway(t, http.StatusOK, `{"total_requests":0,"total_tokens":0}`)

	from := time.Date(2026, 8, 1, 13, 45, 0, 0, time.UTC)
	to := time.Date(2026, 8, 31, 9, 5, 0, 0, time.UTC)
	if _, err := configured(srv.URL).Activity(context.Background(), "vk-42", from, to); err != nil {
		t.Fatalf("Activity: %v", err)
	}
	if !strings.Contains(got.query, "start_time=2026-08-01T00%3A00%3A00Z") {
		t.Errorf("query = %q, want the window opened at the start of the first day", got.query)
	}
	// The last instant of the day, not the last whole second: an end of 23:59:59 would
	// drop everything in the final second of the month.
	if !strings.Contains(got.query, "end_time=2026-08-31T23%3A59%3A59.999999999Z") {
		t.Errorf("query = %q, want the window closed at the end of the last day", got.query)
	}
}

// An account that never made a call has no credential, and that is an answer rather than
// a fault: the usage page renders zeroes. Asking the gateway about an empty id would be a
// request that cannot mean anything.
func TestActivityOfAnAccountWithNoCredentialIsZeroWithoutAsking(t *testing.T) {
	srv, got := gateway(t, http.StatusInternalServerError, `{"error":"boom"}`)

	act, err := configured(srv.URL).Activity(context.Background(), "", time.Now(), time.Now())
	if err != nil {
		t.Fatalf("Activity: %v", err)
	}
	if act.Requests != 0 || act.Tokens != 0 {
		t.Errorf("Activity = %+v, want zeroes", act)
	}
	if got.path != "" {
		t.Errorf("called %s, want no request at all for an account with no credential", got.path)
	}
}

func TestActivityReportsAFailure(t *testing.T) {
	srv, _ := gateway(t, http.StatusInternalServerError, `{"error":"boom"}`)

	if _, err := configured(srv.URL).Activity(context.Background(), "vk-42", time.Now(), time.Now()); err == nil {
		t.Error("Activity must surface a gateway fault rather than report a false zero")
	}
}

// Blocking retires a credential while leaving it legible in the gateway's listings.
// It addresses the key by id: this gateway answers 404 to anything aimed at the secret.
func TestBlockNamesTheKeyById(t *testing.T) {
	srv, got := gateway(t, http.StatusOK, `{"id":"vk-42","is_active":false}`)

	if err := configured(srv.URL).Block(context.Background(), "vk-42"); err != nil {
		t.Fatalf("Block: %v", err)
	}
	if got.path != "/api/governance/virtual-keys/vk-42" || got.method != http.MethodPut {
		t.Errorf("called %s %s, want PUT /api/governance/virtual-keys/vk-42", got.method, got.path)
	}
	if active, ok := got.body["is_active"].(bool); !ok || active {
		t.Errorf("is_active = %v, want false — blocking deactivates rather than erases", got.body["is_active"])
	}
	if got.user != "admin" {
		t.Errorf("basic auth user = %q, want the administrator — a key cannot block itself", got.user)
	}
}

// A key the gateway has already forgotten is in the state asked for.
func TestBlockTreatsAnUnknownKeyAsDone(t *testing.T) {
	srv, _ := gateway(t, http.StatusNotFound, `{"error":"key not found"}`)

	if err := configured(srv.URL).Block(context.Background(), "vk-gone"); err != nil {
		t.Errorf("Block of an unknown key = %v, want nil", err)
	}
}

// An account with no id to name — one credentialled before the id column existed — has
// nothing to block. Sending the request anyway would aim it at an empty path.
func TestBlockWithoutAnIdIsANoop(t *testing.T) {
	srv, got := gateway(t, http.StatusInternalServerError, `{"error":"boom"}`)

	if err := configured(srv.URL).Block(context.Background(), ""); err != nil {
		t.Errorf("Block with no id = %v, want nil", err)
	}
	if got.path != "" {
		t.Errorf("called %s, want no request at all", got.path)
	}
}

func TestBlockReportsARealFailure(t *testing.T) {
	srv, _ := gateway(t, http.StatusInternalServerError, `{"error":"boom"}`)

	if err := configured(srv.URL).Block(context.Background(), "vk-42"); err == nil {
		t.Error("Block must surface a gateway fault; the caller decides whether to care")
	}
}

func TestDeleteNamesTheKeyById(t *testing.T) {
	srv, got := gateway(t, http.StatusOK, `{"deleted":true}`)

	if err := configured(srv.URL).Delete(context.Background(), "vk-42"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if got.path != "/api/governance/virtual-keys/vk-42" || got.method != http.MethodDelete {
		t.Errorf("called %s %s, want DELETE /api/governance/virtual-keys/vk-42", got.method, got.path)
	}
}

// Deleting a key the gateway has already forgotten is the outcome the caller wanted.
// Reporting it as a failure would make account deletion log a fault on every retry.
func TestDeleteTreatsAnUnknownKeyAsDone(t *testing.T) {
	srv, _ := gateway(t, http.StatusNotFound, `{"error":"key not found"}`)

	if err := configured(srv.URL).Delete(context.Background(), "vk-gone"); err != nil {
		t.Errorf("Delete of an unknown key = %v, want nil — it is already in the state asked for", err)
	}
}

func TestDeleteReportsARealFailure(t *testing.T) {
	srv, _ := gateway(t, http.StatusInternalServerError, `{"error":"boom"}`)

	if err := configured(srv.URL).Delete(context.Background(), "vk-42"); err == nil {
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
	if err := c.Delete(context.Background(), "vk-42"); err != nil {
		t.Errorf("Delete on an unconfigured gateway = %v, want nil — there is nothing to erase", err)
	}
	if err := c.Block(context.Background(), "vk-42"); err != nil {
		t.Errorf("Block on an unconfigured gateway = %v, want nil", err)
	}
	if _, err := c.Activity(context.Background(), "vk-42", time.Now(), time.Now()); err == nil {
		t.Error("Activity on an unconfigured gateway must fail rather than report a false zero")
	}
}

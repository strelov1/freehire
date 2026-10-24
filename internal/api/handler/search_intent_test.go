package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/ai/llmkey"
	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/llm"
	"github.com/strelov1/freehire/internal/search/searchintent"
)

// intentApp mounts the endpoint behind a middleware that signs the caller in when
// userID is non-zero and leaves a guest when it is zero, standing in for mw.optional.
func intentApp(t *testing.T, h *intentHandlers, userID int64) *fiber.App {
	t.Helper()
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Use(func(c *fiber.Ctx) error {
		if userID != 0 {
			c.Locals(auth.LocalsUserID, userID)
		}
		return c.Next()
	})
	app.Post("/api/v1/search/interpret", h.InterpretSearch)
	return app
}

// modelSayingJSON is a gateway that answers every completion with one canned body.
func modelSayingJSON(t *testing.T, content string) *llm.Client {
	t.Helper()
	quoted, err := json.Marshal(content)
	if err != nil {
		t.Fatalf("quote: %v", err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"id":"1","object":"chat.completion","choices":`+
			`[{"index":0,"message":{"role":"assistant","content":%s},"finish_reason":"stop"}]}`, quoted)
	}))
	t.Cleanup(srv.Close)
	client, err := llm.New(srv.URL, "sk-test", "test-model")
	if err != nil {
		t.Fatalf("llm.New: %v", err)
	}
	return client
}

func postIntent(t *testing.T, app *fiber.App, body string) (int, string) {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(),
		fiber.MethodPost, "/api/v1/search/interpret", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	out, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return resp.StatusCode, string(out)
}

// The cap must be enforced before the model is called, not after: a request past it is
// someone spending tokens, and paying for the call to find that out defeats the cap.
func TestInterpretSearchRefusesOverlongTextWithoutCallingTheModel(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"id":"1","object":"chat.completion","choices":`+
			`[{"index":0,"message":{"role":"assistant","content":"{}"},"finish_reason":"stop"}]}`)
	}))
	t.Cleanup(srv.Close)
	client, err := llm.New(srv.URL, "sk-test", "test-model")
	if err != nil {
		t.Fatalf("llm.New: %v", err)
	}

	h := newIntentHandlers(llmBinding{client: client})
	body, err := json.Marshal(map[string]string{"text": strings.Repeat("x", searchintent.MaxTextRunes+1)})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	status, _ := postIntent(t, intentApp(t, h, 7), string(body))
	if status != fiber.StatusBadRequest {
		t.Fatalf("status = %d, want 400", status)
	}
	if calls != 0 {
		t.Fatalf("model calls = %d, want 0 — the cap must be checked before spending", calls)
	}
}

func TestInterpretSearchReportsAnUnconfiguredModel(t *testing.T) {
	h := newIntentHandlers(llmBinding{})
	status, _ := postIntent(t, intentApp(t, h, 7), `{"text":"senior go"}`)
	if status != fiber.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503 — the feature is off, not the request wrong", status)
	}
}

func TestInterpretSearchReturnsTheResolvedFilter(t *testing.T) {
	h := newIntentHandlers(llmBinding{client: modelSayingJSON(t,
		`{"summary":"Senior Go roles in Portugal.","seniority":["senior"],"skills":["Golang"],`+
			`"countries":["Portugal"],"cities":["Atlantis City"]}`)})

	status, body := postIntent(t, intentApp(t, h, 7), `{"text":"senior go in portugal"}`)
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200: %s", status, body)
	}
	var out struct {
		Data struct {
			Summary    string              `json:"summary"`
			Facets     map[string][]string `json:"facets"`
			Unresolved []string            `json:"unresolved"`
			Empty      bool                `json:"empty"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatalf("decode %s: %v", body, err)
	}
	if out.Data.Summary != "Senior Go roles in Portugal." {
		t.Fatalf("summary = %q", out.Data.Summary)
	}
	if got := out.Data.Facets["skills"]; len(got) != 1 || got[0] != "go" {
		t.Fatalf("skills = %v, want [go]", got)
	}
	if len(out.Data.Unresolved) == 0 {
		t.Fatal("unresolved is empty — the invented city must be reported, not silently dropped")
	}
	if out.Data.Empty {
		t.Fatal("empty = true, want false")
	}
}

// A model that returns a proposal nothing resolves must reach the caller as "I could
// not turn that into filters" — never as an empty filter, which shows the whole
// catalogue and reads as "everything matches you".
func TestInterpretSearchFlagsAnEmptyResult(t *testing.T) {
	h := newIntentHandlers(llmBinding{client: modelSayingJSON(t,
		`{"summary":"anything","skills":["blockchain-adjacent"]}`)})

	status, body := postIntent(t, intentApp(t, h, 7), `{"text":"something odd"}`)
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200: %s", status, body)
	}
	var out struct {
		Data struct {
			Empty bool `json:"empty"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatalf("decode %s: %v", body, err)
	}
	if !out.Data.Empty {
		t.Fatal("empty = false, want true — nothing resolved")
	}
}

func TestInterpretSearchRefusesARequestWithNothingInIt(t *testing.T) {
	h := newIntentHandlers(llmBinding{client: modelSayingJSON(t, `{}`)})
	status, _ := postIntent(t, intentApp(t, h, 7), `{}`)
	if status != fiber.StatusBadRequest {
		t.Fatalf("status = %d, want 400", status)
	}
}

// Every per-user model call goes out on that user's own gateway credential under a
// feature tag. This one is no exception, and the tag has to be a constant beside the
// others rather than a string written here.
//
// The value is a bare word: the gateway names the dimension in the header, so a
// "feature:" prefix here would file the spend under "feature:search-intent" and split
// one feature's costs across two labels the day anybody wrote it the other way.
func TestSearchIntentHasItsOwnFeatureTag(t *testing.T) {
	if tagSearchIntent == "" || strings.Contains(tagSearchIntent, ":") {
		t.Fatalf("tagSearchIntent = %q, want a bare feature name", tagSearchIntent)
	}
	if tagSearchIntent == tagAssistant {
		t.Fatal("search intent must not be billed as assistant work")
	}
}

// capturingModel is a gateway that answers every completion with one canned body and
// records the credential and feature tag each call arrived with.
type capturingModel struct {
	mu       sync.Mutex
	auth     []string
	features []string
}

func (m *capturingModel) client(t *testing.T, content string) *llm.Client {
	t.Helper()
	quoted, err := json.Marshal(content)
	if err != nil {
		t.Fatalf("quote: %v", err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		m.auth = append(m.auth, r.Header.Get("Authorization"))
		m.features = append(m.features, r.Header.Get("x-bf-dim-feature"))
		m.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"id":"1","object":"chat.completion","choices":`+
			`[{"index":0,"message":{"role":"assistant","content":%s},"finish_reason":"stop"}]}`, quoted)
	}))
	t.Cleanup(srv.Close)
	client, err := llm.New(srv.URL, "sk-service", "test-model")
	if err != nil {
		t.Fatalf("llm.New: %v", err)
	}
	return client
}

func (m *capturingModel) calls() (auth, features []string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.auth...), append([]string(nil), m.features...)
}

// readCountingKeys is the resolver's store, counting every read per account. A read is
// the first half of For; the second half MINTS. Account 0 must never get either.
type readCountingKeys struct {
	*stubKeyQueries
	readMu sync.Mutex
	reads  map[int64]int
}

func newReadCountingKeys() *readCountingKeys {
	return &readCountingKeys{stubKeyQueries: newStubKeyQueries(), reads: map[int64]int{}}
}

func (r *readCountingKeys) GetUserLLMKey(ctx context.Context, id int64) (db.GetUserLLMKeyRow, error) {
	r.readMu.Lock()
	r.reads[id]++
	r.readMu.Unlock()
	return r.stubKeyQueries.GetUserLLMKey(ctx, id)
}

func (r *readCountingKeys) totalReads() int {
	r.readMu.Lock()
	defer r.readMu.Unlock()
	n := 0
	for _, c := range r.reads {
		n += c
	}
	return n
}

// intentBinding wires a configured resolver — a gateway that would mint — over keys, so a
// guest call that wandered into llmkey would be visible as a read rather than a silent
// no-op of an unconfigured resolver.
func intentBinding(t *testing.T, client *llm.Client, keys *readCountingKeys) llmBinding {
	t.Helper()
	gateway := llmkey.New(testGatewayConfig("http://127.0.0.1:1"))
	if gateway == nil {
		t.Fatal("llmkey.New returned nil for a complete configuration")
	}
	return llmBinding{client: client, keys: llmkey.NewResolver(keys, gateway)}
}

// A signed-out visitor is interpreted on the service credential, tagged with the feature —
// and the per-user resolver is never consulted, because consulting it for account 0 would
// read and then MINT a gateway credential for an account that does not exist.
func TestInterpretSearchServesAGuestOnTheServiceCredential(t *testing.T) {
	model := &capturingModel{}
	keys := newReadCountingKeys()
	h := newIntentHandlers(intentBinding(t, model.client(t,
		`{"summary":"Senior Go roles.","seniority":["senior"],"skills":["Golang"]}`), keys))

	status, body := postIntent(t, intentApp(t, h, 0), `{"text":"senior go"}`)
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200: %s", status, body)
	}
	auths, features := model.calls()
	if len(auths) == 0 {
		t.Fatal("the model was never called")
	}
	for i := range auths {
		if auths[i] != "Bearer sk-service" {
			t.Errorf("call %d spent as %q, want the service credential", i, auths[i])
		}
		if features[i] != tagSearchIntent {
			t.Errorf("call %d feature = %q, want %q", i, features[i], tagSearchIntent)
		}
	}
	if n := keys.totalReads(); n != 0 {
		t.Fatalf("resolver read %d credentials for a guest, want 0 — account 0 must never reach llmkey", n)
	}
}

// A signed-in caller is unchanged: the call spends on their own gateway credential.
func TestInterpretSearchSpendsAsTheSignedInCaller(t *testing.T) {
	model := &capturingModel{}
	keys := newReadCountingKeys()
	keys.stored[7], keys.storedIDs[7] = "sk-user-7", "vk-user-7"
	h := newIntentHandlers(intentBinding(t, model.client(t,
		`{"summary":"Senior Go roles.","seniority":["senior"],"skills":["Golang"]}`), keys))

	status, body := postIntent(t, intentApp(t, h, 7), `{"text":"senior go"}`)
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200: %s", status, body)
	}
	auths, features := model.calls()
	if len(auths) == 0 || auths[0] != "Bearer sk-user-7" || features[0] != tagSearchIntent {
		t.Fatalf("calls = %v / %v, want the caller's own credential tagged %q", auths, features, tagSearchIntent)
	}
}

// Drives the real register: a guest spends a per-IP budget of 10 an hour, a signed-in
// caller a per-user budget of 40, and the two are separate keys — a guest exhausting an
// address must not lock out a member behind it, nor the reverse.
func TestInterpretSearchBudgetsGuestsByIPAndMembersByUser(t *testing.T) {
	h := newIntentHandlers(llmBinding{client: modelSayingJSON(t,
		`{"summary":"Senior Go roles.","seniority":["senior"],"skills":["Golang"]}`)})
	app := fiber.New(fiber.Config{ErrorHandler: RenderError, ProxyHeader: fiber.HeaderXForwardedFor})
	optional := func(c *fiber.Ctx) error {
		if c.Get("X-Test-User") != "" {
			c.Locals(auth.LocalsUserID, int64(7))
		}
		return c.Next()
	}
	refuse := func(c *fiber.Ctx) error { return fiber.NewError(fiber.StatusUnauthorized, "auth gate ran") }
	h.register(app.Group("/api/v1"), middleware{
		optional: optional, cookie: refuse, key: refuse, throttler: newTestThrottler(t),
	})

	post := func(ip string, member bool) int {
		req := httptest.NewRequestWithContext(t.Context(), fiber.MethodPost,
			"/api/v1/search/interpret", strings.NewReader(`{"text":"senior go"}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set(fiber.HeaderXForwardedFor, ip)
		if member {
			req.Header.Set("X-Test-User", "1")
		}
		resp, err := app.Test(req, -1)
		if err != nil {
			t.Fatalf("request: %v", err)
		}
		defer resp.Body.Close()
		return resp.StatusCode
	}

	for i := 1; i <= guestSearchIntentsPerHour; i++ {
		if got := post("203.0.113.7", false); got != fiber.StatusOK {
			t.Fatalf("guest call %d = %d, want 200", i, got)
		}
	}
	if got := post("203.0.113.7", false); got != fiber.StatusTooManyRequests {
		t.Fatalf("guest call %d = %d, want 429", guestSearchIntentsPerHour+1, got)
	}
	if got := post("203.0.113.8", false); got != fiber.StatusOK {
		t.Fatalf("a guest on another address = %d, want 200 — the budget is per IP", got)
	}
	for i := 1; i <= searchIntentsPerHour; i++ {
		if got := post("203.0.113.7", true); got != fiber.StatusOK {
			t.Fatalf("member call %d from the exhausted address = %d, want 200 — budgets must not share a key", i, got)
		}
	}
	if got := post("203.0.113.9", true); got != fiber.StatusTooManyRequests {
		t.Fatalf("member call %d = %d, want 429 — the member budget is per user, not per IP", searchIntentsPerHour+1, got)
	}
}

// A guest may not post an encoded body: Fiber decompresses it with no ceiling, and with
// no account behind the request the per-IP budget does not bound one request's cost.
func TestInterpretSearchRefusesAGuestsEncodedBody(t *testing.T) {
	h := newIntentHandlers(llmBinding{client: modelSayingJSON(t, `{}`)})
	req := httptest.NewRequestWithContext(t.Context(), fiber.MethodPost,
		"/api/v1/search/interpret", strings.NewReader(`{"text":"senior go"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(fiber.HeaderContentEncoding, "gzip")
	resp, err := intentApp(t, h, 0).Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusUnsupportedMediaType {
		t.Fatalf("status = %d, want 415", resp.StatusCode)
	}
}

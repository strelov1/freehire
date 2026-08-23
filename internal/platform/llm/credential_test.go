package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/tmc/langchaingo/llms"
)

// headerProxy records the credential and tags of every request it serves, which is the
// only place the question "whose key did this call travel on" can be answered honestly —
// the client's own fields would report what it believes, not what it sent.
type headerProxy struct {
	srv *httptest.Server

	mu     sync.Mutex
	authz  []string
	tags   []string
	preset []string
}

func newHeaderProxy(t *testing.T) *headerProxy {
	t.Helper()
	p := &headerProxy{}
	p.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := map[string]any{}
		_ = json.NewDecoder(r.Body).Decode(&body)

		p.mu.Lock()
		p.authz = append(p.authz, r.Header.Get("Authorization"))
		p.tags = append(p.tags, r.Header.Get("X-Bf-Dim-Feature"))
		p.preset = append(p.preset, r.Header.Get("X-Bf-Dim-Preset"))
		p.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"id":"1","object":"chat.completion","choices":`+
			`[{"index":0,"message":{"role":"assistant","content":"{}"},"finish_reason":"stop"}]}`)
	}))
	t.Cleanup(p.srv.Close)
	return p
}

func (p *headerProxy) seen(t *testing.T) ([]string, []string) {
	t.Helper()
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.authz...), append([]string(nil), p.tags...)
}

func (p *headerProxy) client(t *testing.T) *Client {
	t.Helper()
	c, err := New(p.srv.URL, "sk-service", "test-model")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c
}

// THE test of this change. A schema-bound model is cached under the schema's name and
// rendered shape — NOT under the credential that built it. A clone that changed the token
// while sharing that cache would serve one user's schema-bound call on the model built
// with another user's token, and every assertion about the response would still pass.
//
// Both calls ask for the SAME schema under the SAME name, which is precisely the case
// that collides in the cache.
func TestAsDoesNotShareSchemaBoundModelsAcrossCredentials(t *testing.T) {
	proxy := newHeaderProxy(t)
	base := proxy.client(t)
	schema := testSchema(t)

	alice := base.As("sk-alice", nil, Feature("tailor"))
	bob := base.As("sk-bob", nil, Feature("tailor"))

	if _, err := alice.GenerateJSON(context.Background(), "sys", "usr", WithSchema("shape", schema)); err != nil {
		t.Fatalf("alice: %v", err)
	}
	if _, err := bob.GenerateJSON(context.Background(), "sys", "usr", WithSchema("shape", schema)); err != nil {
		t.Fatalf("bob: %v", err)
	}

	authz, _ := proxy.seen(t)
	if len(authz) != 2 {
		t.Fatalf("proxy served %d requests, want 2", len(authz))
	}
	if authz[0] != "Bearer sk-alice" {
		t.Errorf("first call carried %q, want alice's credential", authz[0])
	}
	if authz[1] != "Bearer sk-bob" {
		t.Errorf("second call carried %q, want bob's credential — a shared schema-model cache "+
			"would hand bob the model built with alice's token", authz[1])
	}
}

// The plain path holds the credential too: the langchaingo model itself is built with the
// token, so re-crediting has to rebuild it and not only the schema cache.
func TestAsRecreditsThePlainPath(t *testing.T) {
	proxy := newHeaderProxy(t)
	base := proxy.client(t)

	if _, err := base.As("sk-alice", nil, Feature("chat")).GenerateJSON(context.Background(), "sys", "usr"); err != nil {
		t.Fatalf("GenerateJSON: %v", err)
	}

	authz, _ := proxy.seen(t)
	if authz[0] != "Bearer sk-alice" {
		t.Errorf("call carried %q, want the credential it was cloned onto", authz[0])
	}
}

// Each dimension travels in its own header, and the feature one carries the feature
// ALONE. This is the whole reason Dimension exists: the previous gateway took a
// comma-separated list of key:value pairs in one header and split them itself, and
// carrying that shape across filed the assistant under "assistant,preset:tailor" — a
// value that is neither the feature nor the preset, and that splits one surface across as
// many labels as it has presets.
func TestAsSendsOneHeaderPerDimension(t *testing.T) {
	proxy := newHeaderProxy(t)
	base := proxy.client(t)

	if _, err := base.As("sk-alice", nil, Feature("assistant"), Dimension{Name: "preset", Value: "tailor"}).
		GenerateJSON(context.Background(), "sys", "usr"); err != nil {
		t.Fatalf("GenerateJSON: %v", err)
	}

	proxy.mu.Lock()
	defer proxy.mu.Unlock()
	if proxy.tags[0] != "assistant" {
		t.Errorf("feature = %q, want the feature alone — a value carrying the preset is a label nobody can group by", proxy.tags[0])
	}
	if proxy.preset[0] != "tailor" {
		t.Errorf("preset = %q, want the preset under its own dimension", proxy.preset[0])
	}
}

// Attribution fails open, and the feature tag must survive that: knowing WHICH feature
// spent is useful even when we could not work out whose account it was.
func TestAsWithoutACredentialStillTags(t *testing.T) {
	proxy := newHeaderProxy(t)
	base := proxy.client(t)

	if _, err := base.As("", nil, Feature("match")).GenerateJSON(context.Background(), "sys", "usr"); err != nil {
		t.Fatalf("GenerateJSON: %v", err)
	}

	authz, tags := proxy.seen(t)
	if authz[0] != "Bearer sk-service" {
		t.Errorf("call carried %q, want the service credential when no user credential resolved", authz[0])
	}
	if tags[0] != "match" {
		t.Errorf("tags = %q, want the feature tag even on an unattributed call", tags[0])
	}
}

// Asking for nothing must change nothing, so a call site can pass through values it did
// not check without acquiring a header it never had.
func TestAsWithNeitherCredentialNorTagsIsTheSameClient(t *testing.T) {
	proxy := newHeaderProxy(t)
	base := proxy.client(t)

	if got := base.As("", nil); got != base {
		t.Error("As with nothing to change should return the receiver rather than a rebuilt clone")
	}
}

func TestAsIsNilSafe(t *testing.T) {
	var c *Client
	if got := c.As("sk-alice", nil, Feature("chat")); got != nil {
		t.Errorf("As on a nil client = %v, want nil", got)
	}
}

func TestAsKeepsTheModelAndTimeout(t *testing.T) {
	proxy := newHeaderProxy(t)
	base := proxy.client(t).WithTimeout(1234)

	clone := base.As("sk-alice", nil, Feature("chat"))
	if clone.ModelID() != base.ModelID() {
		t.Errorf("model = %q, want %q", clone.ModelID(), base.ModelID())
	}
	if clone.timeout != base.timeout {
		t.Errorf("timeout = %v, want %v — re-crediting must not reset an adjusted deadline", clone.timeout, base.timeout)
	}
}

// An injected model is the tests-only seam and has no endpoint to rebuild against, so
// re-crediting it is ignored the way a schema there is ignored — never fatal.
func TestAsOnAnInjectedModelIsAPassthrough(t *testing.T) {
	injected := NewWithModel(stubModel{})
	if got := injected.As("sk-alice", nil, Feature("chat")); got != injected {
		t.Error("As on an injected model should return the receiver, not a client with no endpoint")
	}
}

// refusingProxy answers 401 to one nominated credential and 200 to everything else, so a
// test can play out a gateway that has forgotten a user's key.
func newRefusingProxy(t *testing.T, refuse string) *headerProxy {
	t.Helper()
	p := &headerProxy{}
	p.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p.mu.Lock()
		p.authz = append(p.authz, r.Header.Get("Authorization"))
		p.tags = append(p.tags, r.Header.Get("X-Bf-Dim-Feature"))
		p.preset = append(p.preset, r.Header.Get("X-Bf-Dim-Preset"))
		p.mu.Unlock()

		if r.Header.Get("Authorization") == "Bearer "+refuse {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = fmt.Fprint(w, `{"error":"invalid key"}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"id":"1","object":"chat.completion","choices":`+
			`[{"index":0,"message":{"role":"assistant","content":"{}"},"finish_reason":"stop"}]}`)
	}))
	t.Cleanup(p.srv.Close)
	return p
}

// A gateway that has forgotten a user's credential must not cost that user their answer.
// The refusal is invisible to them: the call is re-made on the service credential and
// completes, and the stale value is reported so it can be replaced.
func TestARefusedCredentialFallsBackAndReportsItself(t *testing.T) {
	proxy := newRefusingProxy(t, "sk-stale")
	base := proxy.client(t)

	var refused int
	clone := base.As("sk-stale", func() { refused++ }, Feature("chat"))

	if _, err := clone.GenerateJSON(context.Background(), "sys", "usr"); err != nil {
		t.Fatalf("a refused user credential must not fail the call: %v", err)
	}
	if refused != 1 {
		t.Errorf("refusal reported %d times, want once so the stale credential is replaced", refused)
	}

	authz, tags := proxy.seen(t)
	if len(authz) != 2 {
		t.Fatalf("proxy served %d requests, want the refused one and the retry", len(authz))
	}
	if authz[0] != "Bearer sk-stale" || authz[1] != "Bearer sk-service" {
		t.Errorf("credentials = %v, want the user's then the service one", authz)
	}
	if tags[1] != "chat" {
		t.Errorf("retry tags = %q, want the feature still named on the fallback", tags[1])
	}
}

// Chaining As on its own output must not turn a later user's fallback into an
// earlier user's credential. Nothing calls As twice today (each handler credits the
// base client held at construction exactly once), but the method has to defend this
// structurally rather than rely on that discipline holding forever.
func TestAsChainedTwiceFallsBackToTheServiceCredentialNotAPriorUser(t *testing.T) {
	proxy := newRefusingProxy(t, "sk-bob")
	base := proxy.client(t)

	alice := base.As("sk-alice", nil, Feature("tailor"))
	var refused int
	bob := alice.As("sk-bob", func() { refused++ }, Feature("tailor"))

	if _, err := bob.GenerateJSON(context.Background(), "sys", "usr"); err != nil {
		t.Fatalf("a refused user credential must not fail the call: %v", err)
	}
	if refused != 1 {
		t.Errorf("refusal reported %d times, want once", refused)
	}

	authz, _ := proxy.seen(t)
	if len(authz) != 2 {
		t.Fatalf("proxy served %d requests, want the refused one and the retry", len(authz))
	}
	if authz[0] != "Bearer sk-bob" {
		t.Fatalf("first call carried %q, want bob's credential", authz[0])
	}
	if authz[1] != "Bearer sk-service" {
		t.Errorf("retry carried %q, want the service credential — not alice's, "+
			"which a double-clone bug would leave as bob's fallback", authz[1])
	}
}

// One retry, not a loop: a gateway refusing the service credential too is a
// misconfiguration, and hammering it would turn one bad key into a request storm.
func TestARefusalIsRetriedOnlyOnce(t *testing.T) {
	proxy := newRefusingProxy(t, "sk-service") // and the user credential is refused too
	base, err := New(proxy.srv.URL, "sk-service", "test-model")
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	_, _ = base.As("sk-service", nil, Feature("chat")).GenerateJSON(context.Background(), "sys", "usr")

	served, _ := proxy.seen(t)
	if len(served) > 2 {
		t.Errorf("proxy served %d requests, want at most the original and one retry", len(served))
	}
}

// The retry exists for a credential the gateway has FORGOTTEN, and for nothing else.
//
// A spend ceiling is refused with 429 (litellm.BudgetExceededError), and retrying that on
// the service credential would spend exactly the money the ceiling was placed to stop —
// making the fuse decorative the moment anyone armed it. Only 401 is retried, and this
// test pins that rather than trusting an upstream constant to stay put.
func TestACeilingRefusalIsNotRetried(t *testing.T) {
	var served int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		served++
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = fmt.Fprint(w, `{"error":{"message":"ExceededBudget: Key over 30d budget."}}`)
	}))
	t.Cleanup(srv.Close)

	base, err := New(srv.URL, "sk-service", "test-model")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	var refused int
	_, _ = base.As("sk-capped", func() { refused++ }, Feature("chat")).
		GenerateJSON(context.Background(), "sys", "usr")

	if served != 1 {
		t.Errorf("proxy served %d requests, want exactly one — a ceiling must not be retried past", served)
	}
	if refused != 0 {
		t.Errorf("refusal reported %d times, want none — the credential is fine, the budget is spent", refused)
	}
}

// Without a user credential there is nothing to fall back FROM, so a refusal is the
// caller's problem to see rather than something to paper over with a second request.
func TestAnUnattributedCallIsNotRetried(t *testing.T) {
	proxy := newRefusingProxy(t, "sk-service")
	base := proxy.client(t)

	_, _ = base.As("", nil, Feature("chat")).GenerateJSON(context.Background(), "sys", "usr")

	authz, _ := proxy.seen(t)
	if len(authz) != 1 {
		t.Errorf("proxy served %d requests, want exactly one — there is no other credential to try", len(authz))
	}
}

// stubModel satisfies llms.Model for the injected-model seam.
type stubModel struct{}

func (stubModel) Call(context.Context, string, ...llms.CallOption) (string, error) { return "", nil }

func (stubModel) GenerateContent(context.Context, []llms.MessageContent, ...llms.CallOption) (*llms.ContentResponse, error) {
	return &llms.ContentResponse{Choices: []*llms.ContentChoice{{Content: "{}"}}}, nil
}

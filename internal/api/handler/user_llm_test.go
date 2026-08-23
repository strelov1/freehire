package handler

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/tmc/langchaingo/llms"

	"github.com/strelov1/freehire/internal/ai/assistant"
	"github.com/strelov1/freehire/internal/ai/llmkey"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/llm"
)

// spendProxy is the model endpoint, recording whose credential and which tags each call
// carried — the only honest place to check attribution, since the client's own fields
// report what it believes rather than what it sent.
type spendProxy struct {
	srv *httptest.Server

	mu     sync.Mutex
	authz  []string
	tags   []string
	preset []string
}

func newSpendProxy(t *testing.T) *spendProxy {
	t.Helper()
	p := &spendProxy{}
	p.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

// keyGateway is the admin API, minting a fixed secret.
// newKeyGateway answers the two calls a mint makes: the policy read, then the create.
// The policy has to be there — a virtual key created without one is refused every
// provider, so a fake that skipped it would let a mint "succeed" into a dead credential.
func newKeyGateway(t *testing.T, mint string) *httptest.Server {
	t.Helper()
	const keys = "/api/governance/virtual-keys"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == keys+"/"+testTemplateKey:
			_, _ = io.WriteString(w, `{"virtual_key":{"id":"`+testTemplateKey+`","provider_configs":[`+
				`{"provider":"zai","weight":1,"allowed_models":["flagship"],"key_ids":["*"]}]}}`)
		case r.Method == http.MethodPost && r.URL.Path == keys:
			_, _ = io.WriteString(w, `{"virtual_key":{"id":"vk-`+mint+`","value":"`+mint+`"}}`)
		default:
			_, _ = io.WriteString(w, `{}`)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// testTemplateKey names the virtual key these fakes serve a provider policy from.
const testTemplateKey = "vk-freehire-service"

// testGatewayConfig is the admin configuration every handler test uses. Spelling it once
// keeps a new required field from having to be added at each call site.
func testGatewayConfig(url string) llmkey.Config {
	return llmkey.Config{
		BaseURL: url, AdminUsername: "admin", AdminPassword: "secret", TemplateKey: testTemplateKey,
	}
}

// stubKeyQueries is the smallest store the resolver needs.
type stubKeyQueries struct {
	mu        sync.Mutex
	stored    map[int64]string
	storedIDs map[int64]string
}

func newStubKeyQueries() *stubKeyQueries {
	return &stubKeyQueries{stored: map[int64]string{}, storedIDs: map[int64]string{}}
}

func (s *stubKeyQueries) GetUserLLMKey(_ context.Context, id int64) (db.GetUserLLMKeyRow, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return db.GetUserLLMKeyRow{LlmKey: s.stored[id], LlmKeyID: s.storedIDs[id]}, nil
}

func (s *stubKeyQueries) ClaimUserLLMKey(_ context.Context, arg db.ClaimUserLLMKeyParams) (db.ClaimUserLLMKeyRow, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stored[arg.ID] = arg.LlmKey.String
	s.storedIDs[arg.ID] = arg.LlmKeyID.String
	return db.ClaimUserLLMKeyRow{LlmKey: arg.LlmKey.String, LlmKeyID: arg.LlmKeyID.String}, nil
}

func (s *stubKeyQueries) ClearUserLLMKey(_ context.Context, arg db.ClearUserLLMKeyParams) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.stored, arg.ID)
	delete(s.storedIDs, arg.ID)
	return nil
}

func (p *spendProxy) llmClient(t *testing.T) *llm.Client {
	t.Helper()
	c, err := llm.New(p.srv.URL, "sk-service", "test-model")
	if err != nil {
		t.Fatalf("llm.New: %v", err)
	}
	return c
}

func TestUserLLMSpendsUnderTheCallersOwnCredential(t *testing.T) {
	proxy := newSpendProxy(t)
	keys := llmkey.NewResolver(newStubKeyQueries(),
		llmkey.New(testGatewayConfig(newKeyGateway(t, "sk-user-7").URL)))

	client := userLLM(context.Background(), keys, proxy.llmClient(t), 7, llm.Feature("assistant"), llm.Dimension{Name: "preset", Value: "chat"})
	if _, err := client.GenerateJSON(context.Background(), "sys", "usr"); err != nil {
		t.Fatalf("GenerateJSON: %v", err)
	}

	proxy.mu.Lock()
	defer proxy.mu.Unlock()
	if proxy.authz[0] != "Bearer sk-user-7" {
		t.Errorf("call carried %q, want the caller's own credential", proxy.authz[0])
	}
	// One header per dimension, and the feature one carries the feature alone. A value
	// of "assistant,preset:chat" would be neither, and would split the assistant across
	// as many labels as it has presets.
	if proxy.tags[0] != "assistant" {
		t.Errorf("feature = %q, want the feature alone", proxy.tags[0])
	}
	if proxy.preset[0] != "chat" {
		t.Errorf("preset = %q, want the preset under its own dimension", proxy.preset[0])
	}
}

// The whole fail-open rule in one test: no admin API configured means no attribution, and
// the call must still go out — tagged, so the feature is known even when the person is not.
func TestUserLLMFallsBackToTheServiceCredential(t *testing.T) {
	proxy := newSpendProxy(t)
	keys := llmkey.NewResolver(newStubKeyQueries(), nil) // unconfigured deployment

	client := userLLM(context.Background(), keys, proxy.llmClient(t), 7, llm.Feature("match"))
	if _, err := client.GenerateJSON(context.Background(), "sys", "usr"); err != nil {
		t.Fatalf("GenerateJSON: %v", err)
	}

	proxy.mu.Lock()
	defer proxy.mu.Unlock()
	if proxy.authz[0] != "Bearer sk-service" {
		t.Errorf("call carried %q, want the service credential", proxy.authz[0])
	}
	if proxy.tags[0] != "match" {
		t.Errorf("tags = %q, want the feature named even unattributed", proxy.tags[0])
	}
}

// The typed-nil trap, caught in production by the integration suite and not by a unit test
// that passed an untyped nil. userLLM returns *llm.Client; assigning a nil one into the
// runner's Model INTERFACE produces a non-nil interface holding a nil pointer, and the
// runner then dereferences it on its first round — inside the stream goroutine, after the
// response has begun.
func TestBoundRunnerKeepsTheOriginalWhenNoClientResolves(t *testing.T) {
	original := assistant.NewRunner(&nilSafeModel{}, assistant.NewStore(nil), assistant.RunnerConfig{MaxSteps: 3})
	h := &assistantHandlers{
		runner: original,
		keys:   llmkey.NewResolver(newStubKeyQueries(), nil),
		// llm is nil: this deployment has no assistant model client to re-credential.
	}

	if got := h.boundRunner(context.Background(), assistant.Session{UserID: 7, Preset: assistant.PresetChat}); got != original {
		t.Error("an unresolved client replaced the runner's model with a typed nil; the turn would panic mid-stream")
	}
}

// nilSafeModel stands in for the assistant's model in a runner nobody is going to run.
type nilSafeModel struct{}

func (nilSafeModel) Chat(context.Context, []llms.MessageContent, []llms.Tool, llm.ChatStream) (*llms.ContentChoice, error) {
	return &llms.ContentChoice{Content: "unused"}, nil
}

func TestUserLLMOnAnUnconfiguredModelStaysNil(t *testing.T) {
	keys := llmkey.NewResolver(newStubKeyQueries(), nil)
	if got := userLLM(context.Background(), keys, nil, 7, llm.Feature("chat")); got != nil {
		t.Errorf("userLLM with no model = %v, want nil so callers keep reporting the feature off", got)
	}
}

// A cancelled request must still be able to clean up a credential the gateway rejected:
// the clean-up runs on a context detached from the caller's.
func TestUserLLMForgetsARejectedCredentialAfterCancellation(t *testing.T) {
	refusing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "Bearer sk-stale" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, `{"error":"invalid key"}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"id":"1","object":"chat.completion","choices":`+
			`[{"index":0,"message":{"role":"assistant","content":"{}"},"finish_reason":"stop"}]}`)
	}))
	t.Cleanup(refusing.Close)

	store := newStubKeyQueries()
	store.stored[7] = "sk-stale"
	keys := llmkey.NewResolver(store,
		llmkey.New(testGatewayConfig(newKeyGateway(t, "sk-fresh").URL)))

	client, err := llm.New(refusing.URL, "sk-service", "test-model")
	if err != nil {
		t.Fatalf("llm.New: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	bound := userLLM(ctx, keys, client, 7, llm.Feature("chat"))
	cancel() // the caller walks away as the refusal comes back

	if _, err := bound.GenerateJSON(context.Background(), "sys", "usr"); err != nil {
		t.Fatalf("a refused credential must not fail the call: %v", err)
	}

	store.mu.Lock()
	defer store.mu.Unlock()
	if store.stored[7] != "" {
		t.Errorf("stored %q, want the rejected credential cleared despite the cancellation", store.stored[7])
	}
}

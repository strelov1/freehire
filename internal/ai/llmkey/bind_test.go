package llmkey

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/strelov1/freehire/internal/platform/llm"
)

// spendProxy is the model endpoint, recording whose credential and which feature dimension
// each call carried — the only honest place to check attribution, since the client's own
// fields report what it believes rather than what it sent.
type spendProxy struct {
	srv *httptest.Server

	mu      sync.Mutex
	authz   []string
	feature []string
}

func newSpendProxy(t *testing.T) *spendProxy {
	t.Helper()
	p := &spendProxy{}
	p.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p.mu.Lock()
		p.authz = append(p.authz, r.Header.Get("Authorization"))
		p.feature = append(p.feature, r.Header.Get("X-Bf-Dim-Feature"))
		p.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"id":"1","object":"chat.completion","choices":`+
			`[{"index":0,"message":{"role":"assistant","content":"{}"},"finish_reason":"stop"}]}`)
	}))
	t.Cleanup(p.srv.Close)
	return p
}

func (p *spendProxy) llmClient(t *testing.T) *llm.Client {
	t.Helper()
	c, err := llm.New(p.srv.URL, "sk-service", "test-model")
	if err != nil {
		t.Fatalf("llm.New: %v", err)
	}
	return c
}

func TestBindSpendsUnderTheCallersOwnCredential(t *testing.T) {
	proxy := newSpendProxy(t)
	r := testResolver(t, newFakeQueries(), &routedGateway{mints: []string{"sk-user-7"}})

	client := Bind(context.Background(), r, proxy.llmClient(t), 7, llm.Feature("assistant"))
	if _, err := client.GenerateJSON(context.Background(), "sys", "usr"); err != nil {
		t.Fatalf("GenerateJSON: %v", err)
	}

	proxy.mu.Lock()
	defer proxy.mu.Unlock()
	if proxy.authz[0] != "Bearer sk-user-7" {
		t.Errorf("call carried %q, want the caller's own credential", proxy.authz[0])
	}
	if proxy.feature[0] != "assistant" {
		t.Errorf("feature = %q, want the feature dimension", proxy.feature[0])
	}
}

// The whole fail-open rule in one test: no admin API configured means no attribution, and
// the call must still go out — tagged, so the feature is known even when the person is not.
func TestBindFallsBackToTheServiceCredential(t *testing.T) {
	proxy := newSpendProxy(t)
	r := NewResolver(newFakeQueries(), nil) // unconfigured deployment

	client := Bind(context.Background(), r, proxy.llmClient(t), 7, llm.Feature("match"))
	if _, err := client.GenerateJSON(context.Background(), "sys", "usr"); err != nil {
		t.Fatalf("GenerateJSON: %v", err)
	}

	proxy.mu.Lock()
	defer proxy.mu.Unlock()
	if proxy.authz[0] != "Bearer sk-service" {
		t.Errorf("call carried %q, want the service credential", proxy.authz[0])
	}
	if proxy.feature[0] != "match" {
		t.Errorf("feature = %q, want the feature named even unattributed", proxy.feature[0])
	}
}

func TestBindOnAnUnconfiguredModelStaysNil(t *testing.T) {
	r := NewResolver(newFakeQueries(), nil)
	if got := Bind(context.Background(), r, nil, 7, llm.Feature("chat")); got != nil {
		t.Errorf("Bind with no model = %v, want nil so callers keep reporting the feature off", got)
	}
}

// A cancelled request must still be able to clean up a credential the gateway rejected:
// the clean-up runs on a context detached from the caller's.
func TestBindForgetsARejectedCredentialAfterCancellation(t *testing.T) {
	refusing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "Bearer sk-stale" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = fmt.Fprint(w, `{"error":"invalid key"}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"id":"1","object":"chat.completion","choices":`+
			`[{"index":0,"message":{"role":"assistant","content":"{}"},"finish_reason":"stop"}]}`)
	}))
	t.Cleanup(refusing.Close)

	q := newFakeQueries()
	q.stored[7] = "sk-stale"
	r := testResolver(t, q, &routedGateway{mints: []string{"sk-fresh"}})

	client, err := llm.New(refusing.URL, "sk-service", "test-model")
	if err != nil {
		t.Fatalf("llm.New: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	bound := Bind(ctx, r, client, 7, llm.Feature("chat"))
	cancel() // the caller walks away as the refusal comes back

	if _, err := bound.GenerateJSON(context.Background(), "sys", "usr"); err != nil {
		t.Fatalf("a refused credential must not fail the call: %v", err)
	}

	q.mu.Lock()
	defer q.mu.Unlock()
	if q.stored[7] != "" {
		t.Errorf("stored %q, want the rejected credential cleared despite the cancellation", q.stored[7])
	}
}

package resumeextract

import (
	"testing"

	"github.com/strelov1/freehire/internal/platform/llm"
)

// As must attribute the extraction to the candidate WITHOUT swapping the client this
// extractor was built with — which is the configuration the extraction depends on, its 120s
// budget above all.
//
// The model id stands in for that client's identity here: it is the one setting readable
// from outside the llm package, and an extractor running on somebody else's client would
// report somebody else's model. The timeout itself is asserted where it lives, in
// llm.TestAsCallerKeepsTheTimeoutTheClientWasBuiltWith.
func TestAsKeepsTheExtractorsOwnClient(t *testing.T) {
	client, err := llm.New("https://gateway.example", "service-key", "the-configured-model")
	if err != nil {
		t.Fatalf("llm.New: %v", err)
	}
	e := NewExtractor(client, nil)

	bound := e.As(llm.NewCaller("a-candidates-own-key", nil, llm.Feature("cv-extract")))

	if got := bound.ModelID(); got != "the-configured-model" {
		t.Errorf("ModelID after binding a caller = %q, want the extractor's own model", got)
	}
}

// A nil extractor is the unconfigured deployment, and binding a caller to it must stay a
// no-op rather than panic on a path that runs in a detached goroutine.
func TestAsIsNilSafe(t *testing.T) {
	var e *Extractor
	if got := e.As(llm.NewCaller("k", nil)); got != nil {
		t.Errorf("As on a nil extractor = %v, want nil", got)
	}
}

// An extractor with no client (no gateway configured) must survive attribution too: there is
// nothing to bind a credential to, and Extract already degrades to ErrDisabled.
func TestAsWithoutAClientStaysDisabled(t *testing.T) {
	e := NewExtractor(nil, nil)

	bound := e.As(llm.NewCaller("k", nil, llm.Feature("cv-extract")))

	if bound == nil {
		t.Fatal("As returned nil for a client-less extractor; the caller would panic on it")
	}
	if got := bound.ModelID(); got != "" {
		t.Errorf("ModelID = %q, want empty with no gateway configured", got)
	}
}

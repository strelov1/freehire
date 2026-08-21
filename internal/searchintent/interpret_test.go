package searchintent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/llm"
)

// modelSaying stands in for the gateway, answering every completion with one canned
// proposal. The point under test is what this package does with a model's answer, not
// how it talks to one.
func modelSaying(t *testing.T, said proposal) *Interpreter {
	t.Helper()
	body, err := json.Marshal(said)
	if err != nil {
		t.Fatalf("marshal proposal: %v", err)
	}
	content, err := json.Marshal(string(body))
	if err != nil {
		t.Fatalf("quote proposal: %v", err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"id":"1","object":"chat.completion","choices":`+
			`[{"index":0,"message":{"role":"assistant","content":%s},"finish_reason":"stop"}]}`, content)
	}))
	t.Cleanup(srv.Close)

	client, err := llm.New(srv.URL, "sk-test", "test-model")
	if err != nil {
		t.Fatalf("llm.New: %v", err)
	}
	return NewInterpreter(client)
}

func TestInterpretResolvesWhatTheModelProposed(t *testing.T) {
	in := modelSaying(t, proposal{
		Summary:   "Senior Go backend roles in Portugal.",
		Seniority: []string{"senior"},
		Category:  []string{"backend"},
		Skills:    []string{"Golang"},
		Countries: []string{"Portugal"},
	})

	got, err := in.Interpret(context.Background(), Request{Text: "senior go backend in portugal"})
	if err != nil {
		t.Fatalf("Interpret: %v", err)
	}
	if !slices.Equal(got.Facets["skills"], []string{"go"}) {
		t.Fatalf("skills = %v, want [go] — the alias must be canonicalised", got.Facets["skills"])
	}
	if !slices.Equal(got.Facets["countries"], []string{"pt"}) {
		t.Fatalf("countries = %v, want [pt]", got.Facets["countries"])
	}
	if got.Summary != "Senior Go backend roles in Portugal." {
		t.Fatalf("summary = %q, want the model's sentence", got.Summary)
	}
}

// The summary must come from the same response as the values. A second call to
// describe the result could describe a different search, and the caller — who is shown
// the sentence and not the facets — would have no way to tell.
func TestInterpretMakesOneModelCall(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"id":"1","object":"chat.completion","choices":`+
			`[{"index":0,"message":{"role":"assistant","content":"{\"summary\":\"anything\"}"},"finish_reason":"stop"}]}`)
	}))
	t.Cleanup(srv.Close)
	client, err := llm.New(srv.URL, "sk-test", "test-model")
	if err != nil {
		t.Fatalf("llm.New: %v", err)
	}

	if _, err := NewInterpreter(client).Interpret(context.Background(), Request{Text: "anything"}); err != nil {
		t.Fatalf("Interpret: %v", err)
	}
	if calls != 1 {
		t.Fatalf("model calls = %d, want 1", calls)
	}
}

func TestInterpretWithoutAModelIsDisabled(t *testing.T) {
	_, err := NewInterpreter(nil).Interpret(context.Background(), Request{Text: "anything"})
	if !errors.Is(err, ErrDisabled) {
		t.Fatalf("err = %v, want ErrDisabled", err)
	}
}

func TestInterpretRefusesAnEmptyRequest(t *testing.T) {
	in := modelSaying(t, proposal{})
	if _, err := in.Interpret(context.Background(), Request{}); err == nil {
		t.Fatal("Interpret accepted a request with nothing to interpret")
	}
}

// The schema and the resolver table are two descriptions of one vocabulary, and the
// dangerous drift is silent in both directions: a facet the schema omits is one the
// model can never ask for, and a facet the schema offers but nothing resolves is a
// value the caller is told was not understood every single time.
func TestProposalAndResolversDescribeTheSameVocabulary(t *testing.T) {
	offered := proposal{}.intent().Facets
	for name := range facetResolvers {
		if _, ok := offered[name]; !ok {
			t.Errorf("facetResolvers resolves %q, but the model has no field to propose it", name)
		}
	}
	for name := range offered {
		if _, ok := facetResolvers[name]; !ok {
			t.Errorf("the model may propose %q, but nothing resolves it", name)
		}
	}
}

// Every closed vocabulary must reach the model as an enum. Naming the values in prose
// and hoping is what the schema exists to replace.
func TestSchemaConstrainsClosedVocabularies(t *testing.T) {
	schema, err := requestSchema()
	if err != nil {
		t.Fatalf("requestSchema: %v", err)
	}
	raw, err := json.Marshal(schema)
	if err != nil {
		t.Fatalf("marshal schema: %v", err)
	}
	for _, value := range []string{"people_manager", "c_level", "climatetech", "hybrid"} {
		if !strings.Contains(string(raw), value) {
			t.Errorf("schema does not enumerate %q, so the model is free to invent around it", value)
		}
	}
}

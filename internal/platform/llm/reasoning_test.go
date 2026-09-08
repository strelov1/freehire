package llm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// bodyRecorder answers any chat request with a fixed reply and keeps what it was sent.
// The wire is the only honest place to check this: the field cannot be read back off the
// client, because the client is not what writes it — the transport is.
type bodyRecorder struct {
	srv *httptest.Server

	mu     sync.Mutex
	bodies []map[string]json.RawMessage
}

func newBodyRecorder(t *testing.T) *bodyRecorder {
	t.Helper()
	r := &bodyRecorder{}
	r.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		raw, _ := io.ReadAll(req.Body)
		fields := map[string]json.RawMessage{}
		_ = json.Unmarshal(raw, &fields)
		r.mu.Lock()
		r.bodies = append(r.bodies, fields)
		r.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"{}"}}]}`)
	}))
	t.Cleanup(r.srv.Close)

	return r
}

func (r *bodyRecorder) last(t *testing.T) map[string]json.RawMessage {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.bodies) == 0 {
		t.Fatal("the endpoint was never called")
	}

	return r.bodies[len(r.bodies)-1]
}

func (r *bodyRecorder) client(t *testing.T) *Client {
	t.Helper()
	c, err := New(r.srv.URL, "service-key", "model-x")
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	return c
}

// The regression guard. WithReasoning must reach the WIRE, not merely a struct field:
// langchaingo has a ReasoningEffort field and the code that would fill it from call
// options is commented out upstream, so a caller that "sets" it there changes nothing.
func TestWithReasoningReachesTheWire(t *testing.T) {
	rec := newBodyRecorder(t)

	if _, err := rec.client(t).GenerateJSON(context.Background(), "sys", "usr", WithReasoning(ReasoningNone)); err != nil {
		t.Fatalf("GenerateJSON: %v", err)
	}

	got, ok := rec.last(t)["reasoning_effort"]
	if !ok {
		t.Fatal("reasoning_effort is absent from the request; the option changed nothing")
	}
	if string(got) != `"none"` {
		t.Errorf("reasoning_effort = %s, want \"none\"", got)
	}
}

// A caller that says nothing must send exactly what it sent before this existed. An
// endpoint seeing an unexpected member is a behaviour change nobody asked for, and on a
// gateway that normalises what it recognises it is one that can reroute the call.
func TestWithoutTheOptionNothingIsAdded(t *testing.T) {
	rec := newBodyRecorder(t)

	if _, err := rec.client(t).GenerateJSON(context.Background(), "sys", "usr"); err != nil {
		t.Fatalf("GenerateJSON: %v", err)
	}

	if _, ok := rec.last(t)["reasoning_effort"]; ok {
		t.Error("reasoning_effort was sent by a call that never asked for it")
	}
}

// The default effort is the zero value, so passing it explicitly must be the same as not
// passing it at all — otherwise a caller threading a value it did not inspect would change
// what its call sends.
func TestTheDefaultEffortSendsNothing(t *testing.T) {
	rec := newBodyRecorder(t)

	if _, err := rec.client(t).GenerateJSON(context.Background(), "sys", "usr", WithReasoning(ReasoningDefault)); err != nil {
		t.Fatalf("GenerateJSON: %v", err)
	}

	if _, ok := rec.last(t)["reasoning_effort"]; ok {
		t.Error("reasoning_effort was sent for the default effort")
	}
}

// The rewrite must leave the rest of the request as langchaingo wrote it. It edits one
// member of a body it did not build, so the thing to prove is that it edits ONLY that one.
func TestTheRewriteLeavesTheRestOfTheRequestAlone(t *testing.T) {
	rec := newBodyRecorder(t)
	c := rec.client(t)

	if _, err := c.GenerateJSON(context.Background(), "sys", "usr"); err != nil {
		t.Fatalf("plain: %v", err)
	}
	plain := rec.last(t)

	if _, err := c.GenerateJSON(context.Background(), "sys", "usr", WithReasoning(ReasoningNone)); err != nil {
		t.Fatalf("with reasoning: %v", err)
	}
	patched := rec.last(t)

	for k, want := range plain {
		got, ok := patched[k]
		if !ok {
			t.Errorf("member %q disappeared from the patched request", k)
			continue
		}
		if string(got) != string(want) {
			t.Errorf("member %q changed: %s -> %s", k, want, got)
		}
	}
	if len(patched) != len(plain)+1 {
		t.Errorf("patched request has %d members, want %d (the original plus reasoning_effort)", len(patched), len(plain)+1)
	}
}

// A body that is not a JSON object is not ours to rewrite. The transport is installed on
// every client, and nothing guarantees only chat requests reach it.
func TestANonJSONBodyIsPassedThrough(t *testing.T) {
	const body = "not json at all"

	got, err := withReasoningEffort([]byte(body), ReasoningNone)
	if err != nil {
		t.Fatalf("withReasoningEffort: %v", err)
	}
	if string(got) != body {
		t.Errorf("body = %q, want it untouched", got)
	}
}

// The effort must survive being bound to a caller's credential: attribution rebuilds the
// client, and a rebuild that dropped the transport would take the option with it — the
// exact shape of the bug that made this necessary in the first place.
func TestReasoningSurvivesBindingACaller(t *testing.T) {
	rec := newBodyRecorder(t)
	bound := rec.client(t).AsCaller(NewCaller("a-users-own-key", nil, Feature("cv-extract")))

	if _, err := bound.GenerateJSON(context.Background(), "sys", "usr", WithReasoning(ReasoningNone)); err != nil {
		t.Fatalf("GenerateJSON: %v", err)
	}

	if _, ok := rec.last(t)["reasoning_effort"]; !ok {
		t.Error("reasoning_effort is absent after binding a caller")
	}
}

// And it must survive a schema-bound call, which is the only kind the résumé extraction
// makes: that path builds a SECOND client of its own, on top of this transport.
func TestReasoningSurvivesASchemaBoundCall(t *testing.T) {
	rec := newBodyRecorder(t)
	schema := map[string]any{"type": "object", "properties": map[string]any{}}

	if _, err := rec.client(t).GenerateJSON(context.Background(), "sys", "usr",
		WithSchema("structured_cv", schema), WithReasoning(ReasoningNone)); err != nil {
		t.Fatalf("GenerateJSON: %v", err)
	}

	body := rec.last(t)
	if _, ok := body["reasoning_effort"]; !ok {
		t.Error("reasoning_effort is absent from a schema-bound request")
	}
	if _, ok := body["response_format"]; !ok {
		t.Error("response_format is absent; the two rewrites are not composing")
	}
}

// Belt and braces on the option's own plumbing, so a failure above points at the transport
// rather than at the config.
func TestWithReasoningSetsTheConfig(t *testing.T) {
	cfg := newGenConfig([]GenOption{WithReasoning(ReasoningNone)})
	if cfg.reasoning != ReasoningNone {
		t.Errorf("reasoning = %q, want %q", cfg.reasoning, ReasoningNone)
	}
	if strings.TrimSpace(string(ReasoningDefault)) != "" {
		t.Error("the default effort must be the empty string, so the zero value sends nothing")
	}
}

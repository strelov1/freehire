package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/strelov1/freehire/internal/llmschema"
)

// recordingProxy stands in for the OpenAI-compatible gateway and keeps every request
// body it served, so a test can assert on what actually went over the wire rather than
// on the client's internal state.
type recordingProxy struct {
	srv *httptest.Server

	mu       sync.Mutex
	requests []map[string]any
}

func newRecordingProxy(t *testing.T) *recordingProxy {
	t.Helper()

	p := &recordingProxy{requests: []map[string]any{}}
	p.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := map[string]any{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		p.mu.Lock()
		p.requests = append(p.requests, body)
		p.mu.Unlock()

		if stream, _ := body["stream"].(bool); stream {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = fmt.Fprint(w, "data: {\"id\":\"1\",\"object\":\"chat.completion.chunk\",\"choices\":"+
				"[{\"index\":0,\"delta\":{\"content\":\"{}\"}}]}\n\n")
			_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"id":"1","object":"chat.completion","choices":`+
			`[{"index":0,"message":{"role":"assistant","content":"{}"},"finish_reason":"stop"}]}`)
	}))
	t.Cleanup(p.srv.Close)

	return p
}

func (p *recordingProxy) client(t *testing.T) *Client {
	t.Helper()

	c, err := New(p.srv.URL, "test-key", "test-model")
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	return c
}

func (p *recordingProxy) lastRequest(t *testing.T) map[string]any {
	t.Helper()

	p.mu.Lock()
	defer p.mu.Unlock()

	if len(p.requests) == 0 {
		t.Fatal("proxy served no request")
	}

	return p.requests[len(p.requests)-1]
}

func testSchema(t *testing.T) llmschema.Schema {
	t.Helper()

	type payload struct {
		Verdict string `json:"verdict"`
	}

	s, err := llmschema.Of[payload]()
	if err != nil {
		t.Fatalf("llmschema.Of: %v", err)
	}

	return s
}

// Every call site that has not migrated depends on this: adding the option must not
// change what a call without it sends.
func TestGenerateJSON_WithoutSchemaIsUnchanged(t *testing.T) {
	proxy := newRecordingProxy(t)

	if _, err := proxy.client(t).GenerateJSON(context.Background(), "sys", "user"); err != nil {
		t.Fatalf("GenerateJSON: %v", err)
	}

	format, ok := proxy.lastRequest(t)["response_format"].(map[string]any)
	if !ok {
		t.Fatal("request carried no response_format at all")
	}
	if format["type"] != "json_object" {
		t.Errorf("response_format.type = %v, want json_object", format["type"])
	}
	if _, ok := format["json_schema"]; ok {
		t.Error("a call without the option sent a json_schema")
	}
}

func TestGenerateJSON_WithSchemaSendsItStrict(t *testing.T) {
	proxy := newRecordingProxy(t)

	_, err := proxy.client(t).GenerateJSON(context.Background(), "sys", "user",
		WithSchema("verdict", testSchema(t)))
	if err != nil {
		t.Fatalf("GenerateJSON: %v", err)
	}

	format, ok := proxy.lastRequest(t)["response_format"].(map[string]any)
	if !ok {
		t.Fatal("request carried no response_format")
	}
	if format["type"] != "json_schema" {
		t.Fatalf("response_format.type = %v, want json_schema", format["type"])
	}

	sent, ok := format["json_schema"].(map[string]any)
	if !ok {
		t.Fatal("response_format carried no json_schema")
	}
	if sent["name"] != "verdict" {
		t.Errorf("json_schema.name = %v, want verdict", sent["name"])
	}
	if sent["strict"] != true {
		t.Errorf("json_schema.strict = %v, want true", sent["strict"])
	}

	props, ok := sent["schema"].(map[string]any)["properties"].(map[string]any)
	if !ok {
		t.Fatal("json_schema.schema carried no properties")
	}
	if _, ok := props["verdict"]; !ok {
		t.Errorf("schema properties = %v, want the contract's verdict field", props)
	}
}

func TestGenerateJSONStream_WithSchemaSendsItToo(t *testing.T) {
	proxy := newRecordingProxy(t)

	_, err := proxy.client(t).GenerateJSONStream(context.Background(), "sys", "user", nil,
		WithSchema("verdict", testSchema(t)))
	if err != nil {
		t.Fatalf("GenerateJSONStream: %v", err)
	}

	format, _ := proxy.lastRequest(t)["response_format"].(map[string]any)
	if format["type"] != "json_schema" {
		t.Fatalf("streamed response_format.type = %v, want json_schema", format["type"])
	}
}

// The response format is bound to the underlying model, not to the call, so a naive
// implementation would rebuild one per request.
func TestGenerateJSON_BuildsOneModelPerSchemaAndReusesIt(t *testing.T) {
	proxy := newRecordingProxy(t)
	c := proxy.client(t)
	schema := testSchema(t)

	for range 3 {
		if _, err := c.GenerateJSON(context.Background(), "sys", "user", WithSchema("verdict", schema)); err != nil {
			t.Fatalf("GenerateJSON: %v", err)
		}
	}
	if got := c.schemaModelCount(); got != 1 {
		t.Fatalf("built %d models for three calls with one schema, want 1", got)
	}

	if _, err := c.GenerateJSON(context.Background(), "sys", "user", WithSchema("other", schema)); err != nil {
		t.Fatalf("GenerateJSON: %v", err)
	}
	if got := c.schemaModelCount(); got != 2 {
		t.Fatalf("a second schema left %d models, want 2", got)
	}
}

// Keying the cache on the name alone would serve the first shape to every later caller
// that reused the name — a response decoded against the wrong contract is a
// zero-valued success nothing would report.
func TestGenerateJSON_OneNameWithTwoShapesGetsTwoModels(t *testing.T) {
	proxy := newRecordingProxy(t)
	c := proxy.client(t)

	type other struct {
		Reason string `json:"reason"`
	}
	second, err := llmschema.Of[other]()
	if err != nil {
		t.Fatalf("llmschema.Of: %v", err)
	}

	for _, s := range []llmschema.Schema{testSchema(t), second} {
		if _, err := c.GenerateJSON(context.Background(), "sys", "user", WithSchema("stage", s)); err != nil {
			t.Fatalf("GenerateJSON: %v", err)
		}
	}

	if got := c.schemaModelCount(); got != 2 {
		t.Fatalf("two shapes under one name built %d models, want 2", got)
	}

	sent, _ := proxy.lastRequest(t)["response_format"].(map[string]any)
	schema, _ := sent["json_schema"].(map[string]any)["schema"].(map[string]any)
	props, _ := schema["properties"].(map[string]any)
	if _, ok := props["reason"]; !ok {
		t.Errorf("the second call was constrained by the first shape: %v", props)
	}
}

// Both degrade silently otherwise: an unconstrained call that looks constrained, and a
// nameless schema most gateways reject.
func TestWithSchema_RejectsANilSchemaAndAnEmptyName(t *testing.T) {
	proxy := newRecordingProxy(t)
	c := proxy.client(t)

	if _, err := c.GenerateJSON(context.Background(), "sys", "user", WithSchema("verdict", nil)); err == nil {
		t.Error("a nil schema was accepted and the call ran unconstrained")
	}
	if _, err := c.GenerateJSON(context.Background(), "sys", "user", WithSchema("", testSchema(t))); err == nil {
		t.Error("an empty schema name was accepted")
	}
}

// A client built from an injected model has no endpoint to rebuild against. Callers'
// tests inject fakes, and they must keep working once their call site passes a schema.
func TestGenerateJSON_InjectedModelIgnoresTheSchemaRatherThanFailing(t *testing.T) {
	c := NewWithModel(&chatModel{chunks: []string{"{}"}})

	got, err := c.GenerateJSON(context.Background(), "sys", "user", WithSchema("verdict", testSchema(t)))
	if err != nil {
		t.Fatalf("GenerateJSON on an injected model: %v", err)
	}
	if got != "{}" {
		t.Errorf("content = %q, want {}", got)
	}
}

// The injector rewrites one member of a body langchaingo owns. Everything else in
// that body — messages, model, streaming flags — must survive untouched, or the
// rewrite would silently change the call it was meant to constrain.
func TestWithResponseFormat_ReplacesOnlyTheResponseFormat(t *testing.T) {
	body := []byte(`{"model":"m","stream":true,"messages":[{"role":"user","content":"hi"}],` +
		`"response_format":{"type":"json_object"}}`)

	patched, err := withResponseFormat(body, json.RawMessage(`{"type":"json_schema"}`))
	if err != nil {
		t.Fatalf("withResponseFormat: %v", err)
	}

	got := map[string]any{}
	if err := json.Unmarshal(patched, &got); err != nil {
		t.Fatalf("patched body is not JSON: %v", err)
	}

	format, _ := got["response_format"].(map[string]any)
	if format["type"] != "json_schema" {
		t.Errorf("response_format.type = %v, want json_schema", format["type"])
	}
	if got["model"] != "m" || got["stream"] != true {
		t.Errorf("model/stream = %v/%v, want m/true — the rest of the request must survive", got["model"], got["stream"])
	}
	if msgs, ok := got["messages"].([]any); !ok || len(msgs) != 1 {
		t.Errorf("messages = %v, want the one message the caller sent", got["messages"])
	}
}

// The transport is installed on one client, but nothing guarantees only chat requests
// reach it; a body it cannot parse must pass through rather than be mangled or fail.
func TestWithResponseFormat_LeavesANonObjectBodyAlone(t *testing.T) {
	body := []byte(`not json at all`)

	patched, err := withResponseFormat(body, json.RawMessage(`{"type":"json_schema"}`))
	if err != nil {
		t.Fatalf("withResponseFormat: %v", err)
	}
	if string(patched) != string(body) {
		t.Errorf("patched = %q, want the body unchanged", patched)
	}
}

func TestGenerateJSON_SchemaPathIsObservedLikeThePlainPath(t *testing.T) {
	proxy := newRecordingProxy(t)
	tracer := &captureTracer{}

	c, err := New(proxy.srv.URL, "test-key", "test-model", WithTracer(tracer, "test"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := c.GenerateJSON(context.Background(), "sys", "user", WithSchema("verdict", testSchema(t))); err != nil {
		t.Fatalf("GenerateJSON: %v", err)
	}

	if len(tracer.got) != 1 {
		t.Fatalf("observed %d generations on the schema path, want 1", len(tracer.got))
	}
	if g := tracer.got[0]; g.Source != "test" || !strings.Contains(g.System, "sys") {
		t.Errorf("generation = %+v, want the same fields the plain path records", g)
	}
}

// schemaModelCount reports how many schema-bound models the client has built. It lives
// here rather than beside the cache because only a test needs it.
func (c *Client) schemaModelCount() int {
	c.schemaModels.mu.Lock()
	defer c.schemaModels.mu.Unlock()

	return len(c.schemaModels.models)
}

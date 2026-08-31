package llm

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"

	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/openai"

	"github.com/strelov1/freehire/internal/platform/llmschema"
)

// SchemaMode selects how a schema is asked for on the wire. It is a property of the
// GATEWAY, not of any call, which is why it lives on Settings rather than on a
// GenOption: one deployment's model either honours strict mode or it does not, and a
// per-call answer would just be the same answer repeated at every call site.
type SchemaMode string

const (
	// SchemaModeStrict sends the schema itself, as `response_format: json_schema`
	// with strict: true. The zero value, and the right request wherever it is
	// understood.
	SchemaModeStrict SchemaMode = ""

	// SchemaModeJSONObject sends `response_format: json_object` and no schema, for
	// a gateway whose model answers a strict schema worse than it answers no
	// schema at all. Measured on z.ai's glm-4.7-flash: a strict two-field object
	// came back as a fenced array of invented job postings, while json_object with
	// the same prompt returned the exact shape three times out of three.
	//
	// The shape is still asked for — every caller in this repository spells its
	// fields out in the system prompt — and still checked, by the caller's own
	// validator. What is lost is the provider-side guarantee, which the package
	// documentation already declines to treat as one.
	SchemaModeJSONObject SchemaMode = "json_object"
)

// GenOption configures a single JSON generation. Options are variadic so a call site
// that asks for nothing keeps sending exactly what it sent before.
type GenOption func(*genConfig)

type genConfig struct {
	schemaName  string
	schema      llmschema.Schema
	schemaAsked bool
}

func newGenConfig(opts []GenOption) genConfig {
	var cfg genConfig
	for _, o := range opts {
		o(&cfg)
	}

	return cfg
}

// WithSchema constrains the response to schema, sent in strict mode under name. The
// name is the model-facing label for the shape and appears in provider logs, so it
// should read like the contract it came from ("structured_cv", "enrichment").
//
// A nil schema or an empty name is an error at call time rather than a call that
// quietly runs unconstrained: both are the failure this package exists to prevent.
func WithSchema(name string, s llmschema.Schema) GenOption {
	return func(cfg *genConfig) {
		cfg.schemaName = name
		cfg.schema = s
		cfg.schemaAsked = true
	}
}

// validate reports the misconfigurations that would otherwise degrade in silence.
func (cfg genConfig) validate() error {
	if !cfg.schemaAsked {
		return nil
	}
	if cfg.schemaName == "" {
		return errors.New("llm: WithSchema needs a name; most gateways reject an empty one")
	}
	if cfg.schema == nil {
		return errors.New("llm: WithSchema was given a nil schema")
	}

	return nil
}

// modelCache holds the per-schema models behind a pointer, so Client stays copyable —
// WithTimeout clones it, and a mutex embedded by value would be copied with it. That
// clone shares the cache, which is right: it addresses the same endpoint with the same
// credential and the same headers.
//
// **As does NOT share it, and must not.** The key below is the schema's name and its
// rendered shape — not the credential, and not the tags. A clone that changed either
// while sharing this cache would be served the model the FIRST caller built: one user's
// call going out on another user's key, or a tagged call going out untagged. Both
// succeed, both decode, and neither is visible in anything but the spend report.
// TestAsDoesNotShareSchemaBoundModelsAcrossCredentials is the guard.
type modelCache struct {
	mu     sync.Mutex
	models map[string]llms.Model
}

func newModelCache() *modelCache {
	return &modelCache{models: map[string]llms.Model{}}
}

func (mc *modelCache) get(name string, build func() (llms.Model, error)) (llms.Model, error) {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	if m, ok := mc.models[name]; ok {
		return m, nil
	}

	m, err := build()
	if err != nil {
		return nil, err
	}
	mc.models[name] = m

	return m, nil
}

// modelFor returns the model a call should run against: the plain one when no schema
// was asked for, otherwise one bound to that schema and reused thereafter.
//
// A client built by NewWithModel has no endpoint to rebuild against — that seam exists
// so callers' tests can inject a fake — so a schema there is ignored rather than
// fatal, and the injected model is used as-is.
func (c *Client) modelFor(cfg genConfig) (llms.Model, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	if cfg.schema == nil || c.baseURL == "" {
		return c.model, nil
	}

	format, err := responseFormat(cfg, c.schemaMode)
	if err != nil {
		return nil, err
	}

	// Keyed on the name AND the rendered schema: two shapes sharing a name would
	// otherwise silently share the first one's constraint, and a response decoded
	// against the wrong contract is a zero-valued success no one would notice. A
	// multi-stage caller passing one name per stage is exactly that case.
	key := cfg.schemaName + "\x00" + string(format)

	return c.schemaModels.get(key, func() (llms.Model, error) {
		m, err := openai.New(
			openai.WithBaseURL(c.baseURL),
			openai.WithToken(c.apiKey),
			openai.WithModel(c.modelID),
			// The schema rewrite sits on top of whatever this client already travels
			// on, so a tagged client's schema-bound calls carry their tags too.
			openai.WithHTTPClient(&http.Client{
				Transport: &schemaInjector{format: format, next: c.transport()},
			}),
		)
		if err != nil {
			return nil, fmt.Errorf("llm: build schema-bound client: %w", err)
		}

		return m, nil
	})
}

// responseFormat renders the strict json_schema response format for cfg.
func responseFormat(cfg genConfig, mode SchemaMode) (json.RawMessage, error) {
	format := map[string]any{
		"type": "json_schema",
		"json_schema": map[string]any{
			"name":   cfg.schemaName,
			"strict": true,
			"schema": cfg.schema,
		},
	}
	if mode == SchemaModeJSONObject {
		format = map[string]any{"type": "json_object"}
	}

	raw, err := json.Marshal(format)
	if err != nil {
		return nil, fmt.Errorf("llm: encode response format: %w", err)
	}

	return raw, nil
}

// schemaInjector rewrites `response_format` in the outgoing chat request.
//
// langchaingo's response-format type cannot express what strict mode needs: its
// property Type is a plain string, while an optional field must be typed
// ["string", "null"] — and without that the model has no legal way to decline a
// required field, so it invents a value instead (measured: 3 runs in 5 asserted a
// visa policy for an ad that stated none). Everything else on the request is
// langchaingo's and passes through untouched.
type schemaInjector struct {
	// next is langchaingo's own transport, which stamps the library's User-Agent.
	// Replacing the whole client would otherwise make schema-bound traffic
	// indistinguishable from any other Go program at the gateway.
	next   http.RoundTripper
	format json.RawMessage
}

func (t *schemaInjector) RoundTrip(req *http.Request) (*http.Response, error) {
	next := t.next
	if next == nil {
		next = http.DefaultTransport
	}

	if req.Body == nil {
		return next.RoundTrip(req)
	}

	// A RoundTripper must close the request body on every path, not only the one
	// that reaches the wire.
	defer req.Body.Close()

	body, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, fmt.Errorf("llm: read request body: %w", err)
	}

	patched, err := withResponseFormat(body, t.format)
	if err != nil {
		return nil, err
	}

	clone := req.Clone(req.Context())
	clone.Body = io.NopCloser(bytes.NewReader(patched))
	clone.ContentLength = int64(len(patched))
	clone.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(patched)), nil
	}

	return next.RoundTrip(clone)
}

// withResponseFormat replaces the response_format member of a chat request, leaving
// every other member as langchaingo wrote it. A body that is not a JSON object is
// returned unchanged — the transport is installed on one client, but nothing
// guarantees only chat requests reach it.
func withResponseFormat(body, format json.RawMessage) (json.RawMessage, error) {
	fields := map[string]json.RawMessage{}
	if err := json.Unmarshal(body, &fields); err != nil {
		return body, nil //nolint:nilerr // not a JSON object: not ours to rewrite
	}

	fields["response_format"] = format

	patched, err := json.Marshal(fields)
	if err != nil {
		return nil, fmt.Errorf("llm: encode patched request: %w", err)
	}

	return patched, nil
}

package llm

import (
	"log"
	"net/http"

	"github.com/tmc/langchaingo/httputil"
	"github.com/tmc/langchaingo/llms/openai"
)

// dimPrefix is the header prefix that tells the gateway which dimensions a call served. The
// header name is the dimension and carries only the value. The gateway files it on the request
// log, the span, and, if listed in its prometheus_labels, every metric.
const dimPrefix = "x-bf-dim-"

// Dimension is one axis the gateway files a call under, sent as `x-bf-dim-<name>: <value>`.
// Each dimension gets its own header with one value; a comma-joined list would be filed as
// a single bogus value.
type Dimension struct {
	Name  string
	Value string
}

// stamp writes each dimension onto a request as its own header. The fallback replay goes
// through it too, so a retry's spend is still attributed.
//
// A dimension with an empty name or value is skipped: the gateway cannot group by it.
func stamp(req *http.Request, dims []Dimension) {
	for _, d := range dims {
		if d.Name == "" || d.Value == "" {
			continue
		}
		req.Header.Set(dimPrefix+d.Name, d.Value)
	}
}

// Feature names the surface a call served; every per-user call carries it (constants in
// internal/api/handler/user_llm.go).
func Feature(value string) Dimension { return Dimension{Name: "feature", Value: value} }

// Caller is WHO a call spends as: a gateway credential, the callback that clears it when the
// gateway no longer knows it, and the dimensions the spend is filed under.
//
// A Caller carries who, never how. It holds no client, so attribution cannot replace a
// component's own client and drop its settings (the timeout above all).
type Caller struct {
	secret    string
	onRefused func()
	dims      []Dimension
}

// NewCaller names who a call spends as. The zero Caller is the service's own credential
// with no dimensions, for unconfigured deployments and background jobs; AsCaller returns the
// receiver untouched for it.
func NewCaller(secret string, onRefused func(), dims ...Dimension) Caller {
	return Caller{secret: secret, onRefused: onRefused, dims: dims}
}

// AsCaller returns this client spending as `caller` and keeping everything else it was built
// with, the timeout most of all.
func (c *Client) AsCaller(caller Caller) *Client {
	return c.As(caller.secret, caller.onRefused, caller.dims...)
}

// As returns a client that spends under a given credential and labels its calls.
//
// An empty secret keeps the client's own credential (attribution fails open) and the tags are
// still applied. Passing neither returns the receiver untouched.
//
// onRefused (may be nil) is called when the gateway no longer knows the secret; the call is
// re-made on the client's own credential and completes, and onRefused replaces the stale value.
//
// Nil-safe, and a no-op on a client built by NewWithModel (no endpoint to rebuild). A failure
// to rebuild returns the receiver: losing attribution is a log line, not a failed request.
func (c *Client) As(secret string, onRefused func(), dims ...Dimension) *Client {
	if c == nil {
		return nil
	}
	if secret == "" && len(dims) == 0 {
		return c
	}
	if c.baseURL == "" {
		return c
	}

	clone := *c
	if secret != "" {
		// The fallback must always be the service credential, never another user's secret.
		// If c is already bound to a user, carry its fallbackKey forward; otherwise
		// base.As(userA).As(userB) would retry userB's refusals on userA's key.
		fallback := c.apiKey
		if c.fallbackKey != "" {
			fallback = c.fallbackKey
		}
		clone.fallbackKey = fallback
		clone.apiKey = secret
		clone.onRefused = onRefused
	}
	clone.dims = dims

	// Must not share the schema-model cache with the source client: it is keyed on schema
	// alone, so the clone would reuse a model built for another credential and its spend
	// would land on the wrong account.
	clone.schemaModels = newModelCache()

	m, err := openai.New(
		openai.WithBaseURL(clone.baseURL),
		openai.WithToken(clone.apiKey),
		openai.WithModel(clone.modelID),
		openai.WithHTTPClient(&http.Client{Transport: clone.transport()}),
	)
	if err != nil {
		log.Printf("llm: could not bind client to a credential: %v", err)
		return c
	}
	clone.model = m

	return &clone
}

// transport is langchaingo's own transport, wrapped in the attribution stamp when this client
// carries tags or a per-user credential. Schema-bound models build on it, so they are tagged too.
func (c *Client) transport() http.RoundTripper {
	// Keep langchaingo's transport: it stamps the User-Agent the gateway groups spend by.
	next := httputil.DefaultTransport
	if len(c.dims) > 0 || c.fallbackKey != "" {
		next = &attribution{
			dims:      c.dims,
			fallback:  c.fallbackKey,
			onRefused: c.onRefused,
			next:      next,
		}
	}
	// Unconditional: reasoning is decided per call, so installing it only on tagged clients
	// would make WithReasoning silently do nothing elsewhere. Outside the attribution stamp so
	// a credential retry re-sends the already rewritten body.
	return &reasoningInjector{next: next}
}

// attribution stamps the feature tags onto an outgoing call and rescues one whose credential
// the gateway has forgotten. It lives at the transport because langchaingo cannot add headers
// and reports a refusal only as error text; the status code is the stable signal.
type attribution struct {
	next      http.RoundTripper
	dims      []Dimension
	fallback  string
	onRefused func()
}

func (t *attribution) RoundTrip(req *http.Request) (*http.Response, error) {
	next := t.next
	if next == nil {
		next = http.DefaultTransport
	}
	// A RoundTripper must not mutate the request it is given.
	clone := req.Clone(req.Context())
	stamp(clone, t.dims)

	resp, err := next.RoundTrip(clone)
	if err != nil || resp.StatusCode != http.StatusUnauthorized || t.fallback == "" {
		return resp, err
	}

	// Unknown credential: retry once on the fallback. Only once, or a bad fallback key would
	// become a request storm.
	retry, ok := replay(req, t.fallback, t.dims)
	if !ok {
		return resp, nil
	}
	resp.Body.Close()
	if t.onRefused != nil {
		t.onRefused()
	}

	return next.RoundTrip(retry)
}

// replay rebuilds a request under a different credential. It reports failure rather than an
// error: if the body cannot be replayed, the original refusal is the answer.
func replay(req *http.Request, credential string, dims []Dimension) (*http.Request, bool) {
	if req.Body != nil && req.GetBody == nil {
		return nil, false
	}
	clone := req.Clone(req.Context())
	if req.GetBody != nil {
		body, err := req.GetBody()
		if err != nil {
			return nil, false
		}
		clone.Body = body
	}
	clone.Header.Set("Authorization", "Bearer "+credential)
	stamp(clone, dims)

	return clone, true
}

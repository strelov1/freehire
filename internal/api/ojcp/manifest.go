package ojcp

import "strings"

// Manifest is the document served at /.well-known/ojcp.json — the one thing an OJCP
// provider MUST publish, and how an agent discovers everything else.
//
// It is RENDERED from this deployment's own configuration rather than kept as a static
// file. The Go service answers it at /api/v1/ojcp/manifest and the SPA proxies that to
// /.well-known/ojcp.json, where an agent looks: nginx routes /api/ to the backend and
// everything else to the Node process, so a Go route at the well-known path itself would
// never receive a request.
//
// Two of its claims are binding: `tools` says what this server answers, and
// `rate_limits` says what it enforces — the spec makes a declared limit a MUST. A
// hand-edited file drifts from the router silently; a rendered one can be asserted against
// the router in a test.
type Manifest struct {
	OJCPVersion string   `json:"ojcp_version"`
	Provider    Provider `json:"provider"`
	// Tools is what this deployment answers. Built from the transports' own registrations,
	// never written out here, so the manifest cannot advertise a tool nothing serves.
	Tools          []string       `json:"tools"`
	FeedEndpoints  *FeedEndpoints `json:"feed_endpoints,omitempty"`
	MCPEndpoint    string         `json:"mcp_endpoint,omitempty"`
	Auth           *ManifestAuth  `json:"auth,omitempty"`
	RateLimits     *RateLimits    `json:"rate_limits,omitempty"`
	ApplyPathTypes []string       `json:"apply_paths,omitempty"`
}

// Provider is who is answering.
type Provider struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	LogoURL     string `json:"logo_url,omitempty"`
}

// FeedEndpoints are the REST routes, for a client that does not speak MCP — including
// OJCP's own conformance suite.
type FeedEndpoints struct {
	Search string `json:"search,omitempty"`
	Detail string `json:"detail,omitempty"`
}

// ManifestAuth states that the read surface is open. Required is false and stays false:
// this is a public catalogue, and an agent should not have to negotiate anything to read it.
type ManifestAuth struct {
	Required bool `json:"required"`
}

// RateLimits is what this deployment enforces, per second. Declaring a figure we do not
// honour breaks a MUST, so it is derived from the same configuration the limiter reads.
type RateLimits struct {
	AnonymousRPS int `json:"anonymous_rps"`
}

// ManifestConfig is what a deployment must supply to describe itself.
type ManifestConfig struct {
	// Origin is the absolute site origin the endpoints are served from.
	Origin string
	// Tools is what the transports actually registered.
	Tools []string
	// AnonymousRPS is the rate this deployment enforces on the OJCP routes.
	AnonymousRPS int
	// ApplyPathTypes are the application mechanisms postings here can carry.
	ApplyPathTypes []string
}

// NewManifest renders the manifest for this deployment.
//
// An empty Origin yields endpoints that are omitted rather than published as relative
// paths: an agent cannot dereference "/api/v1/ojcp/v1/search", and a manifest claiming an
// endpoint that does not resolve is worse than one claiming none.
func NewManifest(cfg ManifestConfig) Manifest {
	origin := strings.TrimSuffix(cfg.Origin, "/")

	m := Manifest{
		OJCPVersion: Version,
		Provider: Provider{
			Name:        "freehire",
			Description: "An open-source IT job aggregator: postings from many sources, normalised into one schema, deduplicated and enriched.",
		},
		Tools:          cfg.Tools,
		Auth:           &ManifestAuth{Required: false},
		ApplyPathTypes: cfg.ApplyPathTypes,
	}
	if cfg.AnonymousRPS > 0 {
		m.RateLimits = &RateLimits{AnonymousRPS: cfg.AnonymousRPS}
	}
	if origin != "" {
		m.FeedEndpoints = &FeedEndpoints{
			Search: origin + "/api/v1/ojcp/v1/search",
			Detail: origin + "/api/v1/ojcp/v1/jobs/{job_id}",
		}
		m.MCPEndpoint = origin + "/api/v1/ojcp/mcp"
	}
	return m
}

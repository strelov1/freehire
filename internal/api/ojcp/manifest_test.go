package ojcp

import (
	"strings"
	"testing"
)

func testManifest() Manifest {
	return NewManifest(ManifestConfig{
		Origin:         testOrigin,
		Tools:          []string{"search_jobs", "get_job_detail", "get_employer_context"},
		AnonymousRPS:   10,
		ApplyPathTypes: []string{"ats_direct", "external_redirect"},
	})
}

func TestManifestConformsToTheStandard(t *testing.T) {
	if err := validateAgainstSchema(t, schemaManifest, testManifest()); err != nil {
		t.Fatalf("manifest rejected: %v", err)
	}
}

func TestManifestPublishesAbsoluteEndpoints(t *testing.T) {
	m := testManifest()

	for name, got := range map[string]string{
		"mcp_endpoint":          m.MCPEndpoint,
		"feed_endpoints.search": m.FeedEndpoints.Search,
		"feed_endpoints.detail": m.FeedEndpoints.Detail,
	} {
		if !strings.HasPrefix(got, "https://") {
			t.Errorf("%s = %q, want an absolute URL", name, got)
		}
	}
}

func TestManifestOmitsEndpointsItCannotAddress(t *testing.T) {
	// A deployment with no configured origin would otherwise publish "/api/v1/..." — a
	// path an agent cannot dereference. A manifest claiming an endpoint that does not
	// resolve is worse than one claiming none, and the schema requires neither.
	m := NewManifest(ManifestConfig{Tools: []string{"search_jobs"}})

	if m.MCPEndpoint != "" {
		t.Errorf("mcp_endpoint = %q, want it omitted without an origin", m.MCPEndpoint)
	}
	if m.FeedEndpoints != nil {
		t.Errorf("feed_endpoints = %+v, want them omitted without an origin", m.FeedEndpoints)
	}
	if err := validateAgainstSchema(t, schemaManifest, m); err != nil {
		t.Fatalf("origin-less manifest rejected: %v", err)
	}
}

func TestManifestDeclaresNoRateLimitItCannotName(t *testing.T) {
	// The spec makes a declared limit binding. A zero would publish "no requests allowed",
	// which is not what an unconfigured limiter means.
	m := NewManifest(ManifestConfig{Origin: testOrigin, Tools: []string{"search_jobs"}})

	if m.RateLimits != nil {
		t.Errorf("rate_limits = %+v, want them omitted rather than declared as zero", m.RateLimits)
	}
}

func TestManifestSaysTheReadSurfaceIsOpen(t *testing.T) {
	// The catalogue is public. An agent must not have to negotiate anything to read it, and
	// `auth` absent would leave that to be guessed.
	m := testManifest()

	if m.Auth == nil || m.Auth.Required {
		t.Errorf("auth = %+v, want it present and not required", m.Auth)
	}
}

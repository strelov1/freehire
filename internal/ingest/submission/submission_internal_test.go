package submission

// White-box tests for the unexported host-normalization helpers, kept separate from
// submission_test.go (package submission_test) because hostOf/normalizeHost are not
// exported: Submit/Reject only ever call hostOf on a URL moderation.CreateInput.Validate
// (or a stored submission row) already guaranteed parses, so the "unreachable in practice"
// parse-failure path has no route to a black-box test.

import "testing"

func TestNormalizeHost(t *testing.T) {
	cases := []struct {
		name string
		host string
		want string
	}{
		{"bare host", "gridnaut.site", "gridnaut.site"},
		{"www prefix stripped", "www.gridnaut.site", "gridnaut.site"},
		{"mixed case lowercased", "WWW.GridNaut.Site", "gridnaut.site"},
		{"surrounding whitespace trimmed", "  gridnaut.site  ", "gridnaut.site"},
		{"port left alone", "gridnaut.site:8080", "gridnaut.site:8080"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := normalizeHost(tc.host); got != tc.want {
				t.Errorf("normalizeHost(%q) = %q, want %q", tc.host, got, tc.want)
			}
		})
	}
}

func TestHostOf(t *testing.T) {
	cases := []struct {
		name string
		url  string
		want string
	}{
		{"ordinary URL", "https://www.Gridnaut.Site/jobs/1", "gridnaut.site"},
		// Unreachable through Submit/Reject in production (Validate already requires an
		// absolute http(s) URL before either calls hostOf), but hostOf itself must still
		// degrade to "" rather than panic if ever handed something url.Parse rejects.
		{"unparseable URL yields empty host", "http://[::1", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := hostOf(tc.url); got != tc.want {
				t.Errorf("hostOf(%q) = %q, want %q", tc.url, got, tc.want)
			}
		})
	}
}

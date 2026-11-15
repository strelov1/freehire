package oauth2server

import (
	"errors"
	"net/url"
	"strings"
)

// ErrInvalidTarget is RFC 8707's invalid_target: not a resource this server protects.
var ErrInvalidTarget = errors.New("invalid_target")

// MCPResourcePath is the one MCP resource (MCPAccountResourcePath is its alias). A grant
// binds to the PATH, not the origin-qualified URL, so a frontend origin change does not
// orphan grants; the origin is checked on request. A grant stored under the old
// "/api/v1/mcp/account" path is refused (invalid_token) and the client signs in again.
const (
	MCPResourcePath        = "/api/v1/mcp"
	MCPAccountResourcePath = "/api/v1/mcp/account"
)

// CanonicalResource validates a request's `resource` values against this origin and
// returns the single MCP resource PATH to bind, or "" when none was sent.
//
// Each value must be an absolute URI with origin's scheme and host (case-insensitive,
// default port ignored) and exactly the MCP path, one trailing slash tolerated. Query
// (even a bare "?"), fragment and userinfo are refused, as are several distinct values.
func CanonicalResource(origin string, raw []string) (string, error) {
	var bound string
	for _, r := range raw {
		if r == "" {
			continue
		}
		want, ok := originKey(origin)
		if !ok {
			return "", ErrInvalidTarget
		}
		path, err := resourcePath(want, r)
		if err != nil || (bound != "" && bound != path) {
			return "", ErrInvalidTarget
		}
		bound = path
	}
	return bound, nil
}

func resourcePath(wantOrigin, raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" || u.User != nil ||
		u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || strings.Contains(raw, "#") {
		return "", ErrInvalidTarget
	}
	if got, ok := originKey(raw); !ok || got != wantOrigin {
		return "", ErrInvalidTarget
	}
	switch strings.TrimSuffix(u.Path, "/") {
	case MCPResourcePath, MCPAccountResourcePath:
		return MCPResourcePath, nil
	}
	return "", ErrInvalidTarget
}

// originKey reduces a URL to lowercase scheme://host[:port] without the default port.
func originKey(raw string) (string, bool) {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" {
		return "", false
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", false
	}
	host := strings.ToLower(u.Hostname())
	if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	port := u.Port()
	if (scheme == "https" && port == "443") || (scheme == "http" && port == "80") {
		port = ""
	}
	if port != "" {
		host += ":" + port
	}
	return scheme + "://" + host, true
}

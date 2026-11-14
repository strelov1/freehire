package oauth2server

import (
	"errors"
	"net/url"
	"strings"
)

// ErrInvalidTarget is RFC 8707's invalid_target: the requested resource is not
// one this server protects.
var ErrInvalidTarget = errors.New("invalid_target")

// MCPResourcePath is the one MCP resource (MCPAccountResourcePath is its alias). A grant is bound to it by PATH, never
// the origin-qualified URL, so a later change of the frontend origin (domain move,
// www/apex) does not orphan bound grants. The origin is checked when the resource
// is requested. "/api/v1/mcp/account" was a second resource; it is now an alias of the
// same server, so asking for it binds MCPResourcePath. A grant already stored with the
// old path is refused (invalid_token) and the client signs in again.
const (
	MCPResourcePath        = "/api/v1/mcp"
	MCPAccountResourcePath = "/api/v1/mcp/account"
)

// CanonicalResource validates the `resource` values a request carried against
// this deployment's origin and returns the single MCP resource PATH to bind, or
// "" when none was sent.
//
// Each value must be an absolute URI whose scheme and host (case-insensitive,
// default port ignored) are origin's and whose path, compared exactly, is the
// MCP endpoint, with one trailing slash tolerated (clients
// canonicalise inconsistently). No query (a bare "?" included), fragment or
// userinfo is accepted. A request names one resource, so several distinct
// values are refused.
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

// originKey reduces a URL to its lowercase scheme://host[:port], dropping the
// scheme's default port, so two spellings of one origin compare equal.
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

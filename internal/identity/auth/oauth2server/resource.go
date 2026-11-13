package oauth2server

import (
	"errors"
	"slices"
	"strings"
)

// ErrInvalidTarget is RFC 8707's invalid_target: the requested resource is not
// one this server protects.
var ErrInvalidTarget = errors.New("invalid_target")

// MCP resource paths, the identifiers the two protected-resource documents
// advertise. A grant is bound to one of them (origin + path).
const (
	MCPResourcePath        = "/api/v1/mcp"
	MCPAccountResourcePath = "/api/v1/mcp/account"
)

// CanonicalResource validates the `resource` values a request carried and
// returns the single identifier to bind, or "" when none was sent. Only the two
// MCP endpoints of this origin are known; one trailing slash is tolerated and
// dropped (clients canonicalise inconsistently), nothing else is: no other
// path, case, query or fragment. A request names one resource, so several
// distinct values are refused.
func CanonicalResource(origin string, raw []string) (string, error) {
	origin = strings.TrimSuffix(origin, "/")
	known := []string{origin + MCPResourcePath, origin + MCPAccountResourcePath}
	var bound string
	for _, r := range raw {
		if r == "" {
			continue
		}
		r = strings.TrimSuffix(r, "/")
		if !slices.Contains(known, r) || (bound != "" && bound != r) {
			return "", ErrInvalidTarget
		}
		bound = r
	}
	return bound, nil
}

// ResourceAllows reports whether a grant bound to bound may be used at the
// endpoint identified by endpoint. An unbound grant ("") is valid everywhere:
// those predate resource indicators or came from clients that never sent one.
func ResourceAllows(bound, endpoint string) bool {
	return bound == "" || bound == endpoint
}

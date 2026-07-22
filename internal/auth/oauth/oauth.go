// Package oauth implements sign-in via external OAuth providers (Google,
// GitHub, LinkedIn) behind a small Provider interface and a registry built
// from config, mirroring how internal/sources grows by adapters. Providers
// are read-only identity fetchers: tokens are used once to resolve who the
// user is and are never stored.
package oauth

import (
	"context"

	"github.com/strelov1/freehire/internal/config"
)

// Identity is what a provider knows about the signing-in user: a stable
// per-provider user id and, when available, an email with its verification
// status. Account resolution MUST ignore unverified emails (linking on an
// unverified email would allow account takeover).
type Identity struct {
	ProviderUserID string
	Email          string
	EmailVerified  bool
}

// Provider is one OAuth provider in the authorization-code flow: it builds
// the consent URL and turns a callback code into an Identity.
type Provider interface {
	Name() string
	AuthCodeURL(state string) string
	FetchIdentity(ctx context.Context, code string) (Identity, error)
}

// constructors maps a provider name to its builder. The redirect URL is a build
// argument, not baked in, so the same registry can serve OAuth on more than one
// domain (the origin is chosen per request — see Registry.Provider).
var constructors = map[string]func(clientID, clientSecret, redirectURL string) Provider{
	"google":   NewGoogle,
	"github":   NewGitHub,
	"linkedin": NewLinkedIn,
}

// Registry holds the credentials of the enabled OAuth providers and builds a
// Provider on demand for a given request origin. Deferring the redirect URL to
// build time is what lets one deployment complete the flow on multiple domains
// during a migration — each domain's callback must be registered with the
// provider. Provider construction is a cheap struct literal (no network), so
// building per request is free.
type Registry struct {
	creds map[string]config.OAuthCredentials
}

// NewRegistry keeps only providers with both a client id and secret for a known
// provider name; unknown names and incomplete credentials are dropped, so an
// unconfigured provider is simply absent (its routes 404 / the list omits it).
func NewRegistry(creds map[string]config.OAuthCredentials) *Registry {
	enabled := make(map[string]config.OAuthCredentials)
	for name, c := range creds {
		if _, known := constructors[name]; known && c.ClientID != "" && c.ClientSecret != "" {
			enabled[name] = c
		}
	}
	return &Registry{creds: enabled}
}

// Names returns the enabled provider names (unsorted; callers sort for output).
func (r *Registry) Names() []string {
	names := make([]string, 0, len(r.creds))
	for name := range r.creds {
		names = append(names, name)
	}
	return names
}

// Provider builds the named provider with its callback rooted at origin
// (origin + /api/v1/auth/oauth/<name>/callback). ok is false for a name that is
// not enabled.
func (r *Registry) Provider(name, origin string) (Provider, bool) {
	c, ok := r.creds[name]
	if !ok {
		return nil, false
	}
	return constructors[name](c.ClientID, c.ClientSecret, origin+"/api/v1/auth/oauth/"+name+"/callback"), true
}

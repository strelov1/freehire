// Package oauth implements sign-in via external OAuth providers behind a Provider
// interface and a registry built from config. Tokens are used once to resolve identity
// and never stored.
package oauth

import (
	"context"

	"github.com/strelov1/freehire/internal/platform/config"
)

// Identity is what a provider knows about the user. Account resolution MUST ignore
// unverified emails; linking on one allows account takeover.
type Identity struct {
	ProviderUserID string
	Email          string
	EmailVerified  bool
}

// Provider is one OAuth provider in the authorization-code flow.
type Provider interface {
	Name() string
	AuthCodeURL(state string) string
	FetchIdentity(ctx context.Context, code string) (Identity, error)
}

// constructors maps a provider name to its builder. The redirect URL is a build argument
// so one registry serves several domains; the full credentials struct is passed because
// Apple needs more than ClientID/ClientSecret.
var constructors = map[string]func(creds config.OAuthCredentials, redirectURL string) Provider{
	"google":   NewGoogle,
	"github":   NewGitHub,
	"linkedin": NewLinkedIn,
	"apple":    NewApple,
}

// Registry holds the enabled providers' credentials and builds a Provider per request
// origin. Each domain's callback must be registered with the provider.
type Registry struct {
	creds map[string]config.OAuthCredentials
}

// credentialsComplete reports whether creds suffice for the provider. Apple needs
// TeamID, KeyID and PrivateKey instead of a client secret.
func credentialsComplete(name string, creds config.OAuthCredentials) bool {
	if creds.ClientID == "" {
		return false
	}
	if name == "apple" {
		return creds.TeamID != "" && creds.KeyID != "" && creds.PrivateKey != ""
	}
	return creds.ClientSecret != ""
}

// NewRegistry drops unknown names and incomplete credentials, so an unconfigured
// provider is absent (its routes 404).
func NewRegistry(creds map[string]config.OAuthCredentials) *Registry {
	enabled := make(map[string]config.OAuthCredentials)
	for name, c := range creds {
		if _, known := constructors[name]; known && credentialsComplete(name, c) {
			enabled[name] = c
		}
	}
	return &Registry{creds: enabled}
}

// Names returns the enabled provider names, unsorted.
func (r *Registry) Names() []string {
	names := make([]string, 0, len(r.creds))
	for name := range r.creds {
		names = append(names, name)
	}
	return names
}

// Provider builds the named provider with its callback at
// origin + /api/v1/auth/oauth/<name>/callback. ok is false when not enabled.
func (r *Registry) Provider(name, origin string) (Provider, bool) {
	c, ok := r.creds[name]
	if !ok {
		return nil, false
	}
	return constructors[name](c, origin+"/api/v1/auth/oauth/"+name+"/callback"), true
}

// ProviderV2 builds a browser provider for the verifier-bound native flow. Apple is used
// here only by the same-origin web recent-auth adapter; native clients use their SDK.
//
// The callback is deliberately the same as Provider's: a provider console registers one
// callback URL per application. The flows are told apart at the callback by which state
// cookie comes back.
func (r *Registry) ProviderV2(name, origin string) (Provider, bool) {
	return r.Provider(name, origin)
}

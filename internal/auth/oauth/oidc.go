package oauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/endpoints"

	"github.com/strelov1/freehire/internal/config"
	"github.com/strelov1/freehire/internal/safehttp"
)

// maxUserinfoBytes caps an identity-provider JSON response. Userinfo payloads are
// small; a multi-MB body is a misbehaving or hostile endpoint, not a real payload.
const maxUserinfoBytes = 1 << 20 // 1 MiB

// userinfoTimeout bounds the OAuth token-exchange and userinfo round-trips.
const userinfoTimeout = 15 * time.Second

// oidcProvider covers every provider that exposes a standard OIDC userinfo
// endpoint (Google, LinkedIn): exchange the code, then one GET for
// sub/email/email_verified.
type oidcProvider struct {
	name        string
	cfg         *oauth2.Config
	userinfoURL string
}

// NewGoogle returns the Google provider ("Sign in with Google" via OIDC).
func NewGoogle(creds config.OAuthCredentials, redirectURL string) Provider {
	return &oidcProvider{
		name: "google",
		cfg: &oauth2.Config{
			ClientID:     creds.ClientID,
			ClientSecret: creds.ClientSecret,
			Endpoint:     endpoints.Google,
			RedirectURL:  redirectURL,
			Scopes:       []string{"openid", "email"},
		},
		userinfoURL: "https://openidconnect.googleapis.com/v1/userinfo",
	}
}

// NewLinkedIn returns the LinkedIn provider ("Sign In with LinkedIn using
// OpenID Connect" — the product must be enabled on the LinkedIn app).
func NewLinkedIn(creds config.OAuthCredentials, redirectURL string) Provider {
	return &oidcProvider{
		name: "linkedin",
		cfg: &oauth2.Config{
			ClientID:     creds.ClientID,
			ClientSecret: creds.ClientSecret,
			Endpoint:     endpoints.LinkedIn,
			RedirectURL:  redirectURL,
			Scopes:       []string{"openid", "email"},
		},
		userinfoURL: "https://api.linkedin.com/v2/userinfo",
	}
}

func (p *oidcProvider) Name() string { return p.name }

func (p *oidcProvider) AuthCodeURL(state string) string {
	return p.cfg.AuthCodeURL(state)
}

func (p *oidcProvider) FetchIdentity(ctx context.Context, code string) (Identity, error) {
	ctx = safehttp.GuardedOAuth2Context(ctx, userinfoTimeout)
	tok, err := p.cfg.Exchange(ctx, code)
	if err != nil {
		return Identity{}, fmt.Errorf("%s: exchange code: %w", p.name, err)
	}

	var ui struct {
		Sub           string `json:"sub"`
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"`
	}
	if err := getJSON(ctx, p.cfg.Client(ctx, tok), p.userinfoURL, &ui); err != nil {
		return Identity{}, fmt.Errorf("%s: userinfo: %w", p.name, err)
	}
	if ui.Sub == "" {
		return Identity{}, fmt.Errorf("%s: userinfo has no sub", p.name)
	}
	return Identity{ProviderUserID: ui.Sub, Email: ui.Email, EmailVerified: ui.EmailVerified}, nil
}

// getJSON GETs url with the token-bearing client and decodes the JSON body.
func getJSON(ctx context.Context, client *http.Client, url string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return errors.New(resp.Status)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, maxUserinfoBytes)).Decode(out)
}

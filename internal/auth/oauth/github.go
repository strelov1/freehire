package oauth

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/endpoints"
)

// githubProvider implements GitHub sign-in. GitHub is plain OAuth2 (no OIDC
// userinfo): the user id comes from /user and the email from /user/emails,
// because the /user email field is null for most accounts.
type githubProvider struct {
	cfg     *oauth2.Config
	apiBase string
}

// NewGitHub returns the GitHub provider.
func NewGitHub(clientID, clientSecret, redirectURL string) Provider {
	return &githubProvider{
		cfg: &oauth2.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			Endpoint:     endpoints.GitHub,
			RedirectURL:  redirectURL,
			Scopes:       []string{"read:user", "user:email"},
		},
		apiBase: "https://api.github.com",
	}
}

func (p *githubProvider) Name() string { return "github" }

func (p *githubProvider) AuthCodeURL(state string) string {
	return p.cfg.AuthCodeURL(state)
}

func (p *githubProvider) FetchIdentity(ctx context.Context, code string) (Identity, error) {
	ctx = guardedOAuthContext(ctx)
	tok, err := p.cfg.Exchange(ctx, code)
	if err != nil {
		return Identity{}, fmt.Errorf("github: exchange code: %w", err)
	}
	client := p.cfg.Client(ctx, tok)

	var user struct {
		ID int64 `json:"id"`
	}
	if err := getJSON(ctx, client, p.apiBase+"/user", &user); err != nil {
		return Identity{}, fmt.Errorf("github: user: %w", err)
	}
	if user.ID == 0 {
		return Identity{}, errors.New("github: user has no id")
	}

	var emails []struct {
		Email    string `json:"email"`
		Primary  bool   `json:"primary"`
		Verified bool   `json:"verified"`
	}
	if err := getJSON(ctx, client, p.apiBase+"/user/emails", &emails); err != nil {
		return Identity{}, fmt.Errorf("github: emails: %w", err)
	}

	// Prefer the primary verified email; fall back to any verified one. An
	// account with no verified email yields an identity without an email,
	// which account resolution rejects.
	var email string
	for _, e := range emails {
		if !e.Verified {
			continue
		}
		if e.Primary {
			email = e.Email
			break
		}
		if email == "" {
			email = e.Email
		}
	}
	return Identity{
		ProviderUserID: strconv.FormatInt(user.ID, 10),
		Email:          email,
		EmailVerified:  email != "",
	}, nil
}

package oauth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"golang.org/x/oauth2"
)

func stubGitHub(t *testing.T, emails []map[string]any) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/login/oauth/access_token", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "gh-token", "token_type": "Bearer"})
	})
	mux.HandleFunc("/user", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"id": 42, "login": "octocat"})
	})
	mux.HandleFunc("/user/emails", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(emails)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func githubForTest(srv *httptest.Server) *githubProvider {
	return &githubProvider{
		cfg: &oauth2.Config{
			ClientID:     "id",
			ClientSecret: "secret",
			Endpoint: oauth2.Endpoint{
				AuthURL:  srv.URL + "/login/oauth/authorize",
				TokenURL: srv.URL + "/login/oauth/access_token",
			},
			RedirectURL: "http://app/callback",
			Scopes:      []string{"read:user", "user:email"},
		},
		apiBase: srv.URL,
	}
}

func TestGitHub_FetchIdentityPrimaryVerifiedEmail(t *testing.T) {
	srv := stubGitHub(t, []map[string]any{
		{"email": "old@e.com", "primary": false, "verified": true},
		{"email": "main@e.com", "primary": true, "verified": true},
	})
	got, err := githubForTest(srv).FetchIdentity(testClientCtx(srv), "code")
	if err != nil {
		t.Fatalf("FetchIdentity: %v", err)
	}
	want := Identity{ProviderUserID: "42", Email: "main@e.com", EmailVerified: true}
	if got != want {
		t.Errorf("identity = %+v, want %+v", got, want)
	}
}

func TestGitHub_FetchIdentityFallsBackToAnyVerified(t *testing.T) {
	srv := stubGitHub(t, []map[string]any{
		{"email": "unverified@e.com", "primary": true, "verified": false},
		{"email": "side@e.com", "primary": false, "verified": true},
	})
	got, err := githubForTest(srv).FetchIdentity(testClientCtx(srv), "code")
	if err != nil {
		t.Fatalf("FetchIdentity: %v", err)
	}
	if got.Email != "side@e.com" || !got.EmailVerified {
		t.Errorf("identity = %+v, want side@e.com verified", got)
	}
}

func TestGitHub_FetchIdentityNoVerifiedEmail(t *testing.T) {
	srv := stubGitHub(t, []map[string]any{
		{"email": "unverified@e.com", "primary": true, "verified": false},
	})
	got, err := githubForTest(srv).FetchIdentity(testClientCtx(srv), "code")
	if err != nil {
		t.Fatalf("FetchIdentity: %v", err)
	}
	if got.EmailVerified || got.Email != "" {
		t.Errorf("identity = %+v, want no email", got)
	}
	if got.ProviderUserID != "42" {
		t.Errorf("ProviderUserID = %q, want 42", got.ProviderUserID)
	}
}

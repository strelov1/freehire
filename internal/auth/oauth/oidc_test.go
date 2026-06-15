package oauth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"golang.org/x/oauth2"
)

// stubOIDC serves a token endpoint and a userinfo endpoint, capturing the
// bearer token the userinfo call presents.
func stubOIDC(t *testing.T, userinfo map[string]any) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "stub-access-token",
			"token_type":   "Bearer",
		})
	})
	mux.HandleFunc("/userinfo", func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Authorization"), "stub-access-token") {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(userinfo)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// testClientCtx injects the stub server's client so the SSRF guard — which would
// reject the loopback httptest server — is bypassed in tests. The production code
// path supplies no client and always gets the guard.
func testClientCtx(srv *httptest.Server) context.Context {
	return context.WithValue(context.Background(), oauth2.HTTPClient, srv.Client())
}

func oidcForTest(srv *httptest.Server) *oidcProvider {
	return &oidcProvider{
		name: "google",
		cfg: &oauth2.Config{
			ClientID:     "id",
			ClientSecret: "secret",
			Endpoint:     oauth2.Endpoint{AuthURL: srv.URL + "/auth", TokenURL: srv.URL + "/token"},
			RedirectURL:  "http://app/callback",
			Scopes:       []string{"openid", "email"},
		},
		userinfoURL: srv.URL + "/userinfo",
	}
}

func TestOIDC_AuthCodeURLCarriesState(t *testing.T) {
	p := oidcForTest(stubOIDC(t, nil))
	u := p.AuthCodeURL("the-state")
	if !strings.Contains(u, "state=the-state") {
		t.Errorf("AuthCodeURL %q missing state", u)
	}
}

func TestOIDC_FetchIdentity(t *testing.T) {
	srv := stubOIDC(t, map[string]any{
		"sub":            "uid-1",
		"email":          "User@Example.com",
		"email_verified": true,
	})
	got, err := oidcForTest(srv).FetchIdentity(testClientCtx(srv), "code")
	if err != nil {
		t.Fatalf("FetchIdentity: %v", err)
	}
	want := Identity{ProviderUserID: "uid-1", Email: "User@Example.com", EmailVerified: true}
	if got != want {
		t.Errorf("identity = %+v, want %+v", got, want)
	}
}

func TestOIDC_FetchIdentityUnverifiedEmail(t *testing.T) {
	srv := stubOIDC(t, map[string]any{"sub": "uid-2", "email": "u@e.com", "email_verified": false})
	got, err := oidcForTest(srv).FetchIdentity(testClientCtx(srv), "code")
	if err != nil {
		t.Fatalf("FetchIdentity: %v", err)
	}
	if got.EmailVerified {
		t.Error("EmailVerified = true, want false")
	}
}

func TestOIDC_FetchIdentityMissingSub(t *testing.T) {
	srv := stubOIDC(t, map[string]any{"email": "u@e.com"})
	if _, err := oidcForTest(srv).FetchIdentity(testClientCtx(srv), "code"); err == nil {
		t.Error("want error for userinfo without sub")
	}
}

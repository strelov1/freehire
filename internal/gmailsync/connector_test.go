package gmailsync

import (
	"net/url"
	"strings"
	"testing"
)

func TestAuthCodeURL(t *testing.T) {
	c := NewConnector("client-123", "secret", "https://freehire.dev")
	raw := c.AuthCodeURL("state-xyz")

	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	q := u.Query()
	if q.Get("state") != "state-xyz" {
		t.Errorf("state = %q", q.Get("state"))
	}
	if q.Get("client_id") != "client-123" {
		t.Errorf("client_id = %q", q.Get("client_id"))
	}
	// Offline + forced consent are required to receive a refresh token.
	if q.Get("access_type") != "offline" {
		t.Errorf("access_type = %q, want offline", q.Get("access_type"))
	}
	if q.Get("prompt") != "consent" {
		t.Errorf("prompt = %q, want consent", q.Get("prompt"))
	}
	// Incremental authorization keeps previously-granted (sign-in) scopes.
	if q.Get("include_granted_scopes") != "true" {
		t.Errorf("include_granted_scopes = %q, want true", q.Get("include_granted_scopes"))
	}
	if !strings.Contains(q.Get("scope"), "gmail.readonly") {
		t.Errorf("scope missing gmail.readonly: %q", q.Get("scope"))
	}
	if q.Get("redirect_uri") != "https://freehire.dev/api/v1/me/gmail/callback" {
		t.Errorf("redirect_uri = %q", q.Get("redirect_uri"))
	}
}

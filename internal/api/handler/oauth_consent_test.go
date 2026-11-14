package handler

import (
	"strings"
	"testing"
)

func TestRedirectDestination(t *testing.T) {
	cases := map[string]string{
		"https://attacker.example.com/cb":                    "attacker.example.com",
		"https://claude.ai/api/mcp/auth_callback":            "claude.ai",
		"http://127.0.0.1:51739/callback":                    "an app on this computer (127.0.0.1)",
		"cursor://anysphere.cursor-retrieval/oauth/callback": "the cursor:// app on this computer",
	}
	for uri, want := range cases {
		if got := redirectDestination(uri); got != want {
			t.Errorf("redirectDestination(%q) = %q, want %q", uri, got, want)
		}
	}
}

// A client may call itself anything, so the page must name the redirect host
// beside the self-chosen name — that host is what a phishing registration
// cannot fake.
func TestOAuthConsentPage_NamesRedirectHostAndFlagsUnverifiedName(t *testing.T) {
	page, err := oauthConsentPage("Claude", "me@example.com", "https://freehire.me", authorizeParams{
		ClientID:    "cid",
		RedirectURI: "https://attacker.example.com/cb",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(page, "<strong>attacker.example.com</strong>") {
		t.Errorf("consent page does not name the redirect host:\n%s", page)
	}
	if !strings.Contains(page, "not verified") {
		t.Errorf("consent page does not say the client name is unverified:\n%s", page)
	}
}

// The resource a client asked for must survive the consent round trip, or the
// POST that mints the code would bind nothing.
func TestOAuthConsentPage_CarriesResourceOnlyWhenBound(t *testing.T) {
	params := authorizeParams{ClientID: "cid", RedirectURI: "https://app.example.com/cb", Resource: "/api/v1/mcp"} // the bound PATH, as resolveAuthorizeResource stores it
	page, err := oauthConsentPage("Claude", "me@example.com", "https://freehire.me", params)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(page, `name="resource" value="https://freehire.me/api/v1/mcp"`) {
		t.Errorf("consent form dropped the resource:\n%s", page)
	}
	params.Resource = ""
	page, err = oauthConsentPage("Claude", "me@example.com", "https://freehire.me", params)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(page, `name="resource"`) {
		t.Errorf("an unbound request must not emit a resource field:\n%s", page)
	}
}

package handler

import (
	"strings"
	"testing"
)

const hostile = `"><script>alert(1)</script>`

// Every field a client or a query string controls must reach the page inert:
// the client name is self-chosen at registration and state is whatever the
// caller put in the URL.
func TestConsentScreens_EscapeEveryCallerControlledValue(t *testing.T) {
	oauth, err := oauthConsentPage(hostile, hostile, "https://freehire.me", authorizeParams{
		ClientID: hostile, RedirectURI: "https://evil.example/cb", State: hostile,
		CodeChallenge: hostile, CodeChallengeMethod: hostile,
	})
	if err != nil {
		t.Fatal(err)
	}
	ext, err := consentPage(hostile, hostile, hostile, "https://freehire.me")
	if err != nil {
		t.Fatal(err)
	}
	for name, page := range map[string]string{"oauth": oauth, "extension": ext} {
		if strings.Contains(page, "<script>") {
			t.Errorf("%s consent page carries an unescaped value:\n%s", name, page)
		}
		if !strings.Contains(page, "&lt;script&gt;") {
			t.Errorf("%s consent page dropped the value instead of escaping it", name)
		}
	}
}

// Cancel must post something other than "allow": both POST handlers approve on
// exactly that value and refuse on anything else.
func TestConsentScreen_ButtonsAndHiddenFields(t *testing.T) {
	page, err := consentPage("https://x.chromiumapp.org/", "st8", "me@example.com", "https://freehire.me/")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`name="decision" value="deny"`,
		`name="decision" value="allow"`,
		`name="redirect_uri" value="https://x.chromiumapp.org/"`,
		`name="state" value="st8"`,
		`action="/api/v1/auth/extension/connect"`,
		"Signed in as me@example.com",
		`href="https://freehire.me/privacy"`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("extension consent page lacks %q", want)
		}
	}
}

// The list is what a person consents to, so it must say what the credential
// cannot reach as well as what it can.
func TestConsentScreen_StatesWhatTheCredentialCannotDo(t *testing.T) {
	page, err := oauthConsentPage("freehire CLI", "", "https://freehire.me", authorizeParams{RedirectURI: "http://127.0.0.1:5000/callback"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(page, "No access to your password") {
		t.Errorf("consent page does not state what the grant cannot reach")
	}
	if strings.Contains(page, "Signed in as") {
		t.Errorf("an empty email still rendered the signed-in line")
	}
}

func TestNoSessionPage_LinksToSignIn(t *testing.T) {
	page, err := noSessionPage("https://freehire.me/")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(page, `href="https://freehire.me/signin"`) {
		t.Errorf("no-session page does not link to sign-in:\n%s", page)
	}
}

func TestConsentScreen_OffersSwitchAccountOnlyWhenSignedIn(t *testing.T) {
	signed, err := renderConsentScreen(consentScreen{SignedIn: "me@example.com", Action: "/x", Origin: "https://freehire.me"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(signed, `form="consent" name="decision" value="switch"`) || !strings.Contains(signed, `<form id="consent"`) {
		t.Errorf("no switch-account button bound to the consent form:\n%s", signed)
	}
	anon, err := renderConsentScreen(consentScreen{Action: "/x", Origin: "https://freehire.me"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(anon, "Switch account") {
		t.Error("switch-account shown with no signed-in account")
	}
}

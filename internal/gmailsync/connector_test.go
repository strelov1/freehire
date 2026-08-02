package gmailsync

import (
	"net/url"
	"strings"
	"testing"
)

func TestAuthCodeURL(t *testing.T) {
	c := NewConnector("client-123", "secret", "https://freehire.me")
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
	if q.Get("redirect_uri") != "https://freehire.me/api/v1/me/gmail/callback" {
		t.Errorf("redirect_uri = %q", q.Get("redirect_uri"))
	}
}

// Connecting a mailbox must not quietly ask for a calendar. They are separate consents
// because they are separate costs: gmail.readonly is a restricted scope and
// calendar.readonly a sensitive one, and a candidate who wants an inbox and not a
// calendar has to be able to say so.
func TestConsentURLsAskForOneThingEach(t *testing.T) {
	c := NewConnector("id", "secret", "https://freehire.me")

	mail := c.AuthCodeURL("state-1")
	calendar := c.CalendarAuthCodeURL("state-2")

	if strings.Contains(mail, "calendar") {
		t.Errorf("the mail consent requested a calendar scope: %s", mail)
	}
	if !strings.Contains(mail, url.QueryEscape(GmailReadonlyScope)) {
		t.Errorf("the mail consent did not request gmail.readonly: %s", mail)
	}
	if !strings.Contains(calendar, url.QueryEscape(CalendarScope)) {
		t.Errorf("the calendar consent did not request the calendar scope: %s", calendar)
	}
	// Incremental: the calendar consent must keep the grants the candidate already gave,
	// or accepting it would cost them their connected mailbox.
	if !strings.Contains(calendar, "include_granted_scopes=true") {
		t.Errorf("the calendar consent was not incremental: %s", calendar)
	}
}

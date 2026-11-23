package gmailsync

import (
	"net/url"
	"slices"
	"strings"
	"testing"
)

func TestSchedulingRequestsExplicitScopesWithoutChangingGmailConnect(t *testing.T) {
	client := NewConnector("client", "secret", "https://freehire.test")
	mailURL, err := url.Parse(client.AuthCodeURL("mail"))
	if err != nil {
		t.Fatal(err)
	}
	schedulerURL, err := url.Parse(client.SchedulingAuthCodeURL("schedule"))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{GmailReadonlyScope, GmailSendScope, CalendarScope, CalendarEventsScope}
	if got := strings.Fields(schedulerURL.Query().Get("scope")); !slices.Equal(got, want) {
		t.Fatalf("scheduling scopes = %v, want %v", got, want)
	}
	if got := strings.Fields(mailURL.Query().Get("scope")); !slices.Equal(got, []string{GmailReadonlyScope}) {
		t.Fatalf("ordinary Gmail scopes changed: %v", got)
	}
	if schedulerURL.Query().Get("state") != "schedule" ||
		schedulerURL.Query().Get("redirect_uri") != mailURL.Query().Get("redirect_uri") {
		t.Fatal("consent must use its own state and the registered Gmail callback")
	}
}

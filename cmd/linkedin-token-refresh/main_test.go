package main

import (
	"strings"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/engage/linkedinauth"
)

var renderNow = time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)

// The published series are what an alert rule is written against, so their names and labels are
// a contract with a file in another repository. A rename here that nobody notices is an alert
// that silently stops firing.
func TestRenderPublishesTheDocumentedSeries(t *testing.T) {
	out := render(linkedinauth.Outcome{
		State:            linkedinauth.StateHealthy,
		ExpiresAt:        renderNow.Add(30 * 24 * time.Hour),
		RefreshExpiresAt: renderNow.Add(300 * 24 * time.Hour),
	}, renderNow)

	for _, want := range []string{
		`freehire_social_token_expires_in_seconds{channel="linkedin"} 2592000`,
		`freehire_social_token_grant_expires_in_seconds{channel="linkedin"} 25920000`,
		`freehire_social_token_renewable{channel="linkedin"} 1`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing series %q in:\n%s", want, out)
		}
	}
	// Every family needs its HELP and TYPE, or the collector drops the file entirely and the
	// worker looks like it published nothing.
	if strings.Count(out, "# TYPE") != 3 {
		t.Errorf("expected three TYPE lines:\n%s", out)
	}
}

// A channel nobody has signed in must not look identical to a broken exporter (no series) or to
// a credential that expired this instant (zero). Negative says "there is none", which is the
// state that actually needs a person.
func TestRenderMarksAnAbsentCredentialNegative(t *testing.T) {
	out := render(linkedinauth.Outcome{State: linkedinauth.StateMissing}, renderNow)

	if !strings.Contains(out, `freehire_social_token_expires_in_seconds{channel="linkedin"} -1`) {
		t.Errorf("an absent credential is not marked negative:\n%s", out)
	}
	if !strings.Contains(out, `freehire_social_token_renewable{channel="linkedin"} 0`) {
		t.Errorf("an absent credential is not marked unrenewable:\n%s", out)
	}
}

// A token that cannot be renewed is the case the whole warning path exists for, and the gauge
// is how anybody learns about it before the channel stops.
func TestRenderMarksAnUnrenewableCredential(t *testing.T) {
	out := render(linkedinauth.Outcome{
		State:     linkedinauth.StateWarn,
		ExpiresAt: renderNow.Add(10 * 24 * time.Hour),
	}, renderNow)

	if !strings.Contains(out, `freehire_social_token_renewable{channel="linkedin"} 0`) {
		t.Errorf("a token with no refresh reported itself renewable:\n%s", out)
	}
	if !strings.Contains(out, `freehire_social_token_grant_expires_in_seconds{channel="linkedin"} -1`) {
		t.Errorf("a missing grant expiry is not marked negative:\n%s", out)
	}
}

package llm

import (
	"testing"
	"time"
)

// The regression this whole type exists for.
//
// Attribution used to travel as a whole *Client, so a component built with a deliberately
// chosen timeout had that client REPLACED for every signed-in caller — and ran on the
// default instead. On prod that turned a 120s résumé-extraction budget into 90s, which is
// exactly long enough to fail on a long CV: the candidate ended up with no banked
// experience, no fit analysis, and no explanation for either.
//
// Asserting on the field rather than on an observed request deliberately. The timeout is
// what bounds a call, so nothing about a call that RETURNS can show it was carried; the
// alternative is a test that sleeps.
func TestAsCallerKeepsTheTimeoutTheClientWasBuiltWith(t *testing.T) {
	const chosen = 120 * time.Second

	base, err := New("https://gateway.example", "service-key", "model-x")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	configured := base.WithTimeout(chosen)
	if configured.timeout != chosen {
		t.Fatalf("precondition: configured timeout = %s, want %s", configured.timeout, chosen)
	}

	bound := configured.AsCaller(NewCaller("a-users-own-key", nil, Feature("cv-extract")))
	if bound.timeout != chosen {
		t.Errorf("timeout after binding a caller = %s, want %s — the entrypoint's budget was discarded", bound.timeout, chosen)
	}
	if bound.apiKey != "a-users-own-key" {
		t.Errorf("credential after binding = %q, want the caller's own", bound.apiKey)
	}
}

// A caller with no credential of its own still tags the call, and still must not disturb
// anything else the client was built with: attribution fails open, and failing open must
// not mean falling back to a different configuration.
func TestAsCallerWithNoSecretKeepsTheTimeoutAndTheServiceCredential(t *testing.T) {
	const chosen = 45 * time.Second

	base, err := New("https://gateway.example", "service-key", "model-x")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	bound := base.WithTimeout(chosen).AsCaller(NewCaller("", nil, Feature("cv-extract")))

	if bound.timeout != chosen {
		t.Errorf("timeout = %s, want %s", bound.timeout, chosen)
	}
	if bound.apiKey != "service-key" {
		t.Errorf("credential = %q, want the service's own when the caller has none", bound.apiKey)
	}
}

// The zero Caller names nobody and nothing, which is what a background job hands over. It
// must be free: no credential swap, no tags, and — the point — no change of configuration.
func TestAsCallerWithTheZeroValueChangesNothing(t *testing.T) {
	base, err := New("https://gateway.example", "service-key", "model-x")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	configured := base.WithTimeout(30 * time.Second)

	if got := configured.AsCaller(Caller{}); got != configured {
		t.Errorf("AsCaller(zero) returned a different client; want the receiver untouched")
	}
}

package main

import (
	"strings"
	"testing"
)

// -rotating is asked for on purpose, so a missing or broken gateway must stop the run rather
// than fall back to the egress that was feeding the ban.
func TestEgressForRotatingRefusesAMissingOrBrokenGateway(t *testing.T) {
	t.Setenv("SOURCES_ROTATING_PROXY_URL", "")
	if _, err := egressFor("workable", true); err == nil {
		t.Fatal("egressFor with no gateway: want an error, got nil")
	}
	t.Setenv("SOURCES_ROTATING_PROXY_URL", "http://user:sup3rsecret@proxy.example:80/\x7f")
	_, err := egressFor("workable", true)
	if err == nil {
		t.Fatal("egressFor with an unparseable gateway: want an error, got nil")
	}
	if strings.Contains(err.Error(), "sup3rsecret") {
		t.Fatalf("error leaks the proxy credential: %v", err)
	}
}

func TestEgressForRotatingBuildsAClientFromTheGateway(t *testing.T) {
	t.Setenv("SOURCES_ROTATING_PROXY_URL", "http://user-rotate:pass@p.example:80")
	if c, err := egressFor("workable", true); err != nil || c == nil {
		t.Fatalf("egressFor = (%v, %v), want a client", c, err)
	}
}

// pandapelatam probes through a client of its own; -rotating must refuse it rather than let that
// client silently replace the rotating egress.
func TestWithProberClientRefusesRotatingForAProberWithItsOwnClient(t *testing.T) {
	p, ok := proberFor("pandapelatam")
	if !ok {
		t.Skip("pandapelatam prober not registered")
	}
	if _, err := withProberClient(p, nil, true); err == nil {
		t.Fatal("withProberClient(-rotating) = nil error, want a refusal")
	}
	if c, err := withProberClient(p, nil, false); err != nil || c == nil {
		t.Fatalf("withProberClient(no -rotating) = (%v, %v), want the prober's own client", c, err)
	}
}

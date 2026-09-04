package ingestsched

import (
	"errors"
	"testing"

	"github.com/strelov1/freehire/internal/ingest/sources"
)

// The scheduler builds an argv and a systemd unit name out of boards.provider, and board
// rows can originate from crowdsourced submissions. This gate is the reason a hostile or
// merely malformed provider key cannot reach either.
func TestValidateProviderKeyAcceptsRegisteredKeys(t *testing.T) {
	// Read the registry rather than hard-coding names: a test that pins a provider list
	// fails on every onboarding, and would tempt the next person to loosen the gate.
	registry := sources.Taxonomy()
	for _, key := range []string{"greenhouse", "lever", "habr_career"} {
		if _, ok := registry[key]; !ok {
			t.Fatalf("test premise broken: %q is not a registered provider", key)
		}
		if err := ValidateProviderKey(key); err != nil {
			t.Errorf("ValidateProviderKey(%q) = %v, want nil", key, err)
		}
	}
}

func TestValidateProviderKeyRejectsUnregisteredKeys(t *testing.T) {
	// habrcareer (no underscore) is the real one: it was the FILE name, it became the
	// systemd unit name, and after cmd/ingest started taking a provider name it matched
	// no boards and exited 0 for a day.
	for _, key := range []string{"habrcareer", "careerspage-typo", "notaprovider"} {
		err := ValidateProviderKey(key)
		if !errors.Is(err, ErrUnknownProvider) {
			t.Errorf("ValidateProviderKey(%q) = %v, want ErrUnknownProvider", key, err)
		}
	}
}

// The character check runs BEFORE the registry lookup, so a key that could not be a unit
// name is rejected on its shape even if some future registry entry were to carry it.
func TestValidateProviderKeyRejectsUnsafeShapes(t *testing.T) {
	for _, key := range []string{
		"",
		"greenhouse lever",    // a space splits one argv element into two
		"greenhouse;rm -rf /", // shell metacharacters
		"greenhouse$(whoami)", // command substitution
		"green/house",         // a path separator inside a unit name
		"../../etc/passwd",    // traversal
		"greenhouse\nlever",   // a newline ends a systemd directive
		"Greenhouse",          // provider keys are lower-case; a case variant is a typo
		"greenhouse@instance", // @ is systemd's own instance separator
		"greenhouse%i",        // % is systemd's specifier prefix
	} {
		err := ValidateProviderKey(key)
		if !errors.Is(err, ErrUnsafeProviderKey) {
			t.Errorf("ValidateProviderKey(%q) = %v, want ErrUnsafeProviderKey", key, err)
		}
	}
}

// Every key the live registry carries must pass the shape check, or the gate would refuse
// a provider the fleet actually crawls. This is the test that would have caught a shape
// rule written to fit "greenhouse" and blind to "habr_career" or "whatjobs-br".
func TestEveryRegisteredProviderKeyPassesTheShapeCheck(t *testing.T) {
	for key := range sources.Taxonomy() {
		if err := ValidateProviderKey(key); err != nil {
			t.Errorf("registered provider %q is refused by its own gate: %v", key, err)
		}
	}
}

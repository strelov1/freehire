package ingestsched

import (
	"errors"
	"fmt"

	"github.com/strelov1/freehire/internal/ingest/sources"
)

// Why a provider key may not be scheduled. Both are reported and skipped, never launched:
// one bad row must not stop the fleet, and it must not be silent either.
var (
	// ErrUnsafeProviderKey is a key whose SHAPE disqualifies it — it could not be a
	// systemd unit name or a single argv element.
	ErrUnsafeProviderKey = errors.New("provider key has an unsafe shape")

	// ErrUnknownProvider is a well-shaped key that no adapter answers to. This is the
	// habr_career failure in its general form: a name that selects nothing crawls
	// nothing and exits 0, which reads exactly like a healthy empty provider.
	ErrUnknownProvider = errors.New("provider is not in the adapter registry")
)

// ValidateProviderKey gates a boards.provider value on its way to an argv and a systemd
// unit name.
//
// Board rows can originate from crowdsourced submissions, so this is a security boundary
// and not a tidiness check. The shape test runs FIRST and independently of the registry:
// the registry is data that changes, while "this cannot be a unit name" is a property of
// the string, and a future registry entry must not be able to smuggle a shape past.
func ValidateProviderKey(key string) error {
	if !safeProviderKey(key) {
		return fmt.Errorf("%q: %w", key, ErrUnsafeProviderKey)
	}
	if _, ok := sources.Taxonomy()[key]; !ok {
		return fmt.Errorf("%q: %w", key, ErrUnknownProvider)
	}
	return nil
}

// safeProviderKey allows lower-case ASCII letters, digits, '_' and '-', and nothing else.
// That is exactly the alphabet the live registry uses (`habr_career`, `whatjobs-br`,
// `ashbygraphql`), and it excludes every character that means something to a shell, to a
// path, or to systemd — including '@' (its instance separator) and '%' (its specifier
// prefix), which would otherwise produce a unit that names a different run than intended.
//
// An allowlist rather than a denylist: a denylist is a list of the attacks someone thought
// of.
func safeProviderKey(key string) bool {
	if key == "" {
		return false
	}
	for _, r := range key {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= '0' && r <= '9':
		case r == '_' || r == '-':
		default:
			return false
		}
	}
	return true
}

package industrytag

import (
	"regexp"
	"testing"
)

// slugForm is the shape a canonical value must have to survive a filter URL.
var slugForm = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

func TestEveryCanonicalIsASlug(t *testing.T) {
	for canonical := range displayNames {
		if !slugForm.MatchString(canonical) {
			t.Errorf("canonical %q is not a slug: it can never be produced by normalize, "+
				"so it is a dead option in the facet list", canonical)
		}
	}
}

func TestEveryAliasTargetIsCanonical(t *testing.T) {
	for alias, canonical := range aliases {
		if _, ok := displayNames[canonical]; !ok {
			t.Errorf("alias %q points at %q, which is not a canonical value", alias, canonical)
		}
	}
}

// An alias key that normalize can never produce is dead weight: it silently never
// matches, and nothing else in the package would report it.
func TestEveryAliasKeyIsInNormalForm(t *testing.T) {
	for alias := range aliases {
		if got := normalize(alias); got != alias {
			t.Errorf("alias key %q is not in normal form: normalize gives %q", alias, got)
		}
	}
}

// A self-mapping alias is redundant — Canonicalize already admits a canonical value
// through displayNames — and doubles the dictionary for nothing.
func TestNoAliasMapsToItself(t *testing.T) {
	for alias, canonical := range aliases {
		if alias == canonical {
			t.Errorf("alias %q maps to itself; canonical values need no alias entry", alias)
		}
	}
}

// A floor on the dictionary's size, mirroring skilltag's. Every other invariant
// iterates the maps, so all of them stay vacuously green if the dictionary is
// truncated — deleting ninety canonicals would break nothing else in this file.
func TestDictionaryIsNotTruncated(t *testing.T) {
	// A floor against truncation, not a target size. The vocabulary is a hand-written
	// list that shrinks whenever curation merges two entries, so the floor sits well
	// below the current count — it must catch "the file got clobbered", not veto a
	// merge.
	const (
		minCanonicals = 60
		minAliases    = 150
	)
	if len(displayNames) < minCanonicals {
		t.Errorf("dictionary has %d canonical values, want at least %d", len(displayNames), minCanonicals)
	}
	if len(aliases) < minAliases {
		t.Errorf("dictionary has %d aliases, want at least %d", len(aliases), minAliases)
	}
}

// Two canonicals sharing display text render as two identical dropdown options,
// which is indistinguishable from a bug to whoever is using the filter.
func TestDisplayNamesAreUnique(t *testing.T) {
	seen := make(map[string]string, len(displayNames))
	for canonical, name := range displayNames {
		if other, ok := seen[name]; ok {
			t.Errorf("canonicals %q and %q both display as %q", other, canonical, name)
		}
		seen[name] = canonical
	}
}

func TestEveryCanonicalHasANonEmptyLabel(t *testing.T) {
	for canonical, name := range displayNames {
		if name == "" {
			t.Errorf("canonical %q has an empty display name", canonical)
		}
	}
}

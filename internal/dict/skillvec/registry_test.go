package skillvec

import (
	"testing"

	"github.com/strelov1/freehire/internal/dict/skilltag"
)

func TestRegistryHasNoDuplicates(t *testing.T) {
	seen := make(map[string]int, len(registry))
	for i, s := range registry {
		if prev, dup := seen[s]; dup {
			t.Fatalf("registry[%d] = %q duplicates registry[%d]; a slug must own exactly one position", i, s, prev)
		}
		seen[s] = i
	}
}

// TestRegistryCoversEveryCanonicalSkill is what keeps a newly-mined skill from
// silently ranking as absent: it has no position until the registry is regenerated.
func TestRegistryCoversEveryCanonicalSkill(t *testing.T) {
	in := make(map[string]bool, len(registry))
	for _, s := range registry {
		in[s] = true
	}
	for _, s := range skilltag.Canonicals() {
		if !in[s] {
			t.Errorf("canonical skill %q has no vector position — run go generate ./internal/dict/skillvec/", s)
		}
	}
}

func TestRegistryFitsTheDeclaredDimensions(t *testing.T) {
	if len(registry) > Dimensions {
		t.Fatalf("registry holds %d skills, past the declared %d dimensions; widening Dimensions requires a full reindex",
			len(registry), Dimensions)
	}
}

func TestPosition(t *testing.T) {
	if got, ok := Position(registry[0]); !ok || got != 0 {
		t.Errorf("Position(%q) = %d, %v; want 0, true", registry[0], got, ok)
	}
	last := len(registry) - 1
	if got, ok := Position(registry[last]); !ok || got != last {
		t.Errorf("Position(%q) = %d, %v; want %d, true", registry[last], got, ok, last)
	}
	if _, ok := Position("definitely-not-a-skill"); ok {
		t.Error("Position() resolved an unknown slug; unknowns must report false, never a guessed position")
	}
}

func TestRegistrySize(t *testing.T) {
	if RegistrySize() != len(registry) {
		t.Errorf("RegistrySize() = %d, want %d", RegistrySize(), len(registry))
	}
	if RegistrySize() > Dimensions {
		t.Errorf("RegistrySize() = %d exceeds Dimensions = %d", RegistrySize(), Dimensions)
	}
}

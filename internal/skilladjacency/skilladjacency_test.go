package skilladjacency

import "testing"

func TestAdjacentVia(t *testing.T) {
	held := map[string]bool{"gcp": true, "vue": true}

	if via, ok := AdjacentVia("aws", held); !ok || via != "gcp" {
		t.Errorf("AdjacentVia(aws) = (%q, %v), want (gcp, true)", via, ok)
	}
	if via, ok := AdjacentVia("react", held); !ok || via != "vue" {
		t.Errorf("AdjacentVia(react) = (%q, %v), want (vue, true)", via, ok)
	}
	// No neighbour held.
	if via, ok := AdjacentVia("pytorch", held); ok || via != "" {
		t.Errorf("AdjacentVia(pytorch) = (%q, %v), want (\"\", false)", via, ok)
	}
	// Skill with no adjacency entry at all.
	if via, ok := AdjacentVia("rust", held); ok || via != "" {
		t.Errorf("AdjacentVia(rust) = (%q, %v), want (\"\", false)", via, ok)
	}
}

func TestAdjacentVia_FirstListedNeighbourWins(t *testing.T) {
	// aws neighbours are [gcp, azure] in listed order; hold only azure.
	held := map[string]bool{"azure": true}
	if via, ok := AdjacentVia("aws", held); !ok || via != "azure" {
		t.Errorf("AdjacentVia(aws) = (%q, %v), want (azure, true)", via, ok)
	}
}

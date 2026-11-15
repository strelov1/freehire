package skilltag

import (
	"slices"
	"testing"
)

func TestRequestedSkills(t *testing.T) {
	cases := []struct{ term, canonical string }{
		{".net maui", "dotnet-maui"},
		{"dotnet maui", "dotnet-maui"},
		{"sourcegraph", "sourcegraph"},
		{"lightroom", "lightroom"},
		{"adobe lightroom", "lightroom"},
		{"etix", "etix"},
		{"design tokens", "design-tokens"},
		{"design token", "design-tokens"},
		{"figma plugins", "figma-plugins"},
		{"figma plugin", "figma-plugins"},
		{"monorepo", "monorepo"},
		{"monorepos", "monorepo"},
		{"system integration", "system-integration"},
		{"systems integration", "system-integration"},
		{"data reconciliation", "data-reconciliation"},
		{"sop development", "sop-development"},
		{"standard operating procedure development", "sop-development"},
		{"event ticketing", "event-ticketing"},
		{"box office management", "box-office-management"},
		{"pos terminals", "pos-terminals"},
		{"pos terminal", "pos-terminals"},
		{"point of sale terminals", "pos-terminals"},
		{"point-of-sale terminal", "pos-terminals"},
		{"cctv", "cctv"},
		{"closed circuit television", "cctv"},
		{"structured cabling", "structured-cabling"},
		{"structured cable system", "structured-cabling"},
		{"structured cabling systems", "structured-cabling"},
		{"MS Fabric", "microsoft-fabric"},
		{"MS Purview", "microsoft-purview"},
	}
	for _, tc := range cases {
		t.Run(tc.term, func(t *testing.T) {
			if got := Parse("Experience with " + tc.term + " required."); !slices.Contains(got, tc.canonical) {
				t.Fatalf("Parse missing %q: %v", tc.canonical, got)
			}
			if got := Canonicalize([]string{tc.term}); !slices.Contains(got, tc.canonical) {
				t.Fatalf("Canonicalize missing %q: %v", tc.canonical, got)
			}
		})
	}
}

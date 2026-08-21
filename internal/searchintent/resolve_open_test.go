package searchintent

import (
	"slices"
	"testing"
)

// The open vocabularies — skills, countries, cities — are too large to name in a
// prompt, so the model writes ordinary words and the dictionaries that already own
// those vocabularies decide what they mean. What no dictionary places is dropped and
// reported; nothing is guessed.

func resolveFacet(t *testing.T, facet string, values ...string) Result {
	t.Helper()
	got, err := intent{Facets: map[string][]string{facet: values}}.resolve()
	if err != nil {
		t.Fatalf("resolve %s=%v: %v", facet, values, err)
	}
	return got
}

func TestResolveCanonicalisesSkillAlias(t *testing.T) {
	got := resolveFacet(t, "skills", "Golang")
	if !slices.Equal(got.Facets["skills"], []string{"go"}) {
		t.Fatalf("skills = %v, want [go]", got.Facets["skills"])
	}
}

func TestResolveDropsSkillNoDictionaryPlaces(t *testing.T) {
	got := resolveFacet(t, "skills", "blockchain-adjacent")
	if len(got.Facets["skills"]) != 0 {
		t.Fatalf("skills = %v, want none", got.Facets["skills"])
	}
	if !slices.Contains(got.Unresolved, "blockchain-adjacent") {
		t.Fatalf("unresolved = %v, want it to name the dropped skill", got.Unresolved)
	}
}

func TestResolveCountryNameToCode(t *testing.T) {
	got := resolveFacet(t, "countries", "Portugal")
	if !slices.Equal(got.Facets["countries"], []string{"pt"}) {
		t.Fatalf("countries = %v, want [pt]", got.Facets["countries"])
	}
}

// A country name resolves to a region as well, but only the country belongs in the
// countries facet — writing the region too would widen the search past what was asked.
func TestResolveCountryDoesNotLeakIntoRegions(t *testing.T) {
	got := resolveFacet(t, "countries", "Portugal")
	if len(got.Facets["regions"]) != 0 {
		t.Fatalf("regions = %v, want none — only countries were asked for", got.Facets["regions"])
	}
}

func TestResolveDropsUnknownCountry(t *testing.T) {
	got := resolveFacet(t, "countries", "Freedonia")
	if len(got.Facets["countries"]) != 0 {
		t.Fatalf("countries = %v, want none", got.Facets["countries"])
	}
	if !slices.Contains(got.Unresolved, "Freedonia") {
		t.Fatalf("unresolved = %v, want it to name the dropped country", got.Unresolved)
	}
}

func TestResolveCityToCanonicalName(t *testing.T) {
	got := resolveFacet(t, "cities", "lisbon")
	if !slices.Equal(got.Facets["cities"], []string{"Lisbon"}) {
		t.Fatalf("cities = %v, want [Lisbon]", got.Facets["cities"])
	}
}

// The city dictionary matches on prefix, so a fragment finds a city that was never
// asked for. Resolving "Ber" to Berlin is a guess, and this package does not guess.
func TestResolveDropsCityFragment(t *testing.T) {
	got := resolveFacet(t, "cities", "Ber")
	if len(got.Facets["cities"]) != 0 {
		t.Fatalf("cities = %v, want none — \"Ber\" names no city", got.Facets["cities"])
	}
	if !slices.Contains(got.Unresolved, "Ber") {
		t.Fatalf("unresolved = %v, want it to name the dropped fragment", got.Unresolved)
	}
}

func TestResolveDropsUnknownCity(t *testing.T) {
	got := resolveFacet(t, "cities", "Zzzqqq")
	if len(got.Facets["cities"]) != 0 {
		t.Fatalf("cities = %v, want none", got.Facets["cities"])
	}
}

func TestResolveKeepsCuratedCollection(t *testing.T) {
	got := resolveFacet(t, "collections", "yc")
	if !slices.Equal(got.Facets["collections"], []string{"yc"}) {
		t.Fatalf("collections = %v, want [yc]", got.Facets["collections"])
	}
}

func TestResolveDropsUncuratedCollection(t *testing.T) {
	got := resolveFacet(t, "collections", "unicorns-of-2019")
	if len(got.Facets["collections"]) != 0 {
		t.Fatalf("collections = %v, want none", got.Facets["collections"])
	}
	if !slices.Contains(got.Unresolved, "unicorns-of-2019") {
		t.Fatalf("unresolved = %v, want it to name the dropped collection", got.Unresolved)
	}
}

func TestResolveKeepsCatalogedRole(t *testing.T) {
	got := resolveFacet(t, "role", "senior_backend")
	if !slices.Equal(got.Facets["role"], []string{"senior_backend"}) {
		t.Fatalf("role = %v, want [senior_backend]", got.Facets["role"])
	}
}

func TestResolveDropsUncatalogedRole(t *testing.T) {
	got := resolveFacet(t, "role", "chief_vibes_officer")
	if len(got.Facets["role"]) != 0 {
		t.Fatalf("role = %v, want none", got.Facets["role"])
	}
}

func TestResolveKeepsDomain(t *testing.T) {
	got := resolveFacet(t, "domains", "fintech")
	if !slices.Equal(got.Facets["domains"], []string{"fintech"}) {
		t.Fatalf("domains = %v, want [fintech]", got.Facets["domains"])
	}
}

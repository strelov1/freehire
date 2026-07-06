package roletag

import (
	"slices"
	"testing"

	"github.com/strelov1/freehire/internal/enrich"
)

func TestDerive(t *testing.T) {
	cases := []struct {
		name      string
		seniority string
		category  string
		title     string
		want      []string
	}{
		// Composite fires only when both axes are resolved.
		{"composite from both axes", "senior", "backend", "Senior Backend Engineer", []string{"senior_backend"}},
		{"composite mid frontend", "middle", "frontend", "Middle Frontend Developer", []string{"middle_frontend"}},
		{"composite lead devops", "lead", "devops", "Lead DevOps Engineer", []string{"lead_devops"}},

		// Composite requires BOTH — a lone seniority or category yields no composite.
		{"seniority only, no named match", "staff", "", "Staff Software Engineer", nil},
		{"category only, no named match", "", "backend", "Backend Developer", nil},

		// Named roles come from the title regardless of the grid.
		{"founding engineer, empty grid", "", "", "Founding Engineer", []string{"founding_engineer"}},
		{"cloud solutions engineer beats adjacency gap", "", "", "Cloud Solutions Engineer", []string{"cloud_solutions_engineer"}},
		{"technical lead", "lead", "", "Technical Lead", []string{"technical_lead"}},
		{"tech lead alias", "lead", "", "Tech Lead", []string{"technical_lead"}},
		{"fractional cto", "c_level", "", "Fractional CTO", []string{"fractional_cto"}},

		// Composite + named coexist and dedupe.
		{"composite plus named", "senior", "backend", "Senior Backend Founding Engineer", []string{"senior_backend", "founding_engineer"}},

		// Never guesses.
		{"nothing resolvable", "", "", "Rockstar Ninja Guru", nil},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Derive(tc.seniority, tc.category, tc.title)
			if !slices.Equal(got, tc.want) {
				t.Fatalf("Derive(%q,%q,%q) = %v, want %v", tc.seniority, tc.category, tc.title, got, tc.want)
			}
		})
	}
}

// Every slug Derive can emit MUST have a catalog label; conversely no derivable
// slug is missing from the catalog. The catalog is the label source of truth.
func TestEveryDerivedSlugIsInCatalog(t *testing.T) {
	cat := Catalog()

	titles := []string{
		"Founding Engineer", "Cloud Solutions Engineer", "Solutions Engineer",
		"Technical Lead", "Fractional CTO", "Staff Engineer",
	}
	var derived []string
	for _, ttl := range titles {
		derived = append(derived, Derive("", "", ttl)...)
	}
	// A representative composite.
	derived = append(derived, Derive("senior", "backend", "Senior Backend Engineer")...)

	for _, slug := range derived {
		if _, ok := cat[slug]; !ok {
			t.Errorf("derived slug %q has no catalog entry", slug)
		}
	}
}

// Every seniority × non-"other" category MUST produce its composite slug, so an
// incomplete label map can't silently drop resolvable roles.
func TestCompositeCoversEverySeniorityAndCategory(t *testing.T) {
	for _, s := range enrich.SeniorityValues {
		for _, c := range enrich.CategoryValues {
			if c == "other" {
				continue
			}
			slug := s + "_" + c
			got := Derive(s, c, "")
			if !slices.Contains(got, slug) {
				t.Errorf("Derive(%q,%q) = %v, missing composite %q", s, c, got, slug)
			}
			if _, ok := Catalog()[slug]; !ok {
				t.Errorf("composite %q missing from catalog", slug)
			}
		}
	}
}

// Every named-role alias's slug MUST have a label, so the ordered alias list and
// the label map can't drift apart (a slug in one but not the other).
func TestEveryNamedRoleHasALabel(t *testing.T) {
	cat := Catalog()
	for _, nr := range namedRoles {
		if _, ok := cat[nr.slug]; !ok {
			t.Errorf("named role alias %q → slug %q has no catalog label", nr.alias, nr.slug)
		}
	}
}

func TestCatalogLabelsAreNonEmpty(t *testing.T) {
	for slug, label := range Catalog() {
		if label == "" {
			t.Errorf("catalog slug %q has an empty label", slug)
		}
	}
}

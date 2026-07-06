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
		// A resolved category always yields the bare role; the composite is added on
		// top when seniority is also resolved. Order: bare, composite, named.
		{"composite adds bare + graded", "senior", "backend", "Senior Backend Engineer", []string{"backend", "senior_backend"}},
		{"composite mid frontend", "middle", "frontend", "Middle Frontend Developer", []string{"frontend", "middle_frontend"}},
		{"composite lead devops", "lead", "devops", "Lead DevOps Engineer", []string{"devops", "lead_devops"}},

		// Bare category role with no seniority — the dominant real-world case.
		{"bare category, no seniority", "", "data_science", "Data Scientist", []string{"data_science"}},
		{"bare category product", "", "product", "Product Manager", []string{"product"}},

		// Category "other" yields no bare role (no natural role noun).
		{"category other yields nothing", "", "other", "Coordinator", nil},

		// Named roles come from the title regardless of the grid.
		{"software engineer catch-all", "", "", "Software Engineer", []string{"software_engineer"}},
		{"founding engineer, empty grid", "", "", "Founding Engineer", []string{"founding_engineer"}},
		{"cloud solutions engineer beats adjacency gap", "", "", "Cloud Solutions Engineer", []string{"cloud_solutions_engineer"}},
		{"technical lead", "lead", "", "Technical Lead", []string{"technical_lead"}},
		{"tech lead alias", "lead", "", "Tech Lead", []string{"technical_lead"}},
		{"fractional cto", "c_level", "", "Fractional CTO", []string{"fractional_cto"}},
		// Length-ordered aliases: the longer, more specific phrase wins.
		{"technical account manager beats account manager", "", "sales", "Technical Account Manager", []string{"sales", "technical_account_manager"}},

		// Bare category + composite + one named coexist without duplicates.
		{"bare + composite + named", "senior", "backend", "Senior Backend Founding Engineer", []string{"backend", "senior_backend", "founding_engineer"}},

		// Never guesses: no category and no named alias.
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

// Every non-"other" category MUST produce its bare role, and every
// seniority × non-"other" category its composite — so an incomplete label map
// can't silently drop resolvable roles, and both are present in the catalog.
func TestBareAndCompositeCoverEveryCategory(t *testing.T) {
	cat := Catalog()
	for _, c := range enrich.CategoryValues {
		if c == "other" {
			continue
		}
		if got := Derive("", c, ""); !slices.Contains(got, c) {
			t.Errorf("Derive(\"\",%q) = %v, missing bare role %q", c, got, c)
		}
		if _, ok := cat[c]; !ok {
			t.Errorf("bare category role %q missing from catalog", c)
		}
		for _, s := range enrich.SeniorityValues {
			slug := s + "_" + c
			if got := Derive(s, c, ""); !slices.Contains(got, slug) {
				t.Errorf("Derive(%q,%q) = %v, missing composite %q", s, c, got, slug)
			}
			if _, ok := cat[slug]; !ok {
				t.Errorf("composite %q missing from catalog", slug)
			}
		}
	}
}

// Every built alias resolves to a slug that has a catalog label, and every named
// role has at least one alias — so the alias list and the label map (both built
// from namedRoleTable) can't drift apart.
func TestEveryNamedRoleHasALabelAndAlias(t *testing.T) {
	cat := Catalog()
	for _, na := range namedAliases {
		if _, ok := cat[na.slug]; !ok {
			t.Errorf("alias %q → slug %q has no catalog label", na.alias, na.slug)
		}
	}
	for _, r := range namedRoleTable {
		if len(r.aliases) == 0 {
			t.Errorf("named role %q has no aliases", r.slug)
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

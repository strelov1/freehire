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
		// Emission order: seniority-only, bare category, composite, named.
		{"seniority + category + graded", "senior", "backend", "Senior Backend Engineer", []string{"senior", "backend", "senior_backend"}},
		{"middle frontend", "middle", "frontend", "Middle Frontend Developer", []string{"middle", "frontend", "middle_frontend"}},
		{"lead devops", "lead", "devops", "Lead DevOps Engineer", []string{"lead", "devops", "lead_devops"}},

		// Seniority-only role: a grade with no category and no named match still
		// filters by seniority (this is what replaces the standalone seniority facet).
		{"seniority only", "senior", "", "Senior Specialist", []string{"senior"}},

		// Bare category role with no seniority — the dominant real-world case.
		{"bare category, no seniority", "", "data_science", "Data Scientist", []string{"data_science"}},
		{"bare category product", "", "product", "Product Manager", []string{"product"}},

		// Category "other" yields no bare/composite role (no natural role noun); a
		// seniority still emits its seniority-only role.
		{"category other, no seniority", "", "other", "Coordinator", nil},
		{"category other with seniority", "lead", "other", "Lead Coordinator", []string{"lead"}},

		// Named roles come from the title regardless of the grid.
		{"software engineer catch-all", "", "", "Software Engineer", []string{"software_engineer"}},
		{"founding engineer, empty grid", "", "", "Founding Engineer", []string{"founding_engineer"}},
		{"cloud solutions engineer beats adjacency gap", "", "", "Cloud Solutions Engineer", []string{"cloud_solutions_engineer"}},
		{"technical lead adds seniority-only", "lead", "", "Technical Lead", []string{"lead", "technical_lead"}},
		{"fractional cto", "c_level", "", "Fractional CTO", []string{"c_level", "fractional_cto"}},
		// Length-ordered aliases: the longer, more specific phrase wins.
		{"technical account manager beats account manager", "", "sales", "Technical Account Manager", []string{"sales", "technical_account_manager"}},

		// Mined granular tech roles co-exist with their coarse bare category.
		{"android developer + mobile", "", "mobile", "Android Developer", []string{"mobile", "android_developer"}},
		{"senior ios engineer", "senior", "mobile", "Senior iOS Engineer", []string{"senior", "mobile", "senior_mobile", "ios_developer"}},
		{"platform engineer + devops", "", "devops", "Platform Engineer", []string{"devops", "platform_engineer"}},
		{"solutions architect", "", "architecture", "Solution Architect", []string{"architecture", "solutions_architect"}},

		// Seniority + bare + composite + one named coexist without duplicates.
		{"all four sources", "senior", "backend", "Senior Backend Founding Engineer", []string{"senior", "backend", "senior_backend", "founding_engineer"}},

		// Never guesses: no seniority, no category, no named alias.
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

// Every seniority MUST produce a seniority-only role present in the catalog, so
// the role facet subsumes the standalone seniority filter it replaces.
func TestSeniorityOnlyRoleForEveryGrade(t *testing.T) {
	cat := Catalog()
	for _, s := range enrich.SeniorityValues {
		if got := Derive(s, "", "Some Title"); !slices.Contains(got, s) {
			t.Errorf("Derive(%q,\"\") = %v, missing seniority-only role %q", s, got, s)
		}
		if _, ok := cat[s]; !ok {
			t.Errorf("seniority-only role %q missing from catalog", s)
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

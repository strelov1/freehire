package roletag

import (
	"slices"
	"testing"

	"github.com/strelov1/freehire/internal/vocab"
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
		// Generalist titles classify assigns no category to: the named role is the
		// only thing that makes them pickable.
		{"product engineer", "", "", "Product Engineer", []string{"product_engineer"}},
		{"member of technical staff, no grade", "", "", "Member of Technical Staff", []string{"member_of_technical_staff"}},
		// MTS grades, so SMTS is one graded role rather than "Senior" + the bare role.
		{"senior member of technical staff", "senior", "", "Senior Member of Technical Staff", []string{"senior", "member_of_technical_staff", "senior_member_of_technical_staff"}},
		// Longest-alias-first ordering: "ai agent engineer" wins over "agent engineer",
		// and "ai product engineer" over "product engineer" — both land on their own role.
		{"agent engineer", "", "ai_engineering", "Agent Engineer", []string{"ai_engineering", "agent_engineer"}},
		{"ai agent engineer", "", "ai_engineering", "AI Agent Engineer", []string{"ai_engineering", "agent_engineer"}},
		{"ai product engineer keeps product_engineer out", "", "ai_engineering", "AI Product Engineer", []string{"ai_engineering", "ai_product_engineer"}},
		// Hyphenated spelling: without its own alias the shorter "product engineer"
		// would win (a hyphen is a word boundary) and mislabel this as a plain
		// product_engineer. "AI-Agent Engineer" needs no entry — "agent engineer"
		// already resolves it to the right role.
		{"ai-product engineer", "", "ai_engineering", "AI-product engineer", []string{"ai_engineering", "ai_product_engineer"}},
		{"ai-agent engineer", "", "ai_engineering", "AI-Agent Engineer", []string{"ai_engineering", "agent_engineer"}},
		{"cloud solutions engineer beats adjacency gap", "", "", "Cloud Solutions Engineer", []string{"cloud_solutions_engineer"}},
		{"technical lead adds seniority-only", "lead", "", "Technical Lead", []string{"lead", "technical_lead"}},
		{"fractional cto", "c_level", "", "Fractional CTO", []string{"c_level", "fractional_cto"}},
		// Length-ordered aliases: the longer, more specific phrase wins.
		{"technical account manager beats account manager", "", "sales", "Technical Account Manager", []string{"sales", "technical_account_manager"}},

		// Mined granular tech roles co-exist with their coarse bare category.
		{"android developer + mobile", "", "mobile", "Android Developer", []string{"mobile", "android_developer"}},
		{"senior ios engineer", "senior", "mobile", "Senior iOS Engineer", []string{"senior", "mobile", "senior_mobile", "ios_developer", "senior_ios_developer"}},
		{"platform engineer + devops", "", "devops", "Platform Engineer", []string{"devops", "platform_engineer"}},
		{"solutions architect", "", "architecture", "Solution Architect", []string{"architecture", "solutions_architect"}},

		// Seniority + bare + composite + one named coexist without duplicates.
		{"all four sources", "senior", "backend", "Senior Backend Founding Engineer", []string{"senior", "backend", "senior_backend", "founding_engineer"}},

		// Gradeable named roles compose with seniority — "Senior Software Engineer"
		// is a single role, not just "Senior" + "Software Engineer".
		{"senior software engineer composes", "senior", "", "Senior Software Engineer", []string{"senior", "software_engineer", "senior_software_engineer"}},
		{"lead android developer composes", "lead", "mobile", "Lead Android Developer", []string{"lead", "mobile", "lead_mobile", "android_developer", "lead_android_developer"}},
		// Non-gradeable named roles do NOT compose (grade is meaningless / baked in).
		{"fractional cto does not compose", "c_level", "", "Fractional CTO", []string{"c_level", "fractional_cto"}},
		{"staff engineer does not compose", "staff", "", "Staff Engineer", []string{"staff", "staff_engineer"}},

		// Prod-gap additions.
		{"swe alias resolves software engineer", "senior", "", "Senior SWE", []string{"senior", "software_engineer", "senior_software_engineer"}},
		{"team lead named role", "lead", "", "Team Lead", []string{"lead", "team_lead"}},
		{"react native named + graded", "senior", "", "Senior React Native Developer", []string{"senior", "react_native_developer", "senior_react_native_developer"}},
		{"flutter named", "", "", "Flutter Developer", []string{"flutter_developer"}},
		{"director non-gradeable", "lead", "", "Director", []string{"lead", "director"}},

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
	for _, c := range vocab.CategoryValues {
		if c == "other" {
			continue
		}
		if got := Derive("", c, ""); !slices.Contains(got, c) {
			t.Errorf("Derive(\"\",%q) = %v, missing bare role %q", c, got, c)
		}
		if _, ok := cat[c]; !ok {
			t.Errorf("bare category role %q missing from catalog", c)
		}
		for _, s := range vocab.SeniorityValues {
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
	for _, s := range vocab.SeniorityValues {
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

// TestDerive_DesignRoles covers both sides of the design split. The engineering
// roles and design_engineer share the substring "design engineer", so this pins the
// longest-alias-first ordering that keeps a qualified title off the generic role.
func TestDerive_DesignRoles(t *testing.T) {
	cases := []struct {
		name      string
		seniority string
		category  string
		title     string
		want      []string
	}{
		// The new category decomposes like every other one: bare role + composite.
		{"engineering design bare", "", "engineering_design", "Design Engineer", []string{"engineering_design", "design_engineer"}},
		{"engineering design graded", "senior", "engineering_design", "Senior Design Engineer", []string{"senior", "engineering_design", "senior_engineering_design", "design_engineer", "senior_design_engineer"}},

		// A qualified engineering title takes its specific role, not the generic one:
		// "mechanical design engineer" is the longer alias, so it wins the ordering.
		{"mechanical designer beats design_engineer", "senior", "engineering_design", "Senior Mechanical Design Engineer",
			[]string{"senior", "engineering_design", "senior_engineering_design", "mechanical_designer", "senior_mechanical_designer"}},
		{"electrical designer", "", "engineering_design", "Electrical Designer", []string{"engineering_design", "electrical_designer"}},
		{"civil designer", "", "engineering_design", "Civil Designer", []string{"engineering_design", "civil_designer"}},
		// Silicon and board design carry the `hardware` category, not the draughting
		// one — the named role is what keeps the specific title pickable inside it.
		{"pcb designer", "", "hardware", "PCB Design Engineer", []string{"hardware", "pcb_designer"}},
		{"chip designer via physical design", "", "hardware", "Physical Design Engineer", []string{"hardware", "chip_designer"}},
		{"chip designer via asic", "", "hardware", "ASIC Design Engineer", []string{"hardware", "chip_designer"}},
		{"chip designer via chip design", "", "hardware", "Chip Design Engineer", []string{"hardware", "chip_designer"}},
		{"pcb layout designer", "", "hardware", "PCB Layout Designer", []string{"hardware", "pcb_designer"}},

		// Product-side specializations stop collapsing into the bare "Designer".
		{"visual designer graded", "senior", "design", "Senior Visual Designer",
			[]string{"senior", "design", "senior_design", "visual_designer", "senior_visual_designer"}},
		{"brand designer", "", "design", "Brand Designer", []string{"design", "brand_designer"}},
		{"motion designer", "", "design", "Motion Designer", []string{"design", "motion_designer"}},
		{"web designer", "", "design", "Web Designer", []string{"design", "web_designer"}},
		{"industrial designer", "", "design", "Industrial Designer", []string{"design", "industrial_designer"}},
		{"ux researcher", "", "design", "UX Researcher", []string{"design", "ux_researcher"}},
		{"user researcher alias", "", "design", "User Researcher", []string{"design", "ux_researcher"}},
		{"design ops", "", "design", "Design Ops Manager", []string{"design", "design_ops"}},
		// "design operations" (17 chars) outranks the pre-existing "head of design"
		// (14) in the length ordering, so this title reads as the ops role rather than
		// the head-of-function one. That is the better label for it, but it is a
		// behaviour change worth pinning.
		{"design operations beats head of design", "", "design", "Head of Design Operations",
			[]string{"design", "design_ops"}},

		// Directorial titles state their level already — they do not compose.
		{"art director does not compose", "lead", "design", "Art Director", []string{"lead", "design", "lead_design", "art_director"}},
		{"creative director does not compose", "c_level", "design", "Creative Director", []string{"c_level", "design", "c_level_design", "creative_director"}},

		// The alias spellings of each new role, so none ships unwitnessed.
		{"designops one word", "", "design", "DesignOps Lead", []string{"design", "design_ops"}},
		{"branding designer", "", "design", "Branding Designer", []string{"design", "brand_designer"}},
		{"motion graphics designer", "", "design", "Motion Graphics Designer", []string{"design", "motion_designer"}},
		{"user experience researcher", "", "design", "User Experience Researcher", []string{"design", "ux_researcher"}},
		// The draughting professions are roles of their own — the bare category noun is
		// all they would otherwise get.
		{"bim modeler", "", "engineering_design", "BIM Modeler", []string{"engineering_design", "bim_specialist"}},
		{"bim coordinator", "", "engineering_design", "Senior BIM Coordinator", []string{"engineering_design", "bim_specialist"}},
		{"draughtsman spelling", "", "engineering_design", "Draughtsman", []string{"engineering_design", "drafter"}},
		{"cad drafter", "", "engineering_design", "CAD Drafter", []string{"engineering_design", "drafter"}},

		// The existing design roles keep winning where they already did.
		{"product designer still wins", "senior", "design", "Senior Product Designer",
			[]string{"senior", "design", "senior_design", "product_designer", "senior_product_designer"}},
		{"ux designer still wins", "", "design", "UX Designer", []string{"design", "ux_designer"}},
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

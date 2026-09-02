package roletag

import (
	"slices"
	"testing"

	"github.com/strelov1/freehire/internal/dict/vocab"
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
		// classify's software_engineering category (added alongside this named
		// role) now ALSO resolves a bare "Software Engineer", so real input carries
		// both: the bare category role plus the named role, deliberately under
		// different labels ("Software Generalist" vs "Software Engineer" — see
		// categoryNoun) so the picker never shows two identical-looking options.
		{"software engineering category + named role coexist", "", "software_engineering", "Software Engineer", []string{"software_engineering", "software_engineer"}},
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

		// FDE alias coverage (refine-ai-role-classification): the field guide's own
		// matching rule is title contains "FDE" or "forward deploy", case-insensitive.
		{"bare fde", "", "ai_engineering", "FDE - Enterprise AI", []string{"ai_engineering", "forward_deployed_engineer"}},
		{"forward deploy engineer", "", "ai_engineering", "Forward Deploy Engineer", []string{"ai_engineering", "forward_deployed_engineer"}},
		{"forward-deployed engineer hyphenated", "", "ai_engineering", "Forward-Deployed Engineer", []string{"ai_engineering", "forward_deployed_engineer"}},
		{"forward deployed engineer unchanged", "", "ai_engineering", "Forward Deployed Engineer", []string{"ai_engineering", "forward_deployed_engineer"}},
		// Synonym titles get their own slug, not merged into FDE — title fidelity.
		{"applied ai engineer own slug", "", "ai_engineering", "Applied AI Engineer", []string{"ai_engineering", "applied_ai_engineer"}},
		{"deployment engineer own slug", "", "", "Deployment Engineer", []string{"deployment_engineer"}},
		// Gradeable, like every other single-phrase named role in this table.
		{"senior forward deployed engineer composes", "senior", "ai_engineering", "Senior Forward Deployed Engineer", []string{"senior", "ai_engineering", "senior_ai_engineering", "forward_deployed_engineer", "senior_forward_deployed_engineer"}},
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
		{"chip designer hyphenated mixed signal", "", "hardware", "Mixed-Signal Design Engineer", []string{"hardware", "chip_designer"}},
		{"chip designer via rf", "", "hardware", "RF Design Engineer", []string{"hardware", "chip_designer"}},
		{"chip designer via standard cell", "", "hardware", "Standard Cell Design Engineer", []string{"hardware", "chip_designer"}},
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

// TestDerive_SEOCluster covers the search-optimization disciplines the coarse
// `seo_specialist` role flattened. The longest-alias-first ordering is what keeps
// "Technical SEO Specialist" off the shorter "seo specialist" alias, so each
// qualified form needs its own entry rather than a prefix rule.
func TestDerive_SEOCluster(t *testing.T) {
	cases := []struct {
		name                       string
		seniority, category, title string
		want                       []string
	}{
		{"technical seo beats bare seo", "", "marketing", "Technical SEO Specialist", []string{"marketing", "technical_seo_specialist"}},
		{"technical seo engineer is the same role", "", "marketing", "Technical SEO Engineer", []string{"marketing", "technical_seo_specialist"}},
		{"content seo", "", "marketing", "Content SEO Specialist", []string{"marketing", "content_seo_specialist"}},
		{"link building", "", "marketing", "Link Building Specialist", []string{"marketing", "link_building_specialist"}},
		{"outreach specialist is link building", "", "marketing", "SEO Outreach Specialist", []string{"marketing", "link_building_specialist"}},
		{"seo analyst", "", "marketing", "SEO Analyst", []string{"marketing", "seo_analyst"}},
		{"graded technical seo composes", "senior", "marketing", "Senior Technical SEO Specialist", []string{"senior", "marketing", "senior_marketing", "technical_seo_specialist", "senior_technical_seo_specialist"}},

		// the coarse role still resolves for the unqualified titles
		{"bare seo specialist unchanged", "", "marketing", "SEO Specialist", []string{"marketing", "seo_specialist"}},
		{"seo manager unchanged", "", "marketing", "SEO Manager", []string{"marketing", "seo_specialist"}},
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

// TestDerive_GEOCluster covers generative-engine optimization. The industry names
// one job three ways — GEO, AEO and GSO — so they collapse to a single slug rather
// than fragmenting the facet three ways. The abbreviation is the whole risk here:
// "geo" is geography everywhere else in this codebase, so only the spelled-out
// forms and the bound "geo specialist"/"geo manager" resolve.
func TestDerive_GEOCluster(t *testing.T) {
	cases := []struct {
		name                       string
		seniority, category, title string
		want                       []string
	}{
		{"spelled out", "", "marketing", "Generative Engine Optimization Specialist", []string{"marketing", "geo_specialist"}},
		{"answer engine variant", "", "marketing", "Answer Engine Optimization Manager", []string{"marketing", "geo_specialist"}},
		{"generative search variant", "", "marketing", "Generative Search Optimization Lead", []string{"marketing", "geo_specialist"}},
		{"bound abbreviation", "", "marketing", "GEO Specialist", []string{"marketing", "geo_specialist"}},
		{"aeo abbreviation", "", "marketing", "AEO Manager", []string{"marketing", "geo_specialist"}},

		// the bare token is geography — these must stay untouched
		{"geospatial analyst untouched", "", "data_analytics", "Geo Data Analyst", []string{"data_analytics"}},
		{"geospatial engineer untouched", "", "", "Geospatial Engineer", nil},
		{"geo targeting manager untouched", "", "marketing", "Geo Targeting Manager", []string{"marketing"}},
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

// TestDerive_SMMCluster covers the social-media disciplines. The manager is a
// generalist bundle; community management, paid social and content creation are
// the function specialists a manager coordinates, so they get their own slugs
// rather than folding into the manager role.
func TestDerive_SMMCluster(t *testing.T) {
	cases := []struct {
		name                       string
		seniority, category, title string
		want                       []string
	}{
		{"paid social", "", "marketing", "Paid Social Specialist", []string{"marketing", "paid_social_specialist"}},
		{"paid social manager", "", "marketing", "Paid Social Manager", []string{"marketing", "paid_social_specialist"}},
		{"content creator", "", "marketing", "Content Creator", []string{"marketing", "content_creator"}},
		{"ugc creator", "", "marketing", "UGC Creator", []string{"marketing", "content_creator"}},
		{"smm abbreviation resolves the manager", "", "marketing", "SMM Manager", []string{"marketing", "social_media_manager"}},

		// already-resolving roles must not shift
		{"social media manager unchanged", "", "marketing", "Social Media Manager", []string{"marketing", "social_media_manager"}},
		{"community manager unchanged", "", "marketing", "Community Manager", []string{"marketing", "community_manager"}},
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

// TestDerive_CommercialMarketingCluster covers the funnel-owning marketing
// functions plus GTM engineering. CRM marketing folds into lifecycle rather than
// getting a slug of its own: an unqualified "CRM Manager" is as often a Salesforce
// administrator as a lifecycle marketer, so only the qualified phrases resolve.
func TestDerive_CommercialMarketingCluster(t *testing.T) {
	cases := []struct {
		name                       string
		seniority, category, title string
		want                       []string
	}{
		{"gtm engineer", "", "sales", "GTM Engineer", []string{"sales", "gtm_engineer"}},
		{"go to market engineer", "", "sales", "Go-To-Market Engineer", []string{"sales", "gtm_engineer"}},
		{"demand generation", "", "marketing", "Demand Generation Manager", []string{"marketing", "demand_generation_manager"}},
		{"lifecycle marketing", "", "marketing", "Lifecycle Marketing Manager", []string{"marketing", "lifecycle_marketing_manager"}},
		{"crm marketing folds into lifecycle", "", "marketing", "CRM Marketing Manager", []string{"marketing", "lifecycle_marketing_manager"}},
		{"retention marketing folds into lifecycle", "", "marketing", "Retention Marketing Lead", []string{"marketing", "lifecycle_marketing_manager"}},
		{"performance marketing", "", "marketing", "Performance Marketing Manager", []string{"marketing", "performance_marketer"}},
		{"marketing operations", "", "marketing", "Marketing Operations Manager", []string{"marketing", "marketing_operations_manager"}},
		{"brand manager", "", "marketing", "Brand Manager", []string{"marketing", "brand_manager"}},
		{"pr manager", "", "marketing", "PR Manager", []string{"marketing", "pr_manager"}},
		{"influencer marketing", "", "marketing", "Influencer Marketing Manager", []string{"marketing", "influencer_marketing_manager"}},
		{"copywriter", "", "marketing", "Copywriter", []string{"marketing", "copywriter"}},
		{"marketing analyst", "", "marketing", "Marketing Analyst", []string{"marketing", "marketing_analyst"}},

		// an unqualified CRM title is not claimed
		{"bare crm manager unclaimed", "", "marketing", "CRM Manager", []string{"marketing"}},
		// the existing growth and product marketing roles must not shift
		{"growth marketer unchanged", "", "marketing", "Growth Marketing Manager", []string{"marketing", "growth_marketer"}},
		{"pmm unchanged", "", "marketing", "Product Marketing Manager", []string{"marketing", "product_marketing_manager"}},
		{"growth engineer stays technical", "", "", "Growth Engineer", []string{"growth_engineer"}},
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

// The spelled-out UX/UI titles. Unlike the "<discipline> developer" phrases added to
// classify — which were redundant for tagging and needed only by search — these were a
// real gap in the tagging itself: "User Experience Designer" derived the bare `design`
// category and no ux_designer, so the role under-counted every posting titled that way.
// Adding them therefore changes what is TAGGED — and every existing posting picks the
// role up on the next scheduled reindex, because roles are derived at index time rather
// than stored, so nothing has to be backfilled.
func TestUXDesignerSpelledOutAliases(t *testing.T) {
	for _, title := range []string{
		"User Experience Designer",
		"User Interface Designer",
		"Senior UX/UI Designer",
	} {
		if got := Derive("", "design", title); !slices.Contains(got, "ux_designer") {
			t.Errorf("Derive(_, design, %q) = %v, want it to contain ux_designer", title, got)
		}
	}
	aliases := NamedAliases()["ux_designer"]
	for _, want := range []string{"user experience designer", "user interface designer", "ux/ui designer"} {
		if !slices.Contains(aliases, want) {
			t.Errorf("NamedAliases()[ux_designer] missing %q", want)
		}
	}
}

// TestDeriveMediaCrafts covers the roles the `creative` category decomposes into.
// The bare category role says "some media craft"; these say which one.
func TestDeriveMediaCrafts(t *testing.T) {
	for _, tc := range []struct {
		title string
		want  string
	}{
		{"Video Editor", "video_editor"},
		{"Senior Video Editor", "video_editor"},
		{"Videographer", "videographer"},
		{"Video Producer", "video_producer"},
		{"Animator", "animator"},
		{"3D Artist", "3d_artist"},
		// Seats on the same 3D pipeline, deliberately folded into one slug.
		{"Character Artist", "3d_artist"},
		{"Environment Artist", "3d_artist"},
		// Crafts of their own, deliberately NOT folded.
		{"2D Artist", "2d_artist"},
		{"VFX Artist", "vfx_artist"},
		{"Concept Artist", "concept_artist"},
		{"Technical Artist", "technical_artist"},
		{"Storyboard Artist", "storyboard_artist"},
		{"Illustrator", "illustrator"},
		{"Photographer", "photographer"},
		{"Photo Editor", "photographer"},
		{"Sound Designer", "sound_designer"},
		{"Sound Design Engineer", "sound_designer"},
		{"Audio Design Engineer", "sound_designer"},
	} {
		if got := Derive("", "creative", tc.title); !slices.Contains(got, tc.want) {
			t.Errorf("Derive(_, creative, %q) = %v, want it to contain %q", tc.title, got, tc.want)
		}
	}
}

// TestDeriveGameRoles pins the decision NOT to add a game category. The titles keep
// whatever category they resolve to today — `design` for the designers,
// `software_engineering` for the developer, none for the producer — and the named
// role is what makes the craft pickable.
func TestDeriveGameRoles(t *testing.T) {
	for _, tc := range []struct {
		title, category, want string
	}{
		{"Game Designer", "design", "game_designer"},
		{"Senior Game Designer", "design", "game_designer"},
		{"Level Designer", "design", "level_designer"},
		{"Narrative Designer", "design", "narrative_designer"},
		{"Game Developer", "software_engineering", "game_developer"},
		// No category resolves for this one; a named role is emitted anyway.
		{"Game Producer", "", "game_producer"},
	} {
		if got := Derive("", tc.category, tc.title); !slices.Contains(got, tc.want) {
			t.Errorf("Derive(_, %q, %q) = %v, want it to contain %q", tc.category, tc.title, got, tc.want)
		}
	}
}

// TestMediaRolesDoNotStealFromDesign guards the collisions the new aliases create:
// every one of these titles contains a shorter new alias and must keep the more
// specific role it already has.
func TestMediaRolesDoNotStealFromDesign(t *testing.T) {
	for _, tc := range []struct {
		title, category, want string
	}{
		{"Motion Designer", "design", "motion_designer"},
		{"Motion Graphics Designer", "design", "motion_designer"},
		{"Graphic Designer", "design", "graphic_designer"},
		{"Visual Designer", "design", "visual_designer"},
		{"Product Designer", "design", "product_designer"},
		{"Industrial Designer", "design", "industrial_designer"},
		// The second-hat titles: a craft alias must not take a role the design alias
		// already owns, on either side of the length ordering.
		{"Graphic Designer & Photographer", "design", "graphic_designer"},
		{"Junior Motion Designer / Animator", "design", "motion_designer"},
		{"Social Media Manager (Canva, Illustrator)", "marketing", "social_media_manager"},
	} {
		if got := Derive("", tc.category, tc.title); !slices.Contains(got, tc.want) {
			t.Errorf("Derive(_, %q, %q) = %v, want it to contain %q", tc.category, tc.title, got, tc.want)
		}
	}
}

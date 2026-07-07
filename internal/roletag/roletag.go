// Package roletag derives a job's natural roles deterministically from its
// already-resolved seniority and category and its title. It is a curated
// dictionary, not a model — the same doctrine as internal/classify and
// internal/skilltag: it emits canonical role slugs for what it can resolve and
// nothing for what it cannot (it never guesses).
//
// A job's roles are, in order:
//   - the bare category role ({category}, e.g. "backend") whenever the category
//     resolves — the dominant real-world case, since most titles carry no grade;
//   - the composite {seniority}_{category} (e.g. "senior_backend") when the
//     seniority also resolves — the graded role on top of the bare one;
//   - at most one named role matched from the title, for roles that do not
//     decompose into the seniority×category grid (founding_engineer,
//     fractional_cto, software_engineer, …).
//
// The package also owns the catalog (slug → human label), the source of truth
// for the picker labels emitted into the web contracts.
package roletag

import (
	"sort"
	"strings"

	"github.com/strelov1/freehire/internal/wordmatch"
)

// seniorityLabel maps each enrich.SeniorityValues canonical to its display word.
var seniorityLabel = map[string]string{
	"intern":    "Intern",
	"junior":    "Junior",
	"middle":    "Middle",
	"senior":    "Senior",
	"lead":      "Lead",
	"staff":     "Staff",
	"principal": "Principal",
	"c_level":   "C-Level",
}

// categoryNoun maps each enrich.CategoryValues canonical (except "other", which
// yields no useful natural role) to its role noun. It is the decomposable-category
// set: the bare role's label and the base of every composite label
// ("{seniorityLabel} {categoryNoun}", e.g. senior + backend → "Senior Backend
// Engineer").
var categoryNoun = map[string]string{
	"backend":             "Backend Engineer",
	"frontend":            "Frontend Engineer",
	"fullstack":           "Fullstack Engineer",
	"mobile":              "Mobile Engineer",
	"devops":              "DevOps Engineer",
	"sre":                 "Site Reliability Engineer",
	"network_engineering": "Network Engineer",
	"data_engineering":    "Data Engineer",
	"data_science":        "Data Scientist",
	"data_analytics":      "Data Analyst",
	"ml_ai":               "ML Engineer",
	"ai_engineering":      "AI Engineer",
	"qa":                  "QA Engineer",
	"security":            "Security Engineer",
	"hardware":            "Hardware Engineer",
	"embedded":            "Embedded Engineer",
	"blockchain":          "Blockchain Engineer",
	"architecture":        "Architect",
	"design":              "Designer",
	"product":             "Product Manager",
	"project_management":  "Project Manager",
	"management":          "Manager",
	"marketing":           "Marketing Specialist",
	"sales":               "Sales Specialist",
	"support":             "Support Specialist",
}

// namedRoleTable is the curated set of roles that do not decompose into the
// seniority×category grid. Each carries its canonical slug, display label, and
// the title aliases that resolve to it (matched whole-word). One entry per role
// — the ordered alias list and the label map are built from this single table,
// so there is nothing to keep in sync. Aliases are lowercase.
var namedRoleTable = []struct {
	slug, label string
	aliases     []string
}{
	// Generic engineering catch-all (classify assigns no category to a bare
	// "Software Engineer"): the largest category-less bucket in the catalogue.
	{"software_engineer", "Software Engineer", []string{"software engineer", "software developer", "software development engineer", "web developer", "sde"}},

	// Startup / cross-cutting engineering.
	{"founding_engineer", "Founding Engineer", []string{"founding engineer"}},
	{"founding_designer", "Founding Designer", []string{"founding designer"}},
	{"founding_pm", "Founding Product Manager", []string{"founding product manager", "founding pm"}},
	{"staff_engineer", "Staff Engineer", []string{"staff engineer"}},
	{"technical_lead", "Technical Lead", []string{"technical lead", "tech lead"}},
	{"forward_deployed_engineer", "Forward Deployed Engineer", []string{"forward deployed engineer"}},
	{"growth_engineer", "Growth Engineer", []string{"growth engineer"}},
	{"developer_advocate", "Developer Advocate", []string{"developer advocate", "developer relations", "developer evangelist", "devrel"}},
	{"research_engineer", "Research Engineer", []string{"research engineer"}},
	{"analytics_engineer", "Analytics Engineer", []string{"analytics engineer"}},
	{"mlops_engineer", "MLOps Engineer", []string{"mlops engineer", "ml ops engineer"}},
	{"prompt_engineer", "Prompt Engineer", []string{"prompt engineer"}},
	{"business_analyst", "Business Analyst", []string{"business analyst"}},
	{"systems_administrator", "Systems Administrator", []string{"systems administrator"}},

	// Granular tech specializations (mined from prod titles — they collapse into a
	// coarse category like mobile/devops/architecture, so a named role keeps the
	// specific title pickable).
	{"android_developer", "Android Developer", []string{"android developer", "android engineer", "android software engineer"}},
	{"ios_developer", "iOS Developer", []string{"ios developer", "ios engineer", "ios software engineer"}},
	{"platform_engineer", "Platform Engineer", []string{"platform engineer"}},
	{"cloud_engineer", "Cloud Engineer", []string{"cloud engineer"}},
	{"infrastructure_engineer", "Infrastructure Engineer", []string{"infrastructure engineer"}},
	{"firmware_engineer", "Firmware Engineer", []string{"firmware engineer"}},
	{"fpga_engineer", "FPGA Engineer", []string{"fpga engineer"}},
	{"qa_automation_engineer", "QA Automation Engineer", []string{"qa automation engineer", "test automation engineer", "automation qa engineer", "sdet"}},
	{"data_platform_engineer", "Data Platform Engineer", []string{"data platform engineer"}},
	{"deep_learning_engineer", "Deep Learning Engineer", []string{"deep learning engineer"}},
	{"genai_engineer", "GenAI Engineer", []string{"genai engineer", "generative ai engineer"}},

	// Architects (named, distinct from the bare "architecture" role).
	{"solutions_architect", "Solutions Architect", []string{"solutions architect", "solution architect"}},
	{"software_architect", "Software Architect", []string{"software architect"}},
	{"enterprise_architect", "Enterprise Architect", []string{"enterprise architect"}},
	{"cloud_architect", "Cloud Architect", []string{"cloud architect"}},
	{"data_architect", "Data Architect", []string{"data architect"}},

	// Security specializations.
	{"security_officer", "Security Officer", []string{"security officer"}},
	{"cybersecurity_engineer", "Cybersecurity Engineer", []string{"cybersecurity engineer", "cyber security engineer"}},
	{"information_security_engineer", "Information Security Engineer", []string{"information security engineer"}},

	// Design specializations.
	{"product_designer", "Product Designer", []string{"product designer"}},
	{"ux_designer", "UX Designer", []string{"ux designer", "ui designer", "ui/ux designer"}},
	{"graphic_designer", "Graphic Designer", []string{"graphic designer"}},
	{"interior_designer", "Interior Designer", []string{"interior designer"}},

	// Non-software professions the catalogue carries (broad scope).
	{"electrical_engineer", "Electrical Engineer", []string{"electrical engineer"}},
	{"mechanical_engineer", "Mechanical Engineer", []string{"mechanical engineer"}},
	{"accountant", "Accountant", []string{"accountant"}},
	{"financial_analyst", "Financial Analyst", []string{"financial analyst"}},
	{"tax_manager", "Tax Manager", []string{"tax manager"}},
	{"program_manager", "Program Manager", []string{"program manager"}},

	// Customer-facing / pre-sales engineering.
	{"cloud_solutions_engineer", "Cloud Solutions Engineer", []string{"cloud solutions engineer"}},
	{"solutions_engineer", "Solutions Engineer", []string{"solutions engineer"}},
	{"sales_engineer", "Sales Engineer", []string{"sales engineer"}},
	{"customer_engineer", "Customer Engineer", []string{"customer engineer"}},
	{"implementation_engineer", "Implementation Engineer", []string{"implementation engineer"}},

	// Product & program.
	{"technical_program_manager", "Technical Program Manager", []string{"technical program manager", "tpm"}},
	{"product_operations_manager", "Product Operations Manager", []string{"product operations manager"}},

	// Marketing (granular names the coarse "marketing" category flattens).
	{"product_marketing_manager", "Product Marketing Manager", []string{"product marketing manager", "pmm"}},
	{"growth_marketer", "Growth Marketer", []string{"growth marketer", "growth marketing manager"}},
	{"seo_specialist", "SEO Specialist", []string{"seo specialist", "seo manager"}},
	{"content_strategist", "Content Strategist", []string{"content strategist", "content marketer"}},
	{"community_manager", "Community Manager", []string{"community manager"}},
	{"social_media_manager", "Social Media Manager", []string{"social media manager"}},

	// Sales & customer success.
	{"sdr", "Sales Development Representative", []string{"sales development representative", "sdr"}},
	{"bdr", "Business Development Representative", []string{"business development representative", "bdr"}},
	{"account_executive", "Account Executive", []string{"account executive"}},
	{"account_manager", "Account Manager", []string{"account manager"}},
	{"customer_success_manager", "Customer Success Manager", []string{"customer success manager", "csm"}},
	{"technical_account_manager", "Technical Account Manager", []string{"technical account manager", "tam"}},
	{"partnerships_manager", "Partnerships Manager", []string{"partnerships manager", "partnership manager"}},
	{"revenue_operations", "Revenue Operations", []string{"revenue operations", "revops"}},

	// People.
	{"technical_recruiter", "Technical Recruiter", []string{"technical recruiter", "tech recruiter"}},

	// Leadership / fractional / C-level.
	{"fractional_cto", "Fractional CTO", []string{"fractional cto"}},
	{"fractional_cfo", "Fractional CFO", []string{"fractional cfo"}},
	{"fractional_cmo", "Fractional CMO", []string{"fractional cmo"}},
	{"fractional_coo", "Fractional COO", []string{"fractional coo"}},
	{"fractional_cpo", "Fractional CPO", []string{"fractional cpo"}},
	{"founder", "Founder", []string{"founder", "co-founder", "cofounder", "technical co-founder"}},
	{"vp_engineering", "VP of Engineering", []string{"vp of engineering", "vp engineering"}},
	{"head_of_product", "Head of Product", []string{"head of product"}},
	{"head_of_growth", "Head of Growth", []string{"head of growth"}},
	{"head_of_design", "Head of Design", []string{"head of design"}},
	{"head_of_marketing", "Head of Marketing", []string{"head of marketing"}},
	{"chief_of_staff", "Chief of Staff", []string{"chief of staff"}},
	{"engineering_manager", "Engineering Manager", []string{"engineering manager"}},
}

// namedAlias pairs a title alias with its canonical slug.
type namedAlias struct{ alias, slug string }

// namedAliases is every alias→slug pair, ordered longest-alias-first so a
// containing phrase wins over a shorter one it contains ("technical account
// manager" over "account manager"); non-overlapping aliases sort by length with
// no behavioural effect. Built once from namedRoleTable.
var namedAliases = buildNamedAliases()

// namedLabel maps each named-role slug to its display label. Built from namedRoleTable.
var namedLabel = buildNamedLabels()

func buildNamedAliases() []namedAlias {
	var out []namedAlias
	for _, r := range namedRoleTable {
		for _, a := range r.aliases {
			out = append(out, namedAlias{a, r.slug})
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return len(out[i].alias) > len(out[j].alias) })
	return out
}

func buildNamedLabels() map[string]string {
	m := make(map[string]string, len(namedRoleTable))
	for _, r := range namedRoleTable {
		m[r.slug] = r.label
	}
	return m
}

// Derive returns a job's canonical role slugs from its resolved seniority,
// resolved category, and title: the bare category role when the category
// resolves, the composite {seniority}_{category} when the seniority also
// resolves, and at most one named role matched whole-word in the title. The
// three sources occupy distinct slug namespaces, so the result carries no
// duplicates. Every slug exists in Catalog; an unresolved input contributes
// nothing.
func Derive(seniority, category, title string) []string {
	var roles []string

	// Seniority-only role: the grade as its own facet value, so "any senior across
	// functions" (and a graded but category-less title) stays filterable through
	// the role picker — the role facet subsumes the standalone seniority filter.
	if _, ok := seniorityLabel[seniority]; ok {
		roles = append(roles, seniority)
	}

	// categoryNoun membership is the decomposable-category set (excludes "other",
	// where "{Seniority} Other" would be meaningless).
	if _, ok := categoryNoun[category]; ok {
		roles = append(roles, category)
		if seniority != "" {
			roles = append(roles, seniority+"_"+category)
		}
	}

	lower := strings.ToLower(title)
	for _, na := range namedAliases {
		if wordmatch.Contains(lower, na.alias, wordmatch.UnicodeBoundary) {
			roles = append(roles, na.slug)
			break
		}
	}

	return roles
}

// Catalog returns the full role catalog — every derivable slug mapped to its
// human label: the bare category roles, the seniority × category composites, and
// the curated named roles. It is the source of truth for picker labels.
func Catalog() map[string]string {
	cat := make(map[string]string, len(categoryNoun)*(len(seniorityLabel)+1)+len(seniorityLabel)+len(namedLabel))
	for sen, senLabel := range seniorityLabel {
		cat[sen] = senLabel // seniority-only role
	}
	for c, noun := range categoryNoun {
		cat[c] = noun
		for sen, senLabel := range seniorityLabel {
			cat[sen+"_"+c] = senLabel + " " + noun
		}
	}
	for slug, label := range namedLabel {
		cat[slug] = label
	}
	return cat
}

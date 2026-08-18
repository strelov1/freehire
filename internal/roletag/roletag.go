// Package roletag derives a job's natural roles deterministically from its
// already-resolved seniority and category and its title. It is a curated
// dictionary, not a model — the same doctrine as internal/classify and
// internal/skilltag: it emits canonical role slugs for what it can resolve and
// nothing for what it cannot (it never guesses).
//
// A job's roles are:
//   - the seniority-only role ({seniority}, e.g. "senior") whenever the grade
//     resolves — so "any senior across functions" stays filterable;
//   - the bare category role ({category}, e.g. "backend") whenever the category
//     resolves — the dominant real-world case, since most titles carry no grade;
//   - the composite {seniority}_{category} (e.g. "senior_backend") when both
//     resolve — the graded role on top of the bare one;
//   - at most one named role from the title, for roles that do not decompose into
//     the seniority×category grid (founding_engineer, software_engineer, …), plus
//     its graded composite {seniority}_{named} unless the role is nonGradeable.
//
// The package also owns the catalog (slug → human label), the source of truth
// for the picker labels emitted into the web contracts.
package roletag

import (
	"sort"
	"strings"

	"github.com/strelov1/freehire/internal/wordmatch"
)

// seniorityLabel maps each vocab.SeniorityValues canonical to its display word.
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

// categoryNoun maps each vocab.CategoryValues canonical (except "other", which
// yields no useful natural role) to its role noun. It is the decomposable-category
// set: the bare role's label and the base of every composite label
// ("{seniorityLabel} {categoryNoun}", e.g. senior + backend → "Senior Backend
// Engineer").
var categoryNoun = map[string]string{
	// Deliberately NOT "Software Engineer": the namedRoleTable below already owns
	// that exact label for its own "software_engineer" slug (title-matched,
	// independent of category, and depended on by web/src/lib/roleRelated.ts and
	// collections.ts) — an identical label on a second slug would show as a
	// confusing duplicate in the role picker.
	"software_engineering": "Software Generalist",
	"backend":              "Backend Engineer",
	"frontend":             "Frontend Engineer",
	"fullstack":            "Fullstack Engineer",
	"mobile":               "Mobile Engineer",
	"devops":               "DevOps Engineer",
	"sre":                  "Site Reliability Engineer",
	"network_engineering":  "Network Engineer",
	"data_engineering":     "Data Engineer",
	"data_science":         "Data Scientist",
	"data_analytics":       "Data Analyst",
	"ml_ai":                "ML Engineer",
	"ai_engineering":       "AI Engineer",
	"qa":                   "QA Engineer",
	"security":             "Security Engineer",
	"hardware":             "Hardware Engineer",
	"embedded":             "Embedded Engineer",
	"blockchain":           "Blockchain Engineer",
	"architecture":         "Architect",
	"design":               "Designer",
	"engineering_design":   "Engineering Designer",
	"product":              "Product Manager",
	"project_management":   "Project Manager",
	"management":           "Manager",
	"marketing":            "Marketing Specialist",
	"sales":                "Sales Specialist",
	"support":              "Support Specialist",
	// IT-company roles added by expand-role-taxonomy.
	"business_analysis":     "Business Analyst",
	"solutions_engineering": "Solutions Engineer",
	"developer_relations":   "Developer Advocate",
	"technical_writing":     "Technical Writer",
	"recruiting":            "Recruiter",
	"hr":                    "HR Manager",
	"finance":               "Finance Manager",
	"legal":                 "Legal Counsel",
	"operations":            "Operations Manager",
	"customer_success":      "Customer Success Manager",
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
	// Generic engineering catch-all. classify's software_engineering category
	// (added alongside this role) now ALSO resolves a bare "Software Engineer" —
	// the two coexist deliberately, the same layering every granular named role
	// below uses over its coarse category (android_developer + mobile,
	// systems_administrator + devops, …); categoryNoun gives the category a
	// distinct label ("Software Generalist") specifically so the two do not show
	// as duplicate-looking picker options. This role's own label/aliases/slug stay
	// unchanged since web/src/lib/roleRelated.ts and collections.ts depend on the
	// exact "software_engineer" slug.
	{"software_engineer", "Software Engineer", []string{"software engineer", "software developer", "software development engineer", "web developer", "sde", "swe"}},

	// Startup / cross-cutting engineering.
	{"founding_engineer", "Founding Engineer", []string{"founding engineer"}},
	// Generalist titles classify leaves category-less: the named role is the only
	// thing that makes them pickable. MTS grades (SMTS/Principal MTS are real
	// rungs), so it is left out of nonGradeable below.
	{"product_engineer", "Product Engineer", []string{"product engineer"}},
	{"member_of_technical_staff", "Member of Technical Staff", []string{"member of technical staff", "member of the technical staff", "mts"}},
	// Agent work is its own craft inside ai_engineering; "ai product engineer" is
	// listed here so the longest-alias-first ordering keeps it off product_engineer.
	{"agent_engineer", "Agent Engineer", []string{"agent engineer", "ai agent engineer"}},
	// The hyphenated spelling needs its own alias: a hyphen is a word boundary, so
	// without it the shorter "product engineer" wins the length ordering and the role
	// lands on product_engineer. "ai-agent engineer" needs no such entry — the bare
	// "agent engineer" already resolves it.
	{"ai_product_engineer", "AI Product Engineer", []string{"ai product engineer", "ai-product engineer"}},
	{"founding_designer", "Founding Designer", []string{"founding designer"}},
	{"founding_pm", "Founding Product Manager", []string{"founding product manager", "founding pm"}},
	{"staff_engineer", "Staff Engineer", []string{"staff engineer"}},
	{"technical_lead", "Technical Lead", []string{"technical lead", "tech lead"}},
	// Alias set matches the field guide's own FDE methodology: title contains "FDE"
	// or "forward deploy", case-insensitive. "forward-deployed" is its own alias
	// because a hyphen is a word boundary here (same trap documented elsewhere in
	// this table) — "forward deploy" alone would not match the hyphenated spelling.
	{"forward_deployed_engineer", "Forward Deployed Engineer", []string{"forward deployed engineer", "forward deploy", "forward-deployed", "fde"}},
	// Field-guide-documented synonym titles for the same class of work, kept as
	// their own slugs rather than merged into forward_deployed_engineer — a job's
	// derived role should reflect the title it was actually posted under.
	{"applied_ai_engineer", "Applied AI Engineer", []string{"applied ai engineer"}},
	{"deployment_engineer", "Deployment Engineer", []string{"deployment engineer"}},
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
	{"react_native_developer", "React Native Developer", []string{"react native", "react-native"}},
	{"flutter_developer", "Flutter Developer", []string{"flutter"}},
	{"solutions_consultant", "Solutions Consultant", []string{"solutions consultant", "solution consultant"}},
	{"scrum_master", "Scrum Master", []string{"scrum master"}},

	// Explicit gaps found in prod: "Team Lead" (classify only gives grade lead),
	// "Director" (frequent, no category) — kept non-gradeable below.
	{"team_lead", "Team Lead", []string{"team lead", "teamlead"}},
	{"director", "Director", []string{"director"}},
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
	{"visual_designer", "Visual Designer", []string{"visual designer"}},
	{"brand_designer", "Brand Designer", []string{"brand designer", "branding designer"}},
	{"motion_designer", "Motion Designer", []string{"motion designer", "motion graphics designer"}},
	{"web_designer", "Web Designer", []string{"web designer"}},
	{"industrial_designer", "Industrial Designer", []string{"industrial designer"}},
	{"ux_researcher", "UX Researcher", []string{"ux researcher", "user researcher", "ux research", "user experience researcher"}},
	{"art_director", "Art Director", []string{"art director"}},
	{"creative_director", "Creative Director", []string{"creative director"}},
	{"design_ops", "Design Ops", []string{"design ops", "designops", "design operations"}},
	// design_engineer is the UNQUALIFIED "Design Engineer" — the product/engineering
	// hybrid and the titles that state no discipline. Every qualified engineering form
	// below carries a longer alias, and Derive matches longest-first, so a
	// "Mechanical Design Engineer" never comes back as a generic design engineer.
	{"design_engineer", "Design Engineer", []string{"design engineer", "product design engineer", "design systems engineer", "design system engineer"}},

	// Engineering-design specializations (the engineering_design category).
	{"mechanical_designer", "Mechanical Designer", []string{"mechanical design engineer", "mechanical designer"}},
	{"electrical_designer", "Electrical Designer", []string{"electrical design engineer", "electrical designer"}},
	{"civil_designer", "Civil Designer", []string{"civil design engineer", "civil designer", "structural designer"}},
	{"drafter", "Drafter", []string{"drafter", "draftsman", "draughtsman", "design drafter", "design draftsman", "cad drafter"}},
	{"bim_specialist", "BIM Specialist", []string{"bim modeler", "bim coordinator", "bim designer", "bim specialist", "revit designer"}},
	{"pcb_designer", "PCB Designer", []string{"pcb design engineer", "pcb designer", "pcb layout engineer", "pcb layout designer"}},
	// The silicon family has many spellings and they all name one role. Whatever is
	// missing here falls back to the generic design_engineer, which is why the list
	// mirrors the categoryTable block that routes these titles to `hardware`.
	{"chip_designer", "Chip Designer", []string{
		"physical design engineer", "vlsi design engineer", "rtl design engineer",
		"analog design engineer", "mixed signal design engineer", "digital design engineer",
		"chip design engineer", "asic design engineer", "soc design engineer",
		"ic design engineer", "semiconductor design engineer", "dft design engineer",
		// A hyphen is a word boundary, so the industry's own "mixed-signal" spelling
		// needs its own alias — the same trap the classify table documents.
		"mixed-signal design engineer", "rf design engineer", "rfic design engineer",
		"analogue design engineer", "silicon design engineer", "memory design engineer",
		"standard cell design engineer",
	}},

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
	// Search optimization split into its four working disciplines. The ordering is by
	// alias LENGTH, not specificity, so the qualified spellings that contain a
	// shorter rival alias need their own entry: "technical seo" (13) loses to "seo
	// specialist" (14) and would hand "Technical SEO Specialist" to the coarse role.
	// The unqualified spellings ("Technical SEO Engineer") ride the short alias.
	{"technical_seo_specialist", "Technical SEO Specialist", []string{"technical seo", "technical seo specialist", "technical seo manager"}},
	{"content_seo_specialist", "Content SEO Specialist", []string{"content seo", "content seo specialist", "content seo manager"}},
	{"link_building_specialist", "Link Building Specialist", []string{"link building", "linkbuilding", "seo outreach"}},
	{"seo_analyst", "SEO Analyst", []string{"seo analyst"}},
	// Generative-engine optimization. GEO, AEO and GSO name the same job, so they
	// collapse to one slug instead of splitting the facet three ways. Only the
	// spelled-out forms and the bound abbreviation resolve: a bare "geo" is
	// geography, and "Geo Data Analyst" must stay with the analysts.
	{"geo_specialist", "Generative Engine Optimization Specialist", []string{
		"generative engine optimization", "answer engine optimization",
		"generative search optimization", "geo specialist", "geo manager",
		"aeo specialist", "aeo manager",
	}},
	{"seo_specialist", "SEO Specialist", []string{"seo specialist", "seo manager"}},
	{"content_strategist", "Content Strategist", []string{"content strategist", "content marketer"}},
	{"community_manager", "Community Manager", []string{"community manager"}},
	{"social_media_manager", "Social Media Manager", []string{"social media manager", "smm manager", "smm specialist"}},
	// The function specialists a social media manager coordinates. They are peers of
	// the community manager, not rungs under the manager, so each keeps its own slug.
	{"paid_social_specialist", "Paid Social Specialist", []string{"paid social"}},
	{"content_creator", "Content Creator", []string{"content creator", "ugc creator", "content producer"}},
	// The funnel-owning functions. CRM and retention marketing fold into lifecycle:
	// they name the same post-conversion work, and an unqualified "CRM Manager" is as
	// often a Salesforce administrator, so only the qualified phrases resolve.
	{"demand_generation_manager", "Demand Generation Manager", []string{"demand generation", "demand gen"}},
	{"lifecycle_marketing_manager", "Lifecycle Marketing Manager", []string{"lifecycle marketing", "crm marketing", "retention marketing"}},
	{"performance_marketer", "Performance Marketer", []string{"performance marketing", "paid media", "media buyer"}},
	{"marketing_operations_manager", "Marketing Operations Manager", []string{"marketing operations", "marketing ops"}},
	{"brand_manager", "Brand Manager", []string{"brand manager", "brand marketing manager"}},
	{"pr_manager", "PR Manager", []string{"pr manager", "pr specialist", "public relations manager"}},
	{"influencer_marketing_manager", "Influencer Marketing Manager", []string{"influencer marketing"}},
	{"copywriter", "Copywriter", []string{"copywriter"}},
	{"marketing_analyst", "Marketing Analyst", []string{"marketing analyst"}},
	// GTM engineering builds the outbound revenue pipeline. It sits with the sales
	// category (see internal/classify) but is its own craft. Only the phrase resolves:
	// a bare "gtm" names the go-to-market discipline, which is a skill, not this role.
	{"gtm_engineer", "GTM Engineer", []string{"gtm engineer", "go-to-market engineer", "go to market engineer"}},

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

// nonGradeable are the named roles that do NOT compose with a seniority: the grade
// is meaningless or already baked in (fractional/founding/staff/lead/exec). Every
// other named role grades ("Senior Software Engineer" = senior_software_engineer),
// so the picker offers the graded role as a single option like the category
// composites.
var nonGradeable = map[string]bool{
	"fractional_cto": true, "fractional_cfo": true, "fractional_cmo": true,
	"fractional_coo": true, "fractional_cpo": true,
	"founder": true, "founding_engineer": true, "founding_designer": true, "founding_pm": true,
	"staff_engineer": true, "technical_lead": true, "vp_engineering": true, "chief_of_staff": true,
	"head_of_product": true, "head_of_growth": true, "head_of_design": true, "head_of_marketing": true,
	// team_lead already implies the lead grade; director is exec-level.
	"team_lead": true, "director": true,
	// Directorial and ops titles that state their own level: "Senior Creative
	// Director" is not a rung anyone posts.
	"art_director": true, "creative_director": true, "design_ops": true,
}

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

// NamedAliases maps each named role's slug to its curated title aliases (the same
// lowercase aliases used for title matching). Exposed so the web role picker can
// search by shorthand — typing "swe" or "devrel" finds the role, not just its
// display label. Returns a copy; the table stays immutable.
func NamedAliases() map[string][]string {
	m := make(map[string][]string, len(namedRoleTable))
	for _, r := range namedRoleTable {
		m[r.slug] = append([]string(nil), r.aliases...)
	}
	return m
}

// Derive returns a job's canonical role slugs from its resolved seniority,
// resolved category, and title:
//   - the seniority-only role ({seniority}) when the grade resolves;
//   - the bare category role ({category}) and its composite {seniority}_{category};
//   - at most one named role matched whole-word in the title, plus its composite
//     {seniority}_{named} when the named role is gradeable.
//
// The sources occupy distinct slug namespaces, so the result carries no
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
			// A gradeable named role composes with the seniority, so
			// "Senior Software Engineer" is one role, not "Senior" + "Software Engineer".
			if seniority != "" && !nonGradeable[na.slug] {
				roles = append(roles, seniority+"_"+na.slug)
			}
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
	// addGraded registers a role and every seniority-graded variant of it (the
	// bare slug plus "{Senior} {label}" for each grade) — the shape a category or
	// a gradeable named role takes.
	addGraded := func(slug, label string) {
		cat[slug] = label
		for sen, senLabel := range seniorityLabel {
			cat[sen+"_"+slug] = senLabel + " " + label
		}
	}
	for sen, senLabel := range seniorityLabel {
		cat[sen] = senLabel // seniority-only role
	}
	for c, noun := range categoryNoun {
		addGraded(c, noun)
	}
	for slug, label := range namedLabel {
		if nonGradeable[slug] {
			cat[slug] = label
		} else {
			addGraded(slug, label)
		}
	}
	return cat
}

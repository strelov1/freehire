package classify

import (
	"slices"
	"testing"

	"github.com/strelov1/freehire/internal/vocab"
)

func TestParse(t *testing.T) {
	cases := []struct {
		title         string
		wantSeniority string
		wantCategory  string
	}{
		{"Senior Backend Engineer", "senior", "backend"},
		{"Junior Frontend Developer", "junior", "frontend"},
		// The dotted abbreviations resolve through the bare "sr"/"jr" aliases —
		// '.' is a non-word boundary, so no separate dotted alias is needed.
		{"Sr. Backend Engineer", "senior", "backend"},
		{"Jr. Frontend Developer", "junior", "frontend"},
		// A title truncated mid-word by the feed still resolves via the
		// truncated-tail alias (the full "data science"/"data scientist" forms
		// win in any complete title).
		{"Senior Data Scien", "senior", "data_science"},
		{"Lead DevOps Engineer", "lead", "devops"},
		{"Staff Software Engineer", "staff", ""},
		{"Full Stack Developer", "", "fullstack"},
		{"Data Analyst", "", "data_analytics"},
		{"QA Automation Engineer", "", "qa"},
		{"Product Manager", "", "product"},
		{"Head of Engineering", "c_level", ""},
		{"Mid Backend Developer", "middle", "backend"},
		{"Старший backend-разработчик", "senior", "backend"},
		{"Младший фронтенд разработчик", "junior", "frontend"},
		{"Ведущий инженер DevOps", "lead", "devops"},
		{"Аналитик данных", "", "data_analytics"},
		{"Тестировщик ПО", "", "qa"},
		{"Дизайнер интерфейсов", "", "design"},
		// Russian category words are inflected — the dictionary lists the common
		// surface forms (whole-word match, no stemming), so these must resolve.
		{"Мобильный разработчик", "", "mobile"},
		{"Инженер по информационной безопасности", "", "security"},
		{"Специалист по продажам", "", "sales"},
		{"Специалист технической поддержки", "", "support"},
		{"Lead Senior Engineer", "lead", ""},
		// Architecture and network engineering are their own categories.
		{"Solutions Architect", "", "architecture"},
		{"Senior Software Architect", "senior", "architecture"},
		{"Cloud Architect", "", "architecture"},
		{"Системный архитектор", "", "architecture"},
		{"Network Engineer", "", "network_engineering"},
		{"Senior Network Administrator", "senior", "network_engineering"},
		{"Сетевой инженер", "", "network_engineering"},
		// A functional prefix wins over a bare "manager" (consistent precedence);
		// "operations" is now a recognized function, so these resolve to it.
		{"Reactor Operations Manager", "", "operations"},
		{"Sales Manager", "", "sales"},
		{"Support Manager", "", "support"},
		{"Operations Manager", "", "operations"},
		// AI-application roles (RAG/agents/LLM/prompt/applied AI) are their own
		// category; classic ML and explicitly ML-carrying titles stay ml_ai.
		{"AI Engineer", "", "ai_engineering"},
		{"GenAI Engineer", "", "ai_engineering"},
		{"LLM Engineer", "", "ai_engineering"},
		{"Senior Prompt Engineer", "senior", "ai_engineering"},
		{"Generative AI Researcher", "", "ai_engineering"},
		{"Applied AI Engineer", "", "ai_engineering"},
		{"RAG Engineer", "", "ai_engineering"},
		{"Machine Learning Engineer", "", "ml_ai"},
		{"Deep Learning Engineer", "", "ml_ai"},
		{"ML Engineer", "", "ml_ai"},
		// A combined ML-carrying form keeps the ML bucket (explicit ML beats bare AI).
		{"ML/AI Engineer", "", "ml_ai"},
		{"AI/ML Engineer", "", "ml_ai"},
		// AI titles whose words are not adjacent: the bare "ai engineer" alias cannot
		// span the intervening noun, so each form is listed. Agent/automation/research
		// work all builds ON models rather than training them, so they read as applied
		// AI, not ML.
		{"AI Product Engineer", "", "ai_engineering"},
		{"Agent Engineer", "", "ai_engineering"},
		{"AI Agent Engineer", "", "ai_engineering"},
		{"AI Research Engineer", "", "ai_engineering"},
		{"AI Automation Engineer", "", "ai_engineering"},
		{"Lead Agent Engineer (Langchain)", "lead", "ai_engineering"},
		// Hyphenated spellings of the same roles — a hyphen is a word boundary, so the
		// spaced aliases cannot reach them.
		{"AI-product engineer", "", "ai_engineering"},
		{"AI-Agent Engineer", "", "ai_engineering"},
		{"AI-Research Engineer", "", "ai_engineering"},
		{"AI-Automation Engineer", "", "ai_engineering"},
		// Trap: "algebrik.ai-product manager" is the domain "algebrik.ai" followed by
		// "product manager", not an AI-product role. Anchoring every alias on the role
		// noun ("engineer") keeps it in product.
		{"Algebrik.ai-Product Manager", "", "product"},
		// "AI-native"/"AI-enabled" describe how the engineer WORKS, not what they
		// build, so they claim no category — the rest of the title still decides.
		{"AI-Native Engineer (Test Automation)", "", "qa"},
		{"AI-Native Engineer", "", ""},
		{"Senior AI-Native Product Engineer", "senior", ""},
		// Generalist titles state no sub-discipline, so the category stays empty
		// rather than being guessed (they are carried by is_tech and a named role).
		{"Founding Engineer", "", ""},
		{"Product Engineer", "", ""},
		// SEO / social fold into marketing; "social media" beats a bare "manager".
		{"SEO Specialist", "", "marketing"},
		{"Social Media Manager", "", "marketing"},
		// LLM-mined alias gaps for existing categories: whole-word matching missed
		// these common IT titles (e.g. "security" does not match inside "cybersecurity").
		{"Cybersecurity Engineer", "", "security"},
		{"Senior Cyber Security Analyst", "senior", "security"},
		{"Firmware Engineer", "", "embedded"},
		{"Scrum Master", "", "project_management"},
		{"Program Manager", "", "project_management"},
		{"Скрам-мастер", "", "project_management"},
		{"Cat Herder", "", ""},
		{"", "", ""},
	}
	for _, c := range cases {
		got := Parse(c.title)
		if got.Seniority != c.wantSeniority || got.Category != c.wantCategory {
			t.Errorf("Parse(%q) = {%q, %q}, want {%q, %q}",
				c.title, got.Seniority, got.Category, c.wantSeniority, c.wantCategory)
		}
	}
}

// TestParseGradeBlindPhrases covers the role names that CONTAIN a grade word
// without stating a grade. Before the phrases were masked, "Member of Technical
// Staff" read as the staff grade and — because "staff" outranks "senior" in the
// table — even "Senior Member of Technical Staff" resolved to staff.
func TestParseGradeBlindPhrases(t *testing.T) {
	cases := []struct {
		title         string
		wantSeniority string
		wantCategory  string
	}{
		// MTS is a generic IC title at Oracle/xAI/Pure Storage, not the staff grade.
		{"Member of Technical Staff", "", ""},
		{"Member of the Technical Staff, Interpretability", "", ""},
		// With the phrase masked, the remaining words state the real grade.
		{"Senior Member of Technical Staff", "senior", ""},
		{"Senior Member of Technical Staff (SMTS) - Cloud Product Support", "senior", "support"},
		{"Principal Member of Technical Staff", "principal", ""},
		{"Principal Member of Technical Staff, Full-stack Engineer", "principal", "fullstack"},
		// "Mid-training" is a model-training stage (the sibling of pre- and
		// post-training), not the middle grade. The hyphen is a word boundary, so
		// the bare "mid" alias matched inside it.
		{"Member of Technical Staff - Mid-training", "", ""},
		// "Agent Post-Training" states a research area, not a role noun, so the
		// category stays empty — the dictionary does not guess from a topic word.
		{"Member of Technical Staff — Agent Post-Training", "", ""},
		// "Middle East" is a region. 142 of 217 prod titles carrying it were being
		// graded middle on the strength of the geography alone.
		{"Middle East Editor", "", ""},
		{"Sales Director – Middle East", "", "sales"},
		{"Senior Backend Engineer, Middle East", "senior", "backend"},
		// Hyphenated spellings of the same names. A hyphen is a word boundary, so the
		// spaced phrase cannot mask them — each spelling is its own entry.
		{"Enterprise Account Executive (Middle-East & Africa)", "", "sales"},
		{"Demand Planning Specialist – Africa & Middle-East", "", ""},
		{"Sales Lead-Generation Program Manager / Retail", "", "project_management"},
		// Regression: the honest grade abbreviation still resolves.
		{"Mid-Level Backend Engineer", "middle", "backend"},
		{"Mid Backend Engineer", "middle", "backend"},
		{"Middle Frontend Developer", "middle", "frontend"},
		// "Lead Generation" names a marketing function, not the lead grade.
		{"Lead Generation Specialist", "", ""},
		{"Lead Generation Manager", "", "management"},
		// Regression: an honest grade that merely shares the word is untouched.
		{"Staff Software Engineer", "staff", ""},
		{"Senior Staff Engineer", "staff", ""},
		{"Technical Staff Engineer", "staff", ""},
		{"Team Lead", "lead", ""},
	}
	for _, c := range cases {
		got := Parse(c.title)
		if got.Seniority != c.wantSeniority || got.Category != c.wantCategory {
			t.Errorf("Parse(%q) = {%q, %q}, want {%q, %q}",
				c.title, got.Seniority, got.Category, c.wantSeniority, c.wantCategory)
		}
	}
}

func TestCategories(t *testing.T) {
	tests := []struct {
		name string
		text string
		want []string
	}{
		{"single category", "Senior Backend Engineer", []string{"backend"}},
		{"several distinct categories, precedence order", "Backend Engineer and Data Engineer doing machine learning", []string{"data_engineering", "ml_ai", "backend"}},
		{"duplicate aliases collapse to one", "backend and back-end developer", []string{"backend"}},
		{"generic title resolves nothing", "Software Engineer", nil},
		{"empty", "", nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := Categories(tc.text); !slices.Equal(got, tc.want) {
				t.Errorf("Categories(%q) = %v, want %v", tc.text, got, tc.want)
			}
		})
	}
}

func TestCanonicalValuesAreInVocabulary(t *testing.T) {
	for _, e := range seniorityTable {
		if !slices.Contains(vocab.SeniorityValues, e.canonical) {
			t.Errorf("seniority alias %q -> %q not in SeniorityValues", e.alias, e.canonical)
		}
	}
	for _, e := range categoryTable {
		if !slices.Contains(vocab.CategoryValues, e.canonical) {
			t.Errorf("category alias %q -> %q not in CategoryValues", e.alias, e.canonical)
		}
	}
}

// TestParse_RoleExpansionBatch covers the categoryAliases expansion: non-tech role
// titles that feed the enrichment gate (higher-precision than the description tier)
// plus a few tech-role synonyms. Precision cases confirm the bare-"manager" fallback
// and unrelated roles are unaffected.
func TestParse_RoleExpansionBatch(t *testing.T) {
	cases := []struct{ title, wantCategory string }{
		// non-tech titles (feed the gate) — new forms not previously matched
		{"SDR", "sales"},
		{"Business Development Manager", "sales"},
		{"Account Manager", "sales"},
		{"Customer Success Manager", "customer_success"}, // split out of support
		{"Help Desk Technician", "support"},
		{"Customer Service Specialist", "support"},
		{"Copywriter", "marketing"},
		{"Content Writer", "marketing"},
		{"Brand Manager", "marketing"},
		// tech-role synonyms (facet quality)
		{"Platform Engineer", "devops"},
		{"Cloud Engineer", "devops"},
		{"Infrastructure Engineer", "devops"},
		{"System Administrator", "devops"},
		{"SDET", "qa"},
		{"Test Automation Engineer", "qa"},

		// precision — existing behavior must be unchanged
		{"Cloud Architect", "architecture"}, // not devops
		{"Sales Manager", "sales"},          // functional prefix still wins
		{"Growth Engineer", ""},             // "growth" deliberately not added (ambiguous)
	}
	for _, c := range cases {
		if got := Parse(c.title).Category; got != c.wantCategory {
			t.Errorf("Parse(%q).Category = %q, want %q", c.title, got, c.wantCategory)
		}
	}
}

// TestParse_ITCompanyRoles covers the ten IT-company role categories added by the
// expand-role-taxonomy change, with the fall-through guards that keep the terminal
// analyst->data_analytics / manager->management aliases (and bare sales / ux) from
// stealing a more specific role.
func TestParse_ITCompanyRoles(t *testing.T) {
	cases := []struct{ title, wantCategory string }{
		// recruiting
		{"Technical Recruiter", "recruiting"},
		{"IT Recruiter", "recruiting"},
		{"Talent Acquisition Specialist", "recruiting"},
		{"Рекрутер", "recruiting"},
		// hr
		{"HR Business Partner", "hr"},
		{"HRBP", "hr"},
		{"People Operations Manager", "hr"},
		{"Менеджер по персоналу", "hr"},
		// finance — guards against analyst-> and manager-> fall-throughs
		{"Financial Analyst", "finance"},
		{"Chief Financial Officer", "finance"},
		{"Bookkeeper", "finance"},
		{"Finance Manager", "finance"},
		{"Главный бухгалтер", "finance"},
		// legal
		{"Legal Counsel", "legal"},
		{"Compliance Officer", "legal"},
		{"Paralegal", "legal"},
		{"Юрисконсульт", "legal"},
		// operations — guard against manager-> fall-through
		{"Operations Manager", "operations"},
		{"Office Manager", "operations"},
		{"Chief of Staff", "operations"},
		{"Специалист по закупкам", "operations"},
		// customer_success — split out of support
		{"Customer Success Manager", "customer_success"},
		{"Onboarding Specialist", "customer_success"},
		{"Renewals Manager", "customer_success"},
		// business_analysis — guard against analyst-> fall-through; BI routes to analytics
		{"Business Analyst", "business_analysis"},
		{"Systems Analyst", "business_analysis"},
		{"Системный аналитик", "business_analysis"},
		{"BI Analyst", "data_analytics"},
		{"Business Intelligence Analyst", "data_analytics"},
		// solutions_engineering — guard: beats bare sales
		{"Sales Engineer", "solutions_engineering"},
		{"Solutions Consultant", "solutions_engineering"},
		{"Пресейл-инженер", "solutions_engineering"},
		// developer_relations
		{"Developer Advocate", "developer_relations"},
		{"DevRel", "developer_relations"},
		{"Technical Evangelist", "developer_relations"},
		// technical_writing — guard: beats ux/designer
		{"Technical Writer", "technical_writing"},
		{"UX Writer", "technical_writing"},
		{"Content Designer", "technical_writing"},
		{"Технический писатель", "technical_writing"},

		// precision — existing behavior must be unchanged
		{"Solutions Architect", "architecture"}, // stays architecture, not solutions_engineering
		{"Content Writer", "marketing"},         // stays marketing, not technical_writing
		{"Account Manager", "sales"},            // stays sales, not customer_success
		{"Data Analyst", "data_analytics"},      // plain analyst role unaffected
	}
	for _, c := range cases {
		if got := Parse(c.title).Category; got != c.wantCategory {
			t.Errorf("Parse(%q).Category = %q, want %q", c.title, got, c.wantCategory)
		}
	}
}

// TestParse_DesignSplit covers the split of the design craft: engineering
// draughting (mechanical, electrical, civil, chip) resolves to engineering_design,
// while `design` keeps meaning product, visual and experience design. The bare
// "Design Engineer" goes to the engineering side — that population is
// overwhelmingly mechanical in the catalogue — so a product hybrid has to state a
// marker of its own. The guards at the end pin the neighbouring aliases that the
// inserted block sits next to.
func TestParse_DesignSplit(t *testing.T) {
	cases := []struct{ title, wantCategory string }{
		// engineering design — the qualified forms
		{"Mechanical Design Engineer", "engineering_design"},
		{"Senior Electrical Design Engineer", "engineering_design"},
		{"Civil Design Engineer", "engineering_design"},
		{"Structural Designer", "engineering_design"},
		{"Piping Designer", "engineering_design"},
		{"Plumbing Designer / Drafter", "engineering_design"},
		{"Process Design Engineer", "engineering_design"},
		{"Packaging Design Engineer", "engineering_design"},
		{"Electrical Designer", "engineering_design"},
		{"Civil Designer", "engineering_design"},
		{"CAD Designer", "engineering_design"},
		{"Design Drafter", "engineering_design"},
		// The BIM / architectural-draughting family. It is the largest population still
		// left in `design` after the first pass — `revit` alone tags 2846 of those jobs —
		// and the bare "design engineer" alias cannot see any of these titles.
		{"Architectural Designer", "engineering_design"},
		{"BIM Designer", "engineering_design"},
		{"Revit Designer", "engineering_design"},
		{"Senior BIM Coordinator", "engineering_design"},
		{"Tool Designer", "engineering_design"},
		// Print and magazine layout is the product-design craft, so no bare
		// "layout designer" alias: the phrase names both trades.
		{"Magazine Layout Designer", "design"},
		{"Mold Designer", "engineering_design"},
		{"Draftsman", "engineering_design"},
		{"CAD Drafter", "engineering_design"},
		{"Design Technician", "engineering_design"},
		// Silicon and board design stay with `hardware`, which already owns the rest of
		// that team: "Hardware Design Engineer" and "FPGA Design Engineer" resolve there
		// through the earlier hardware aliases, so filing their colleagues under
		// draughting would split one discipline across two facets — and cost them the
		// technical treatment (enrichment, embeddings) they have today.
		{"PCB Design Engineer", "hardware"},
		{"PCB Layout Designer", "hardware"},
		{"Physical Design Engineer", "hardware"},
		{"Analog Design Engineer", "hardware"},
		{"RTL Design Engineer", "hardware"},
		{"VLSI Design Engineer", "hardware"},
		{"Senior VLSI Design Lead", "hardware"},
		{"Hardware Design Engineer", "hardware"},
		{"FPGA Design Engineer", "hardware"},
		// the bare title resolves to the engineering side
		{"Design Engineer", "engineering_design"},
		{"Senior Design Engineer", "engineering_design"},
		// product hybrids keep `design`, but only with an explicit marker
		{"Product Design Engineer", "design"},
		{"Design Systems Engineer", "design"},
		{"UI Engineer", "design"},
		{"UX Engineer", "design"},
		// The interface-design hybrids: the marker must be read BEFORE the bare
		// "design engineer" below, which would otherwise file them as draughting.
		{"UX Design Engineer", "design"},
		{"UI Design Engineer", "design"},
		{"UI/UX Design Engineer", "design"},
		{"Web Design Engineer", "design"},
		{"Design Engineer, Product", "design"},
		// product / visual design is untouched
		{"Senior Product Designer", "design"},
		{"UX Designer", "design"},
		{"UI/UX Designer", "design"},
		{"Visual Designer", "design"},
		{"Graphic Designer", "design"},
		{"Дизайнер интерфейсов", "design"},
		// Russian: "конструктор" is the draughting profession, not a UI designer, and
		// the hyphen is a word boundary so the compound form resolves through it.
		{"Инженер-конструктор", "engineering_design"},
		{"Конструктор металлоконструкций", "engineering_design"},

		// Software-anchored forms: "design" here qualifies the engineering, it is not
		// the craft. They must not be filed as draughting — that would take a software
		// job out of the technical catalogue entirely.
		{"Software Design Engineer", ""},
		{"Senior Software Design Engineer", ""},
		{"Software Design Engineer in Test", ""},
		{"Systems Design Engineer", ""},
		// Where a better category exists, say so rather than emitting nothing.
		{"Cloud Design Engineer", "devops"},
		{"Solution Design Engineer", "solutions_engineering"},
		{"Solutions Design Engineer", "solutions_engineering"},
		// These name design disciplines of their own and stay on the product side.
		{"Service Design Engineer", "design"},
		{"Experience Design Engineer", "design"},
		{"Sound Design Engineer", "design"},
		{"Game Design Engineer", "design"},

		// The rest of the silicon family rides with `hardware` too — the first pass
		// covered only six phrases and left these as draughting.
		{"ASIC Design Engineer", "hardware"},
		{"SoC Design Engineer", "hardware"},
		{"IC Design Engineer", "hardware"},
		{"Digital Design Engineer", "hardware"},
		{"Mixed Signal Design Engineer", "hardware"},
		{"DFT Design Engineer", "hardware"},
		{"Semiconductor Design Engineer", "hardware"},

		// precision — the neighbours of the inserted block must not shift
		{"Hardware Design Engineer", "hardware"},        // hardware precedes the design block
		{"Content Designer", "technical_writing"},       // technical_writing still wins
		{"UX Writer", "technical_writing"},              // ditto
		{"Senior Firmware Design Engineer", "embedded"}, // embedded precedes the design block
		{"Network Design Engineer", "network_engineering"},
	}
	for _, c := range cases {
		if got := Parse(c.title).Category; got != c.wantCategory {
			t.Errorf("Parse(%q).Category = %q, want %q", c.title, got, c.wantCategory)
		}
	}
}

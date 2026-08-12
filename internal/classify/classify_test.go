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
		{"Staff Software Engineer", "staff", "software_engineering"},
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
		// Non-English forms of the generic software_engineering catch-all: only
		// software- or language-anchored phrases resolve (never a bare
		// developer/engineer/programmer noun — prod titles show that noun alone
		// also names a real-estate developer in ES/PT/FR or a machine/CNC
		// programmer in ES/DE, in every language sampled, not just English).
		{"Desarrollador de Software Semi Senior", "senior", "software_engineering"},
		{"Ingeniera de Software con IA", "", "software_engineering"},
		{"Programador CNC", "", ""},
		// The seniorityTable is ALSO English+Russian only — a separate, not-yet-done
		// gap — so "Sênior"/"Starszy" resolve no grade even though the category
		// does. Category and seniority are independent matches (Parse runs both
		// against the same title), so this is not a category defect.
		{"Engenheira de Software Sênior", "", "software_engineering"},
		{"Développeur Java (H/F)", "", "software_engineering"},
		{"Développeur.euse senior (C#/React)", "senior", ""},
		{"Softwareentwickler (m/w/d)", "", "software_engineering"},
		{"Software-Entwickler (m/w/d)", "", "software_engineering"},
		{"SPS-Programmierer (m/w/d)", "", ""},
		{"Starszy Programista .NET z doświadczeniem w Power BI", "", "software_engineering"},
		{"Sviluppatore Software Junior", "junior", "software_engineering"},
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
		// build, so they name no discipline — the rest of the title still decides,
		// and the bare form falls to the generic software_engineering catch-all.
		{"AI-Native Engineer (Test Automation)", "", "qa"},
		{"AI-Native Engineer", "", "software_engineering"},
		{"Senior AI-Native Product Engineer", "senior", ""},
		// "Founding Engineer" states no sub-discipline either, so it resolves to the
		// same generic catch-all. "Product Engineer" deliberately does not (see
		// tech.go: prod titles split ~2:1 software vs manufacturing for that one), so
		// it stays empty rather than being guessed — carried by is_tech alone.
		{"Founding Engineer", "", "software_engineering"},
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
		// MTS is a generic IC title at Oracle/xAI/Pure Storage, not the staff grade —
		// but it IS software, so it resolves to the generic catch-all category.
		{"Member of Technical Staff", "", "software_engineering"},
		{"Member of the Technical Staff, Interpretability", "", "software_engineering"},
		// With the phrase masked, the remaining words state the real grade.
		{"Senior Member of Technical Staff", "senior", "software_engineering"},
		{"Senior Member of Technical Staff (SMTS) - Cloud Product Support", "senior", "support"},
		{"Principal Member of Technical Staff", "principal", "software_engineering"},
		{"Principal Member of Technical Staff, Full-stack Engineer", "principal", "fullstack"},
		// "Mid-training" is a model-training stage (the sibling of pre- and
		// post-training), not the middle grade. The hyphen is a word boundary, so
		// the bare "mid" alias matched inside it.
		{"Member of Technical Staff - Mid-training", "", "software_engineering"},
		// "Agent Post-Training" states a research area, not a role noun, so the
		// category does not narrow past the generic catch-all — the dictionary does
		// not guess from a topic word.
		{"Member of Technical Staff — Agent Post-Training", "", "software_engineering"},
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
		{"Staff Software Engineer", "staff", "software_engineering"},
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
		{"generic title resolves to the catch-all", "Software Engineer", []string{"software_engineering"}},
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
		// categoryNone is the one canonical outside the vocabulary: a blind alias uses
		// it to say "this title names no category", and matchCategory serves it as "".
		if e.canonical == categoryNone {
			continue
		}
		if !slices.Contains(vocab.CategoryValues, e.canonical) {
			t.Errorf("category alias %q -> %q not in CategoryValues", e.alias, e.canonical)
		}
	}
}

// A blind alias must never leak its sentinel to a caller — Parse and Categories both
// have to translate it, or "-" would be written into jobs.category and served as a
// facet value.
func TestCategoryNoneNeverEscapes(t *testing.T) {
	for _, title := range []string{
		"Software Design Engineer",
		"Senior Software Design Engineer",
		"Software Design Engineering Manager",
	} {
		if got := Parse(title).Category; got == categoryNone {
			t.Errorf("Parse(%q).Category leaked the sentinel", title)
		}
		if got := Categories(title); slices.Contains(got, categoryNone) {
			t.Errorf("Categories(%q) = %v leaked the sentinel", title, got)
		}
	}
	// The alias map feeds cmd/gen-contracts, so a leak there reaches the web picker.
	if _, ok := CategoryAliases()[categoryNone]; ok {
		t.Error("CategoryAliases() carries the sentinel; it would ship as a pickable value")
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

// TestParse_AliasGapFill covers title phrasing for IT sub-roles that already have a
// home category (project_management, security, devops, data_engineering,
// data_analytics) but whose common surface forms the dictionary never learned —
// found by sampling live prod titles. No new category values are involved.
func TestParse_AliasGapFill(t *testing.T) {
	cases := []struct{ title, wantCategory string }{
		// agile/PM
		{"Agile Coach", "project_management"},
		{"Senior Agile Coach", "project_management"},
		{"Release Train Engineer", "project_management"},
		{"Agile Transformation Lead", "project_management"},
		{"Agile Transformation Manager", "project_management"}, // guard: manager-> fall-through
		{"SAFe Scrum Master", "project_management"},
		{"SAFe Practitioner", "project_management"},
		{"Scaled Agile Framework Coach", "project_management"},

		// security — narrower technical niches
		{"IAM Engineer", "security"},
		{"Identity and Access Management Analyst", "security"}, // guard: analyst-> fall-through
		{"GRC Analyst", "security"},
		{"Vulnerability Management Engineer", "security"},
		{"Vulnerability Analyst", "security"}, // guard: analyst-> fall-through
		{"Incident Response Engineer", "security"},
		{"Red Team Operator", "security"},
		{"Red Teamer", "security"},
		{"Blue Team Analyst", "security"},
		{"Penetration Tester", "security"},
		{"Penetration Testing Engineer", "security"},
		{"Pentester", "security"},
		{"Pentest Engineer", "security"},
		{"Threat Intelligence Analyst", "security"},
		{"Threat Intel Lead", "security"},
		{"CISO", "security"},
		{"Chief Information Security Officer", "security"},
		{"DevSecOps Engineer", "security"}, // guard: stays security, not devops

		// data/devops
		{"Data Platform Engineer", "data_engineering"},
		{"Data Governance Lead", "data_engineering"},
		{"Data Governance Manager", "data_engineering"}, // guard: manager-> fall-through
		{"Data Steward", "data_engineering"},
		{"MLOps Engineer", "devops"}, // guard: stays devops, not ml_ai/data_engineering
		{"ML Ops Engineer", "devops"},
		{"Analytics Engineer", "data_analytics"},
		{"Senior Analytics Engineer", "data_analytics"},
		{"Platform Engineering Team Leader", "devops"}, // gerund form, not just "platform engineer"

		// precision — word-traps deliberately excluded, existing behavior unchanged
		{"Safe Driving Instructor", ""},       // bare "safe" must not resolve
		{"Customs Compliance Specialist", ""}, // bare "compliance" must not resolve to security
		{"Platform Engineer", "devops"},       // pre-existing alias unaffected
		{"Data Analyst", "data_analytics"},    // plain analyst role unaffected
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
		{"BIM Specialist", "engineering_design"},
		{"Die Designer", "engineering_design"},
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
		// job out of the technical catalogue entirely — and they must not fall through
		// to the business aliases further down the table either.
		{"Software Design Engineer", ""},
		{"Senior Software Design Engineer", ""},
		{"Software Design Engineer - Sales Tools", ""},
		{"Software Design Engineer, Support Platform", ""},
		{"Software Design Engineering Manager", ""},
		// SDET spelled out has a category of its own.
		{"Software Design Engineer in Test", "qa"},
		// "Systems Design Engineer" is NOT masked: a qualifier makes it draughting, and
		// blanking the category would strip the placement that vetoes deletion.
		{"HVAC Systems Design Engineer", "engineering_design"},
		{"Mechanical Systems Design Engineer", "engineering_design"},
		{"Systems Design Engineer", "engineering_design"},
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
		// The hyphenated spelling is the industry's own, and a hyphen is a word
		// boundary — so it needs its own alias, like "middle-east" and "ai-product".
		{"Mixed-Signal Design Engineer", "hardware"},
		{"Analog/Mixed-Signal Design Engineer", "hardware"},
		{"RF Design Engineer", "hardware"},
		{"RFIC Design Engineer", "hardware"},
		{"Analogue Design Engineer", "hardware"},
		{"Silicon Design Engineer", "hardware"},
		{"Memory Design Engineer", "hardware"},

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

// TestParse_GTMEngineering pins the go-to-market engineering family to `sales`,
// where revenue-operations titles already sit. The role builds the outbound data
// pipeline rather than selling, but the split is not title-separable from RevOps,
// and the granularity that matters lives in the role dictionary, not here. The
// block sits before the bare `sales` alias, so the guards pin the neighbours it
// must not disturb.
func TestParse_GTMEngineering(t *testing.T) {
	cases := []struct{ title, wantCategory string }{
		{"GTM Engineer", "sales"},
		{"Go-To-Market Engineer", "sales"},
		{"Go To Market Engineer", "sales"},
		{"Senior GTM Engineer", "sales"},

		// precision — the neighbours of the inserted block must not shift
		{"Sales Engineer", "solutions_engineering"},
		{"Sales Manager", "sales"},
		{"Revenue Operations Manager", "sales"},
	}
	for _, c := range cases {
		if got := Parse(c.title).Category; got != c.wantCategory {
			t.Errorf("Parse(%q).Category = %q, want %q", c.title, got, c.wantCategory)
		}
	}
}

// TestParse_MarketingDisciplines covers the marketing disciplines the category
// dictionary did not name. Most did not merely resolve to nothing — the generic
// "manager" alias further down the table claimed them for `management`, so this
// block corrects wrong data, not just missing data. Every alias is a phrase: the
// bare discipline nouns ("growth", "content", "performance") also occur in
// technical titles, and the guards at the end pin the ones they must not claim.
func TestParse_MarketingDisciplines(t *testing.T) {
	cases := []struct{ title, wantCategory string }{
		// were claimed by the generic "manager" alias
		{"Demand Generation Manager", "marketing"},
		{"Paid Media Manager", "marketing"},
		{"Community Manager", "marketing"},
		{"PR Manager", "marketing"},
		{"Answer Engine Optimization Manager", "marketing"},
		// resolved to nothing
		{"Growth Marketer", "marketing"},
		{"Paid Social Specialist", "marketing"},
		{"Paid Search Manager", "marketing"},
		{"Media Buyer", "marketing"},
		{"Link Building Specialist", "marketing"},
		{"Content Creator", "marketing"},
		{"Generative Engine Optimization Specialist", "marketing"},
		{"GEO Specialist", "marketing"},

		// precision — a marketing word inside a technical title must not claim it
		{"Growth Engineer", ""},
		{"Content Platform Engineer", "devops"},
		{"Geo Data Analyst", "data_analytics"},
		{"Community Manager, Developer Relations", "developer_relations"},
		// already-resolving marketing titles must not shift
		{"Growth Marketing Manager", "marketing"},
		{"Product Marketing Manager", "marketing"},
		{"Content Designer", "technical_writing"},
	}
	for _, c := range cases {
		if got := Parse(c.title).Category; got != c.wantCategory {
			t.Errorf("Parse(%q).Category = %q, want %q", c.title, got, c.wantCategory)
		}
	}
}

// TestParse_MarketingDisciplinesRU covers the Russian marketing titles. As
// elsewhere in this dictionary they are listed as full surface forms rather than
// stems, because the matcher requires word boundaries. Most were claimed by the
// bare "менеджер" alias before this block existed.
func TestParse_MarketingDisciplinesRU(t *testing.T) {
	cases := []struct{ title, wantCategory string }{
		{"Таргетолог", "marketing"},
		{"Контент-менеджер", "marketing"},
		{"Бренд-менеджер", "marketing"},
		{"Пиар-менеджер", "marketing"},
		{"Копирайтер", "marketing"},
		{"Комьюнити-менеджер", "marketing"},
		{"SMM-менеджер", "marketing"},

		// already resolving — must not shift
		{"Интернет-маркетолог", "marketing"},
		{"Менеджер по персоналу", "hr"},
		{"Менеджер по продажам", "sales"},
	}
	for _, c := range cases {
		if got := Parse(c.title).Category; got != c.wantCategory {
			t.Errorf("Parse(%q).Category = %q, want %q", c.title, got, c.wantCategory)
		}
	}
}

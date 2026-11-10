package classify

import (
	"slices"
	"testing"

	"github.com/strelov1/freehire/internal/dict/vocab"
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
		{"Специалист технической поддержки", "", "it_support"},
		// Non-English forms of the generic software_engineering catch-all: only
		// software- or language-anchored phrases resolve (never a bare
		// developer/engineer/programmer noun — prod titles show that noun alone
		// also names a real-estate developer in ES/PT/FR or a machine/CNC
		// programmer in ES/DE, in every language sampled, not just English). The
		// Spanish fallback (spanish.go) reads the bare noun only after its vetoes,
		// and files the CNC programmer with the machine shop. "Semi Senior" is Latin
		// America's middle rung, not a senior.
		{"Desarrollador de Software Semi Senior", "middle", "software_engineering"},
		{"Ingeniera de Software con IA", "", "software_engineering"},
		{"Programador CNC", "", "manufacturing"},
		// The main seniorityTable cannot see an accented grade — "sênior" does not
		// contain "senior" — so the Portuguese fallback (portuguese.go) reads it on
		// the accent-folded title.
		{"Engenheira de Software Sênior", "senior", "software_engineering"},
		{"Développeur Java (H/F)", "", "software_engineering"},
		// The French fallback (french.go) reads the bare noun once the business and
		// property developers are vetoed.
		{"Développeur.euse senior (C#/React)", "senior", "software_engineering"},
		{"Softwareentwickler (m/w/d)", "", "software_engineering"},
		{"Software-Entwickler (m/w/d)", "", "software_engineering"},
		{"SPS-Programmierer (m/w/d)", "", ""},
		{"Starszy Programista .NET z doświadczeniem w Power BI", "senior", "software_engineering"},
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
		{"Senior Member of Technical Staff (SMTS) - Cloud Product Support", "senior", "software_engineering"},
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
		{"Lead Generation Manager", "", ""},
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
		{"Help Desk Technician", "it_support"},
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

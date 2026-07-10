package classify

import (
	"slices"
	"testing"

	"github.com/strelov1/freehire/internal/enrich"
)

func TestParse(t *testing.T) {
	cases := []struct {
		title         string
		wantSeniority string
		wantCategory  string
	}{
		{"Senior Backend Engineer", "senior", "backend"},
		{"Junior Frontend Developer", "junior", "frontend"},
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
		{"Reactor Operations Manager", "", "management"},
		// A functional prefix wins over a bare "manager" (consistent precedence).
		{"Sales Manager", "", "sales"},
		{"Support Manager", "", "support"},
		{"Operations Manager", "", "management"},
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
	for alias, canon := range seniorityAliases {
		if !slices.Contains(enrich.SeniorityValues, canon) {
			t.Errorf("seniority alias %q -> %q not in SeniorityValues", alias, canon)
		}
	}
	for alias, canon := range categoryAliases {
		if !slices.Contains(enrich.CategoryValues, canon) {
			t.Errorf("category alias %q -> %q not in CategoryValues", alias, canon)
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
		{"Customer Success Manager", "support"},
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
		{"Cloud Architect", "architecture"},  // not devops
		{"Operations Manager", "management"}, // bare-manager fallback intact
		{"Sales Manager", "sales"},           // functional prefix still wins
		{"Growth Engineer", ""},              // "growth" deliberately not added (ambiguous)
	}
	for _, c := range cases {
		if got := Parse(c.title).Category; got != c.wantCategory {
			t.Errorf("Parse(%q).Category = %q, want %q", c.title, got, c.wantCategory)
		}
	}
}

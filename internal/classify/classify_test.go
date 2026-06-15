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
		{"Leading Solutions Architect", "", ""},
		{"Reactor Operations Manager", "", "management"},
		// A functional prefix wins over a bare "manager" (consistent precedence).
		{"Sales Manager", "", "sales"},
		{"Support Manager", "", "support"},
		{"Operations Manager", "", "management"},
		// AI / GenAI roles fold into ml_ai (research/applied AI share one bucket).
		{"GenAI Engineer", "", "ml_ai"},
		{"LLM Engineer", "", "ml_ai"},
		{"Senior Prompt Engineer", "senior", "ml_ai"},
		{"Generative AI Researcher", "", "ml_ai"},
		{"Applied AI Engineer", "", "ml_ai"},
		// SEO / social fold into marketing; "social media" beats a bare "manager".
		{"SEO Specialist", "", "marketing"},
		{"Social Media Manager", "", "marketing"},
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

package classify

import "testing"

// TestArgentineTitles pins the rioplatense title vocabulary: live postings from the
// Argentine crawl (pandapelatam, hiringroom, manatal, getonbrd, torre), measured
// 2026-10-27.
func TestArgentineTitles(t *testing.T) {
	cases := []struct {
		title, category, seniority string
	}{
		// The trades Argentina names its own way.
		{"Preventista", "sales", ""},
		{"Preventista (Importante cadena)", "sales", ""},
		{"Promovendedor/a Córdoba", "sales", ""},
		{"Clarkista con registro E2 (req212589) Eventual", "logistics", ""},
		{"Cadete zona Malvin", "logistics", ""},
		{"Mucama/o para importante hotel 5 estrellas", "hospitality", ""},
		{"Mucama de limpieza en casa particular", "personal_services", ""},
		{"Plegadores para importante empresa metalúrgica", "manufacturing", ""},
		{"Pickers / Armadores de pedidos (importante multinacional)", "logistics", ""},
		{"Despachante de aduana", "logistics", ""},
		// A title that opens with "Líder" names the lead; anywhere else the word praises
		// the employer.
		{"Líder Comercial (con base en Catamarca)", "sales", "lead"},
		{"Lider Tester", "qa", "lead"},
		{"Líder DevOps con foco en IA y Automatización (AIOps)", "devops", "lead"},
		{"Repositor/a Consumo Masivo (Empresa Líder)", "retail", ""},
		// The general manager is the executive, as the director general is.
		{"Gerente General", "", "c_level"},
		{"Asistente del Gerente General", "", ""},
		// An opening "Ssr" is the grade, even beside Next.js.
		{"Ssr Fullstack Developer (Node.js + Next.js)", "fullstack", "middle"},
		{"Frontend Engineer (SSR, Next.js)", "frontend", ""},
		{"SSR / SSG Developer (Next.js)", "", ""},
		{"Разработчик SSR (Next.js)", "software_engineering", ""},
	}
	for _, c := range cases {
		got := Parse(c.title)
		if got.Category != c.category || got.Seniority != c.seniority {
			t.Errorf("Parse(%q) = %q/%q, want %q/%q", c.title, got.Category, got.Seniority, c.category, c.seniority)
		}
	}
}

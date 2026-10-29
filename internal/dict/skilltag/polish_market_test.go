package skilltag

import (
	"slices"
	"testing"
)

// TestParse_PolishMarket covers the 2026-10-29 batch mined from the open Polish-language
// postings (erecruiter, traffit, nofluffjobs, solidjobs, smartrecruiters, teamtailor,
// recruitee, eures, whatjobs-pl). Each is the Polish name of a practice the dictionary
// already knows in English, in the case the clause puts it; every positive is a real
// clause, lightly trimmed.
func TestParse_PolishMarket(t *testing.T) {
	cases := []struct {
		name   string
		in     string
		want   []string
		absent []string
	}{
		{"tests", "Potrafisz tworzyć testy automatyczne, jednostkowe; doświadczenie w pisaniu testów jednostkowych i testów integracyjnych.",
			[]string{"test-automation", "unit-testing", "integration-testing"}, nil},
		{"microservices", "Doświadczenie w pracy z architekturą mikroserwisową i konteneryzacją (Docker).",
			[]string{"microservices", "containerization"}, nil},
		{"rdbms", "Znajomość relacyjnych baz danych (PostgreSQL, Oracle).", []string{"rdbms"}, nil},
		{"version control", "Praktyczna znajomość systemów kontroli wersji (Git).", []string{"version-control"}, nil},
		{"data", "Doświadczenie w modelowaniu danych i budowie hurtowni danych.", []string{"data-modeling", "data-warehousing"}, nil},
		{"distributed", "Doświadczenie w projektowaniu systemów rozproszonych; znajomość wzorców projektowych.",
			[]string{"distributed-systems", "design-patterns"}, nil},
		{"embedded requirement", "Doświadczenie w programowaniu systemów wbudowanych (C, C++).", []string{"embedded-systems"}, nil},
		{"ml", "Projekty związane z uczeniem maszynowym i dużymi modelami językowymi.", []string{"machine-learning", "llm"}, nil},
		{"observability", "Monitoring i obserwowalność: Prometheus, Grafana.", []string{"observability"}, nil},
		// An agency's list of services and an employer's pitch are not the requirement.
		{"agency services", "Nasze obszary działania to m.in.: doradztwo technologiczne, tworzenie oprogramowania, systemy wbudowane, rozwiązania chmurowe.",
			nil, []string{"embedded-systems"}},
		{"ai pitch", "Narzędzia oparte na sztucznej inteligencji wspierające codzienną pracę.", nil, []string{"ai"}},
		{"freight", "Obsługa konteneryzacji ładunków w porcie morskim.", nil, []string{"containerization"}},
		{"document control", "Kontrola wersji dokumentów i archiwizacja.", nil, []string{"version-control"}},
		{"electrician", "Montaż instalacji elektrycznych i sieci komputerowe. Kontakt: Julia.", nil, []string{"julia"}},
		{"cyber sector", "Międzynarodowa firma z branży cyberbezpieczeństwa i ochrony danych.", nil, []string{"cybersecurity"}},
	}
	for _, c := range cases {
		got := Parse(c.in)
		for _, w := range c.want {
			if !slices.Contains(got, w) {
				t.Errorf("%s: Parse(%q) = %v, want %q", c.name, c.in, got, w)
			}
		}
		for _, a := range c.absent {
			if slices.Contains(got, a) {
				t.Errorf("%s: Parse(%q) = %v, must not contain %q", c.name, c.in, got, a)
			}
		}
	}
}

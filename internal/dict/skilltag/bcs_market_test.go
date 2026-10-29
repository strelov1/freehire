package skilltag

import (
	"slices"
	"testing"
)

// TestParse_BCSMarket covers the 2026-10-29 batch mined from the open Croatian, Serbian
// and Bosnian postings placed in hr/rs/ba/me (talentlyft, eures, zohorecruit,
// smartrecruiters, teamtailor). Each is the BCS name of a practice the dictionary
// already knows in English, in the case the posting wrote it; every positive is a real
// clause, lightly trimmed.
func TestParse_BCSMarket(t *testing.T) {
	cases := []struct {
		name   string
		in     string
		want   []string
		absent []string
	}{
		{"rdbms instrumental", "Iskustvo u radu s relacijskim bazama podataka (Oracle, PostgreSQL)", []string{"rdbms"}, nil},
		{"rdbms serbian", "Poznavanje relacionih baza podataka i SQL-a", []string{"rdbms"}, nil},
		{"ml genitive", "Osnovno znanje strojnog učenja i obrade prirodnog jezika", []string{"machine-learning", "nlp"}, nil},
		{"ml serbian", "Iskustvo sa mašinskim učenjem i dubokim učenjem", []string{"machine-learning", "deep-learning"}, nil},
		{"no diacritics", "Poznavanje masinskog ucenja i racunarskog vida", []string{"machine-learning", "computer-vision"}, nil},
		{"cybersecurity", "IT stručnjak za kibernetičku sigurnost", []string{"cybersecurity"}, nil},
		{"cybersecurity serbian", "Iskustvo u oblasti sajber bezbednosti", []string{"cybersecurity"}, nil},
		{"oop", "Poznavanje objektno orijentiranog programiranja (C#, Java)", []string{"oop"}, nil},
		{"ci", "Postavljanje kontinuirane integracije i automatizacije testiranja",
			[]string{"ci-cd", "test-automation"}, nil},
		{"data", "Modeliranje podataka, skladišta podataka i vizualizacija podataka",
			[]string{"data-modeling", "data-warehousing", "data-visualization"}, nil},
		{"genai", "Razvoj rješenja generativne umjetne inteligencije", []string{"generative-ai"}, nil},
		{"systems", "Razvoj ugrađenih sustava i distribuiranih sistema",
			[]string{"embedded-systems", "distributed-systems"}, nil},
		{"words", "Iskustvo s virtualizacijom, kontejnerizacijom i mikroservisima",
			[]string{"virtualization", "containerization", "microservices"}, nil},
		{"pentest", "Provođenje penetracijskog testiranja", []string{"penetration-testing"}, nil},
		// The plain words and the left-out terms are no skill.
		{"plain", "Tražimo komunikativnu osobu, sklonu timskom radu, s vozačkom dozvolom B kategorije.", nil,
			[]string{"ci-cd", "rdbms", "data-modeling"}},
		{"delivery", "Kontinuirana isporuka robe kupcima i kontrola verzija dokumentacije.", nil,
			[]string{"ci-cd", "version-control"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Parse(c.in)
			for _, w := range c.want {
				if !slices.Contains(got, w) {
					t.Errorf("Parse(%q) = %v, missing %q", c.in, got, w)
				}
			}
			for _, a := range c.absent {
				if slices.Contains(got, a) {
					t.Errorf("Parse(%q) = %v, unexpected %q", c.in, got, a)
				}
			}
		})
	}
}

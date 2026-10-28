package skilltag

import (
	"slices"
	"testing"
)

// TestParse_FrenchMarket covers the 2026-10-28 batch mined from the open French-language
// postings (wttj, flatchr, werecruit, smartrecruiters, teamtailor, workday, eures,
// recruitee, lever, join). Each is the French name of a practice the dictionary already
// knows in English; every positive is a real clause, lightly trimmed.
func TestParse_FrenchMarket(t *testing.T) {
	cases := []struct {
		name   string
		in     string
		want   []string
		absent []string
	}{
		{"ci-cd", "Construire les chaînes d'intégration et de déploiement continu (CI/CD)", []string{"ci-cd"}, nil},
		{"ci-cd plural", "La démarche DevOps : intégration et déploiement continus.", []string{"ci-cd"}, nil},
		{"ci-cd delivery", "Automatiser la livraison continue des services sur GCP", []string{"ci-cd"}, nil},
		{"observability", "Monitoring / observabilité : Prometheus, Grafana", []string{"observability"}, nil},
		{"genai", "Maîtrise des technologies de l'IA générative appliquées au développement", []string{"generative-ai"}, nil},
		{"tests", "Élaborer les tests unitaires et les tests d'intégration ; tests automatisés",
			[]string{"unit-testing", "integration-testing", "test-automation"}, nil},
		{"iac", "Mise en œuvre de l'infrastructure en tant que code (IaC)", []string{"infrastructure-as-code"}, nil},
		{"containers", "Connaissance de la conteneurisation Docker et/ou OpenShift", []string{"containerization"}, nil},
		{"version control", "Bonne pratique des outils de gestion de versions et de code source", []string{"version-control"}, nil},
		{"architecture", "Architecture hexagonale, architecture événementielle et systèmes distribués",
			[]string{"hexagonal-architecture", "event-driven-architecture", "distributed-systems"}, nil},
		{"data", "Développer des pipelines de données et la modélisation des données ; gouvernance de la donnée",
			[]string{"data-pipelines", "data-modeling", "data-governance"}, nil},
		{"data stores", "Entrepôts de données (Snowflake, BigQuery) et lac de données", []string{"data-warehousing", "data-lake"}, nil},
		{"ml", "Apprentissage automatique, apprentissage profond et vision par ordinateur",
			[]string{"machine-learning", "deep-learning", "computer-vision"}, nil},
		{"nlp", "Solutions de traitement du langage naturel (NLP)", []string{"nlp"}, nil},
		{"oop", "Vous maîtrisez la programmation orientée objet", []string{"oop"}, nil},
		{"bi", "Licence en informatique, orientée informatique décisionnelle", []string{"business-intelligence"}, nil},
		{"embedded", "Développement de logiciel embarqué et de systèmes embarqués", []string{"embedded-systems"}, nil},
		{"rdbms", "Bonne maîtrise de SQL et des bases de données relationnelles (SGBDR)", []string{"rdbms"}, nil},
		{"pentest", "Une société spécialisée dans les tests d'intrusion", []string{"penetration-testing"}, nil},
		{"security", "Garantir la cybersécurité et la protection des données", []string{"cybersecurity"}, nil},
		// The plain words are no skill, and neither is the data quality a clerk looks after.
		{"clerk", "Vous veillez à la qualité des données clients et à la facturation.", nil,
			[]string{"data-quality"}},
		{"plain words", "Nous recherchons une personne rigoureuse, autonome et souriante.", nil,
			[]string{"ci-cd", "unit-testing", "data-quality"}},
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
					t.Errorf("Parse(%q) = %v, must NOT contain %q", c.in, got, a)
				}
			}
		})
	}
}

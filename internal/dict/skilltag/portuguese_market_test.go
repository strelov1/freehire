package skilltag

import (
	"slices"
	"testing"
)

// TestParse_PortugueseMarket covers the 2026-10-27 batch mined from the requirement
// clauses ("Conhecimento em …", "Experiência com …") of the open Brazilian and Portuguese
// technical postings (gupy, solides, quickin, catho, vagas, inhire, pandape, workday,
// smartrecruiters, teamtailor, whatjobs-br/pt). Each is the Portuguese name of a practice
// the dictionary already knows in English; every positive is a real clause, lightly
// trimmed.
func TestParse_PortugueseMarket(t *testing.T) {
	cases := []struct {
		name   string
		in     string
		want   []string
		absent []string
	}{
		{"microservices", "Vivência com microsserviços, sistemas distribuídos e mensageria",
			[]string{"microservices", "distributed-systems"}, nil},
		{"microservices no accent", "Conhecimento em arquitetura de microsservicos", []string{"microservices"}, nil},
		{"observability", "Domínio de observabilidade, log estruturado e rastreio de requisição", []string{"observability"}, nil},
		{"ci-cd", "Integração contínua e implantação contínua com GitHub Actions", []string{"ci-cd"}, nil},
		{"iac", "Conhecimento em infraestrutura como código (Terraform)", []string{"infrastructure-as-code"}, nil},
		{"unit testing", "Experiência com testes unitários e testes de integração",
			[]string{"unit-testing", "integration-testing"}, nil},
		{"test automation", "Implementar estratégias de testes automatizados e automação de testes", []string{"test-automation"}, nil},
		{"version control", "Experiência com Git para versionamento de código e controle de versão", []string{"version-control"}, nil},
		{"design patterns", "Boas práticas, padrões de projeto e arquitetura limpa",
			[]string{"design-patterns", "clean-architecture"}, nil},
		{"hexagonal", "Arquitetura hexagonal e desenvolvimento orientado a testes", []string{"hexagonal-architecture", "tdd"}, nil},
		{"data modeling", "Modelagem de dados e modelagem dimensional; modelação de dados",
			[]string{"data-modeling", "dimensional-modeling"}, nil},
		{"data platform", "Construir pipelines de dados para a engenharia de dados e a ciência de dados",
			[]string{"data-pipelines", "data-engineering", "data-science"}, nil},
		{"data governance", "Governança de dados e visualização de dados", []string{"data-governance", "data-visualization"}, nil},
		{"ml", "Aprendizado de máquina e aprendizado profundo", []string{"machine-learning", "deep-learning"}, nil},
		{"genai nlp", "Inteligência artificial generativa e processamento de linguagem natural",
			[]string{"generative-ai", "nlp"}, nil},
		{"eda", "Arquitetura orientada a eventos com Kafka", []string{"event-driven-architecture"}, nil},
		{"oop", "Programação orientada a objetos e orientação a objetos", []string{"oop"}, nil},
		{"bi", "Ferramentas de inteligência de negócios", []string{"business-intelligence"}, nil},
		{"embedded", "Desenvolvimento de sistemas embarcados em C", []string{"embedded-systems"}, nil},
		{"computer vision", "Projetos de visão computacional", []string{"computer-vision"}, nil},
		{"rdbms", "Bancos de dados relacionais (Oracle, SQL Server, PostgreSQL)", []string{"rdbms"}, nil},
		{"vector db", "Bancos de dados vetoriais e RAG", []string{"vector-databases"}, nil},
		{"pentest", "Testes de intrusão e testes de penetração", []string{"penetration-testing"}, nil},
		// The Portuguese words alone are no skill.
		{"delivery", "Motorista entregador: entrega contínua aos nossos clientes.", nil, []string{"ci-cd"}},
		{"plain words", "Buscamos pessoas com vontade de aprender e trabalhar em equipe.", nil,
			[]string{"data-modeling", "rest", "ci-cd"}},
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

package skilltag

import (
	"slices"
	"testing"
)

// TestParse_SpanishMarket covers the 2026-10-27 batch mined from the requirement clauses
// ("Conocimientos de …", "Experiencia en …") of the open Spanish-language technical
// postings (infojobs, occ, pandapelatam, teamtailor, factorial, getonbrd, torre,
// hiringroom, personio, eures, getmanfred). Each is the Spanish name of a practice the
// dictionary already knows in English; every positive is a real clause, lightly trimmed.
func TestParse_SpanishMarket(t *testing.T) {
	cases := []struct {
		name   string
		in     string
		want   []string
		absent []string
	}{
		{"microservices", "Experiencia en arquitecturas de microservicios con Java", []string{"microservices"}, nil},
		{"rest", "Diseño e integración de APIs REST y servicios REST", []string{"rest"}, nil},
		{"ci-cd", "Conocimientos de integración continua y despliegue continuo", []string{"ci-cd"}, nil},
		{"ci-cd no accent", "Experiencia en integracion continua (Jenkins)", []string{"ci-cd"}, nil},
		{"observability", "Experiencia en observabilidad: logging, métricas y trazas", []string{"observability"}, nil},
		{"iac", "Infraestructura como código con Terraform", []string{"infrastructure-as-code"}, nil},
		{"unit testing", "Escribir pruebas unitarias y tests unitarios", []string{"unit-testing"}, nil},
		{"test automation", "Experiencia en pruebas automatizadas y automatización de pruebas", []string{"test-automation"}, nil},
		{"version control", "Manejo de control de versiones (Git)", []string{"version-control"}, nil},
		{"design patterns", "Conocimientos de patrones de diseño y principios de arquitectura", []string{"design-patterns"}, nil},
		{"architectures", "Arquitectura hexagonal y arquitectura limpia", []string{"hexagonal-architecture", "clean-architecture"}, nil},
		{"ddd tdd", "Diseño dirigido por el dominio y desarrollo dirigido por pruebas", []string{"domain-driven-design", "tdd"}, nil},
		{"distributed systems", "Experiencia en sistemas distribuidos de alta disponibilidad", []string{"distributed-systems"}, nil},
		{"data modeling", "Modelado de datos y modelado dimensional", []string{"data-modeling", "dimensional-modeling"}, nil},
		{"data platform", "Construir pipelines de datos para la ingeniería de datos y la ciencia de datos",
			[]string{"data-pipelines", "data-engineering", "data-science"}, nil},
		{"data stores", "Diseño de almacén de datos y lago de datos", []string{"data-warehousing", "data-lake"}, nil},
		{"data governance", "Gobierno del dato y calidad del dato", []string{"data-governance", "data-quality"}, nil},
		{"data viz", "Visualización de datos con Power BI", []string{"data-visualization", "powerbi"}, nil},
		{"ml", "Aprendizaje automático y aprendizaje profundo", []string{"machine-learning", "deep-learning"}, nil},
		{"genai nlp", "IA generativa y procesamiento del lenguaje natural", []string{"generative-ai", "nlp"}, nil},
		{"eda", "Arquitectura orientada a eventos con Kafka", []string{"event-driven-architecture"}, nil},
		{"oop", "Programación orientada a objetos en C#", []string{"oop"}, nil},
		{"bi", "Herramientas de inteligencia de negocio", []string{"business-intelligence"}, nil},
		{"computer vision", "Proyectos de vision artificial y visión por computador", []string{"computer-vision"}, nil},
		// The Spanish words alone are no skill.
		{"plain words", "Buscamos personas con ganas de aprender y trabajar en equipo.", nil,
			[]string{"data-quality", "rest", "ci-cd"}},
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

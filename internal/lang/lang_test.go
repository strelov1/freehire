package lang

import "testing"

func TestDetect(t *testing.T) {
	cases := []struct {
		name string
		text string
		want string
	}{
		{
			name: "english prose",
			text: "We are looking for a senior backend engineer to design and build " +
				"scalable services in Go and PostgreSQL across our distributed platform.",
			want: "en",
		},
		{
			name: "portuguese prose",
			text: "Estamos em busca de um coordenador de manutenção para liderar as " +
				"operações de facilities e garantir a confiabilidade dos nossos sistemas.",
			want: "pt",
		},
		{
			name: "russian prose",
			text: "Мы ищем опытного backend-разработчика для проектирования и создания " +
				"масштабируемых сервисов на Go и PostgreSQL в нашей распределённой платформе.",
			want: "ru",
		},
		{
			name: "html tags ignored, prose wins",
			text: "<p><strong>Sobre a vaga</strong></p><ul><li>Responsável pela operação " +
				"logística e gestão da equipe de transportes na unidade, garantindo a " +
				"confiabilidade dos processos e o cumprimento dos prazos de entrega.</li>" +
				"<li>Liderar a equipe operacional e acompanhar os indicadores de desempenho.</li></ul>",
			want: "pt",
		},
		{
			name: "too short -> empty",
			text: "Software Engineer",
			want: "",
		},
		{
			name: "empty -> empty",
			text: "",
			want: "",
		},
		{
			// Tech-heavy English prose (brands, acronyms, code) scores "unreliable"
			// in whatlanggo yet is clearly English: the English-leaning fallback
			// rescues it instead of dropping the language.
			name: "unreliable english tech-sparse -> en",
			text: "Stack: React, Redux, TypeScript, Node.js, GraphQL, Docker, " +
				"Kubernetes, AWS, Terraform, PostgreSQL, Redis, Kafka. CI/CD via " +
				"GitHub Actions. Remote-first team.",
			want: "en",
		},
		{
			// Reliable non-English prose must NOT be pulled to "en" by the fallback:
			// German scores reliably and keeps its own code.
			name: "reliable german stays de",
			text: "Wir suchen zum naechstmoeglichen Zeitpunkt einen erfahrenen " +
				"Entwickler fuer unser Team in Berlin, der unsere Plattform " +
				"weiterentwickelt und den Betrieb der Dienste sicherstellt.",
			want: "de",
		},
		{
			// Precision boundary: an unreliable posting whose best guess is NOT
			// English is left empty rather than guessed — never mislabelled.
			name: "unreliable non-english-guess stays empty",
			text: "Join Acme! We use Go, gRPC, k8s. Ship fast. Great perks. " +
				"Apply now via our portal today.",
			want: "",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Detect(c.text); got != c.want {
				t.Errorf("Detect() = %q, want %q", got, c.want)
			}
		})
	}
}

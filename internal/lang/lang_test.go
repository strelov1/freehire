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
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Detect(c.text); got != c.want {
				t.Errorf("Detect() = %q, want %q", got, c.want)
			}
		})
	}
}

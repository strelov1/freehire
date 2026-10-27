package location

import "testing"

// TestPortugueseWorkMode pins the Portuguese arrangement prose. The sentences are cut
// from live Brazilian and Portuguese postings (gupy, solides, quickin, catho, vagas,
// inhire, pandape, smartrecruiters, workday, teamtailor and whatjobs-br/pt), measured
// 2026-10-27.
func TestPortugueseWorkMode(t *testing.T) {
	cases := []struct {
		name, desc, want string
	}{
		// The model stated, with or without accents, markup and a spaced colon.
		{"modelo hibrido", "<p>Modelo de trabalho: híbrido</p><p>Localização: São Paulo</p>", "hybrid"},
		{"regime", "Modelo de trabalho regime híbrido, com alternância semanal.", "hybrid"},
		{"modalidade", "Modalidade: híbrida, de segunda a sexta-feira.", "hybrid"},
		{"office days", "Regime híbrido (3 dias presenciais no bairro São Sebastião e 2 dias home office).", "hybrid"},
		{"days a week", "Localização e frequência: 2x por semana presencial, com visitas aos clientes.", "hybrid"},
		{"office weekday", "Modelo de trabalho: presencial, com home office às quartas-feiras.", "hybrid"},
		{"european days", "O regime de trabalho é com 3 dias por semana no escritório e 2 dias de trabalho remoto.", "hybrid"},
		{"mixed", "Jornada híbrida (presencial + home office) segunda a sexta das 8h às 17h30.", "hybrid"},
		// Remote needs a stated model or a totality; "home office" as the model is remote.
		{"100% remoto", "Atuação 100% remota, contrato PJ.", "remote"},
		{"model remote", "Modelo de trabalho: remoto. Benefícios: plano de saúde.", "remote"},
		{"model home office", "Modelo de trabalho: (Home Office) segunda a sexta - feira das 08h00 às 17h48.", "remote"},
		{"local home office", "Horário de trabalho: seg. a sex.; local de trabalho: home office", "remote"},
		{"totalmente", "Esta é uma posição totalmente remota, dentro do Brasil.", "remote"},
		{"modality remote", "Modalidade remota, com disponibilidade para viagens ocasionais.", "remote"},
		// Onsite is stated, or home working refused.
		{"presencial", "Modelo de trabalho: presencial. Requisitos: ensino médio completo.", "onsite"},
		{"spaced colon", "Modalidade : Presencial Benefícios: vale-transporte.", "onsite"},
		{"atuacao", "Atuação presencial em Blumenau/SC.", "onsite"},
		{"regime presencial", "Regime presencial, de segunda a sexta.", "onsite"},
		{"sem home office", "Horário comercial, sem home office.", "onsite"},
		// A negated or refused offer is not the arrangement.
		{"not possible", "Home office: não é possível para esta função. Modelo presencial.", "onsite"},
		{"negated", "Esta vaga não é 100% remota; modelo híbrido em Lisboa.", "hybrid"},
		// The trigger words in their other senses state no arrangement.
		{"allowance only", "Benefícios: vale refeição, auxílio home office, day off no aniversário.", ""},
		{"remote support", "Suporte remoto aos usuários e abertura de chamados.", ""},
		{"remote work skills", "Estrutura adequada para trabalho remoto (ambiente silencioso e boa conexão).", ""},
		{"intern boilerplate", "Não há possibilidade de inclusão dos estagiários em programa de gestão por teletrabalho ou sistema híbrido de aprendizagem.", ""},
		{"interview", "Etapas: entrevista presencial com o gestor.", ""},
		{"anywhere", "Home office: trabalhe de onde quiser durante as férias.", ""},
	}
	for _, c := range cases {
		if got := WorkModeFromDescription(c.desc); got != c.want {
			t.Errorf("%s: WorkModeFromDescription(%q) = %q, want %q", c.name, c.desc, got, c.want)
		}
	}
}

// TestPortugueseWorkModeLocationMarkers pins the Portuguese and Spanish words a location
// line states its arrangement in.
func TestPortugueseWorkModeLocationMarkers(t *testing.T) {
	cases := []struct{ loc, want string }{
		{"Remoto, Remoto, BR", "remote"},
		{"São Paulo (Remoto), SP, BR", "remote"},
		{"Híbrido - Digital Workplace, Híbrido, BR", "hybrid"},
		{"São Paulo - SP - Presencial (Zona Sul), São Paulo - SP, BR", "onsite"},
	}
	for _, c := range cases {
		if got := Parse(c.loc).WorkMode; got != c.want {
			t.Errorf("Parse(%q).WorkMode = %q, want %q", c.loc, got, c.want)
		}
	}
}

// TestNegationIsAWholeWord pins the shared guard: a negation is a word of its own and
// governs its own clause. "Entorno", "bono" and "polígono" end in "no", and a "não" in the
// sentence before does not refuse the next one. Both were measured on the Spanish crawl
// (infojobs, pandapelatam), where they had hidden a stated hybrid.
func TestNegationIsAWholeWord(t *testing.T) {
	cases := []struct{ name, desc, want string }{
		{"entorno", "Proyecto estable en un entorno dinámico, modelo híbrido.", "hybrid"},
		{"bono", "Caja y fondo de ahorro + bono por desempeño, esquema híbrido y flexibilidad.", "hybrid"},
		{"poligono", "Ubicación: polígono Hytasa, Sevilla. Modalidad: híbrida.", "hybrid"},
		{"previous clause", "Esta vaga não é 100% remota; modelo híbrido em Lisboa.", "hybrid"},
		{"still negates", "Horario de oficina, no teletrabajo.", ""},
	}
	for _, c := range cases {
		if got := WorkModeFromDescription(c.desc); got != c.want {
			t.Errorf("%s: WorkModeFromDescription(%q) = %q, want %q", c.name, c.desc, got, c.want)
		}
	}
}

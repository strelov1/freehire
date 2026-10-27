package location

import "testing"

// TestSpanishWorkMode pins the Spanish arrangement prose. The sentences are cut from live
// Spanish-language postings (factorial, teamtailor, getonbrd, torre, personio, eures and
// the whatjobs Spanish-speaking markets), measured 2026-10-27.
func TestSpanishWorkMode(t *testing.T) {
	cases := []struct {
		name, desc, want string
	}{
		// The model stated, with or without accents and markup.
		{"modalidad", "<p>Ubicación: Málaga</p><p>Modalidad: híbrida (2-3 días en oficina)</p>", "hybrid"},
		{"no accents", "Modalidad hibrida en Santa Fe, Ciudad de México.", "hybrid"},
		{"esquema", "Trabajo en esquema híbrido (3 días home office - 2 días en sitio).", "hybrid"},
		{"office days", "Lugar/modalidad de trabajo: 3 dias de home office / 2 dias presencial.", "hybrid"},
		{"presencial days", "Barcelona (1-2 días presenciales a la semana, resto remoto).", "hybrid"},
		{"mixed", "40% presencial / 60% teletrabajo, presencial y teletrabajo", "hybrid"},
		// Home working listed as a benefit is hybrid.
		{"benefit", "<li>Horario flexible, teletrabajo, asesoramiento de especialistas</li>", "hybrid"},
		{"allowance", "Salario: 21000 euros + ayuda comedor + ayuda teletrabajo", "hybrid"},
		{"linkedin line", "Hay opciones de teletrabajo/trabajo desde casa disponibles para este puesto.", "hybrid"},
		{"home office support", "Así como apoyo de home office y oportunidades de desarrollo.", "hybrid"},
		// Remote needs a totality, and is read before the hybrid phrase it contains.
		{"100%", "Modalidad: 100% remoto. Descripción del puesto: buscamos un lead.", "remote"},
		{"100% teletrabajo", "100% teletrabajo: trabajarás en remoto desde cualquier punto de España.", "remote"},
		{"teletrabajo 100", "Ofrecemos posibilidades de teletrabajo 100%, aceptando candidatos de toda España.", "remote"},
		{"remota", "La posición es remota desde Chile, con fortaleza en frontend.", "remote"},
		{"totalmente", "Este puesto totalmente remoto se centrará en la supervisión.", "remote"},
		// Onsite is stated, or home working refused.
		{"100% presencial", "Jornada completa y modalidad 100 % presencial en nuestra oficina de Marbella.", "onsite"},
		{"modalidad presencial", "Modalidad de trabajo: presencial. Requisitos: ingeniería.", "onsite"},
		{"empleo presencial", "Lugar de trabajo: empleo presencial", "onsite"},
		{"sin teletrabajo", "Horario de lunes a viernes, sin teletrabajo.", "onsite"},
		{"not offered", "Horario de oficina; no se ofrece teletrabajo.", "onsite"},
		{"no possibility", "Puesto en planta, sin posibilidad de teletrabajo.", "onsite"},
		// A negated, refused, hedged or seasonal offer is not the arrangement.
		{"refused", "Teletrabajo/remoto no corresponde en esta posición.", ""},
		{"hedged", "Modalidad: híbrida, con posibilidad de evaluar 100% remoto.", "hybrid"},
		{"summer perk", "Jornada intensiva y teletrabajo durante agosto.", ""},
		{"seasonal full", "Teletrabajo completo en verano, Navidad y Semana Santa.", ""},
		{"start month", "Incorporación en agosto, modalidad 100% remoto.", "remote"},
		{"markup between", "<b>Modalidad</b> mixta: oficina y casa.", "hybrid"},
		// The trigger words in their other senses state no arrangement.
		{"tutoring", "Si deseas impartir clases presenciales o en línea, ¡únete!", ""},
		{"interview", "Haremos una entrevista personal (presencial u online).", ""},
		{"remote support", "Soporte técnico remoto y escalado de tickets.", ""},
		{"remote team", "Capacidad para colaborar de forma remota y autónoma.", ""},
		{"hybrid tech", "Adaptabilidad a entornos tecnológicos híbridos (legacy + moderno).", ""},
		{"hybrid apps", "Aplicaciones móviles nativas/híbridas con altos estándares.", ""},
		{"perk breakfast", "Desayuno en jornada presencial, snacks en oficina. Tipo de jornada: híbrida", "hybrid"},
	}
	for _, c := range cases {
		if got := WorkModeFromDescription(c.desc); got != c.want {
			t.Errorf("%s: WorkModeFromDescription(%q) = %q, want %q", c.name, c.desc, got, c.want)
		}
	}
}

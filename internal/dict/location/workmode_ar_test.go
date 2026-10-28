package location

import "testing"

// TestArgentineWorkMode pins the rioplatense arrangement prose: sentences cut from live
// postings placed in Argentina (manatal, workday, smartrecruiters, oracle, greenhouse,
// pandapelatam, hiringroom), measured 2026-10-27.
func TestArgentineWorkMode(t *testing.T) {
	cases := []struct {
		name, desc, want string
	}{
		// "Esquema" and "jornada laboral" carry the model in Argentina.
		{"esquema remoto", "Esquema de trabajo remoto con flexibilidad horaria.", "remote"},
		{"esquema bare", "Horario de trabajo: L a V 9 a 18 hs. Esquema remoto", "remote"},
		{"esquema de empleo", "<p><strong>Esquema de empleo:</strong> presencial.</p>", "onsite"},
		{"esquema de empleo hibrido", "Esquema de empleo: híbrido. Modalidad de contratación: eventual.", "hybrid"},
		{"esquema presencial", "Adaptación a esquema presencial.", "onsite"},
		{"jornada laboral", "Jornada laboral: presencial, de lunes a viernes de 08:00 a 17:00 h.", "onsite"},
		{"jornada laboral hibrida", "Lugar de trabajo: Retiro. Jornada laboral híbrida.", "hybrid"},
		{"modalidad laboral", "Modalidad laboral: presencial de lunes a viernes de 7.30 a 17.30 hs", "onsite"},
		{"modalidad remota", "Modalidad de trabajo remota con asistencias eventuales para reuniones en cliente final.", "remote"},
		{"weekly visits", "Modalidad de trabajo <strong>remota con 1/2 asistencias semanales en CABA.</strong>", "hybrid"},
		{"modalidad es", "La modalidad es híbrida con asistencia en oficinas en zona CABA.", "hybrid"},
		{"ubicacion", "Ubicación: Híbrido. Tipo de contrato: Full-time.", "hybrid"},
		{"space before colon", "Modalidad de trabajo : híbrida (3 presencial x 2 home office).", "hybrid"},
		{"presencialidad requerida", "Modalidad: presencialidad requerida en Retiro (CABA)", "onsite"},
		{"remoto 100", "El rol admite trabajo remoto 100% y requiere dominio de SQL.", "remote"},
		{"presencial 100", "Administrador de servidores - presencial 100% en Berisso (La Plata)", "onsite"},
		// A share of office days is hybrid.
		{"share", "Buenos Aires, Parque Patricios | Híbrido (40% presencial) full-time", "hybrid"},
		{"share remote", "Modalidad: 80% remoto – presencialidad en CABA", "hybrid"},
		{"visits far", "Modalidad de trabajo: remota con flexibilidad horaria y 2 asistencias semanales en CABA.", "hybrid"},
		{"share after label", "Modalidad de trabajo: remoto, 40% presencial.", "hybrid"},
		// Not the arrangement.
		{"negated share", "Puesto en planta; no se ofrece 50% teletrabajo.", ""},
		{"seasonal share", "50% teletrabajo durante agosto.", ""},
		{"torre template", "Ubicación: Remoto - Av. Andres Reyes 338, San Isidro, Peru. Modalidad de trabajo: híbrido", "hybrid"},
		{"benefit refund", "Reintegro de gastos de home office.", ""},
		{"discount", "Gimnasio 100% bonificado y prepaga 100% bonificada.", ""},
		{"attendance bonus", "Bono de puntualidad semanal 10%, bono de asistencia semanal 10%, empleo de oficina.", ""},
		{"meetings", "Disponibilidad para asistir a encuentros presenciales semanales en Córdoba.", ""},
	}
	for _, c := range cases {
		if got := WorkModeFromDescription(c.desc); got != c.want {
			t.Errorf("%s: WorkModeFromDescription(%q) = %q, want %q", c.name, c.desc, got, c.want)
		}
	}
}

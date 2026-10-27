package classify

import "testing"

// TestSpanishHeadsAndFields pins the Spanish title vocabulary. A Spanish title names a
// generic head and the field it works in ("Técnico de mantenimiento", "Auxiliar de
// cocina"), so a role noun or a field resolves and a bare generic head does not. Every
// title here is a live posting from the Spanish-language crawl (infojobs, occ,
// pandapelatam, teamtailor, factorial, getonbrd, torre, hiringroom), measured
// 2026-10-27.
func TestSpanishHeadsAndFields(t *testing.T) {
	cases := []struct {
		title, category, seniority string
	}{
		// The gender forms: slashed, hyphenated, parenthesised, and the notice.
		{"Camarero/a", "hospitality", ""},
		{"MOZO/A DE ALMACÉN", "logistics", ""},
		{"Operarios/a de Bodega en Quilicura Turno Noche", "logistics", ""},
		{"ENCARGADO-A DE OBRA", "skilled_trades", ""},
		{"COCINERO (H/M/X) zona OLEIROS en CORUÑA", "hospitality", ""},
		{"Psicólogo clínico  (H/M/X) / Mallorca", "healthcare", ""},
		// The main table already carries the alias; only the gender suffix hid it.
		{"OPERARIO/A DE PRODUCCIÓN", "manufacturing", ""},
		{"Desarrollador/a de Software", "software_engineering", ""},
		// Accents are folded: titles are written both ways.
		{"TECNICO DE MANTENIMIENTO", "skilled_trades", ""},
		{"Técnico/a de mantenimiento", "skilled_trades", ""},

		// A generic head takes the field's category.
		{"Ejecutivo de ventas", "sales", ""},
		{"Asesor/a comercial", "sales", ""},
		{"Auxiliar de cocina", "hospitality", ""},
		{"Dependiente/a de tienda", "retail", ""},
		{"Auxiliar de limpieza", "personal_services", ""},
		{"Auxiliar de recursos humanos", "hr", ""},
		{"Técnico/a de RRHH", "hr", ""},
		{"Analista de compras", "operations", ""},
		{"Auxiliar contable", "finance", ""},
		{"Administrativo/a contable", "finance", ""},
		{"Analista de cuentas por pagar", "finance", ""},
		{"Asesor de crédito grupal", "finance", ""},
		{"Coordinador de operaciones", "operations", ""},
		{"Técnico/a Superior en Prevención de Riesgos Laborales", "occupational_safety", ""},
		{"Supervisor de seguridad e higiene", "occupational_safety", ""},
		{"Auxiliar de Seguridad y Medio Ambiente", "occupational_safety", ""},
		// A head and field whose answer is not the field's.
		{"Ingeniero de ventas", "solutions_engineering", ""},
		{"Ingeniero de procesos", "industrial_engineering", ""},
		{"Ingeniero/a de mantenimiento ferroviario", "industrial_engineering", ""},
		{"Auxiliar de Enfermería / Gerocultor/a", "healthcare", ""},
		{"Camarero/a de pisos", "hospitality", ""},
		{"Cajero/a Bancario – Cananea, Sonora", "finance", ""},
		{"Educador/a social", "social_services", ""},
		{"Jefe/a de obra", "skilled_trades", "lead"},
		{"Jefe de Proyectos", "project_management", "lead"},

		// A role noun outranks a field: the warehouse hand moves goods for the shop.
		{"Electricista", "skilled_trades", ""},
		{"Conductor/a de camión", "logistics", ""},
		{"Mozo/a de almacén para tienda", "logistics", ""},
		{"Ayudante camarero centro comercial", "hospitality", ""},
		{"Teleoperador/a", "support", ""},
		{"Abogado/a", "legal", ""},
		{"Reclutador", "recruiting", ""},
		{"Fisioterapeuta", "healthcare", ""},
		{"Especialista en Toma de Muestras", "healthcare", ""},
		{"Vigilantes de Seguridad", "public_safety", ""},
		{"Socorrista", "public_safety", ""},
		{"Monitor/a deportivo/a", "education", ""},
		{"MANIPULADOR/A", "manufacturing", ""},
		{"Maquinista con clase D/ Quilicura / Turno tarde", "logistics", ""},
		{"Montacarguista", "logistics", ""},
		{"Tablajero", "retail", ""},
		{"Auxiliar de abarrotes", "retail", ""},

		// The generic heads that name a desk on their own.
		{"Administrativo/a", "administration", ""},
		{"Comercial", "sales", ""},
		{"Vendedor", "sales", ""},

		// IT.
		{"Analista Programador/a Oracle (PL/SQL y Forms)", "software_engineering", ""},
		{"Programador Jr", "software_engineering", "junior"},
		{"INGENIEROS INFORMÁTICOS", "software_engineering", ""},
		{"Ingeniero de Datos", "data_engineering", ""},
		{"Analista de datos BI", "data_analytics", ""},
		{"Científico de Datos", "data_science", ""},
		{"Analista funcional", "business_analysis", ""},
		{"Administrador/a de Sistemas Unix/Linux", "devops", ""},
		{"Técnico/a informático/a", "it_support", ""},
		{"Técnico/a de Soporte IT  - Oficinas Sevilla", "it_support", ""},
		{"Técnico/a de Redes", "network_engineering", ""},
		{"Analista de ciberseguridad", "security", ""},
		{"Arquitecto de Soluciones", "architecture", ""},
		{"Ingeniero/a de Pruebas de Sistemas", "qa", ""},
		{"Diseñador gráfico", "design", ""},

		// The developer and programmer who write no software.
		{"Desarrollador de Negocios Exportación", "sales", ""},
		{"Desarrollador inmobiliario", "", ""},
		{"Programador CNC", "manufacturing", ""},
		{"Programador PLC", "", ""},
		{"Programador/a de rutas - Hospitalet de Llobregat", "logistics", ""},
		{"Programador(a) de Manutenção Industrial", "", ""},
		// "Médico" is also the equipment a salesperson sells or an engineer services.
		{"Medico General", "healthcare", ""},
		{"Ejecutivo de ventas equipo médico", "sales", ""},
		{"Ingeniero de Servicio Equipo Médico IMAGENOLOGIA", "industrial_engineering", ""},
		{"Representante médico", "sales", ""},
		// A security firm's salesperson sells.
		{"Consultor Comercial Seguridad Privada", "sales", ""},
		{"Supervisor de Seguridad Privada", "public_safety", ""},
		// "Maestro" is also a foreman and a restaurant chain.
		{"Maestro de Cocina", "hospitality", ""},
		{"Operarios Juan Maestro", "", ""},
		// The meter reader and the toll collector are not accountants.
		{"Operarios lectores de contadores", "", ""},
		{"COBRADOR DE PEAJE", "", ""},
		// A banking lawyer is not a bank teller.
		{"Advogado(a) / Bancário", "", ""},
		// "Centro comercial" is the mall, "ingeniero comercial" a Chilean degree.
		{"Gerente centro comercial", "", ""},
		{"Ingeniero comercial", "", ""},

		// A generic head with no field resolves nothing.
		{"Técnico", "", ""},
		{"Ayudante general", "", ""},
		{"Operarios Part Time", "", ""},
		{"Gerente de sucursal", "", ""},
	}
	for _, c := range cases {
		got := Parse(c.title)
		if got.Category != c.category || got.Seniority != c.seniority {
			t.Errorf("Parse(%q) = {%q, %q}, want {%q, %q}", c.title, got.Category, got.Seniority, c.category, c.seniority)
		}
	}
}

// TestSpanishGrades pins the Spanish grade words. "Jefe" leads, as the German "-leiter"
// does; "responsable", "gerente", "supervisor" and "coordinador" state no grade, as
// "manager" and "supervisor" state none in the main table.
func TestSpanishGrades(t *testing.T) {
	cases := []struct {
		title, seniority string
	}{
		{"Becario/a de RRHH", "intern"},
		{"Beca RRHH – Generalista", "intern"},
		{"Prácticas Community Manager", "intern"},
		{"Gestor/a de Publicación de Vehículos Online - Contrato en Prácticas", "intern"},
		{"Practicante de recursos humanos", "intern"},
		{"Pasante Talento Humano", "intern"},
		{"Aprendiz de Infraestructura", "intern"},
		// The trainee grade outranks the lead.
		{"Becario Jefe de Proyecto", "intern"},
		{"Jefe de Turno", "lead"},
		{"Jefe/a de cocina", "lead"},
		{"Líder de ventas", "lead"},
		{"Lider de Tienda", "lead"},
		{"Líder Técnico de Logística", "lead"},
		// The assistant OF a lead states no grade.
		{"Ayudante de jefe de obra", ""},
		{"Adjunto al jefe de tienda", ""},
		{"Subjefe de turno", ""},
		{"Director/a general", "c_level"},
		{"Directora ejecutiva", "c_level"},
		{"DIRECTOR FINANCIERO Y CONTROL DE GESTIÓN", "c_level"},
		// A plain director, manager or supervisor states no grade.
		{"Director comercial", ""},
		{"Responsable de mantenimiento", ""},
		{"Gerente de tienda", ""},
		{"Supervisor de producción", ""},
		{"Coordinador de proyectos", ""},
		// Latin America's semi senior is the middle rung; a range keeps its upper bound.
		{"Ingeniero(a) de Datos Semi Senior", "middle"},
		{"DESARROLLADOR FULL STACK SEMI-SENIOR (JAVA+REACT)", "middle"},
		{"COMERCIAL SEMISENIOR (GUAYAQUIL)", "middle"},
		{"Semi Sr Data Scientist", "middle"},
		{"Administrativo/a Contable SSR con exp en Tango gestión", "middle"},
		{"Analista funcional Ssr- Sr", "senior"},
		{"Ingeniero/a de Datos SSR/SR", "senior"},
		// Portuguese shares the fallback: its internship and middle grade.
		{"Estágio Administrativo", "intern"},
		{"Estagiário de Logística - Roteirização", "intern"},
		{"Analista de Recursos Humanos Pleno", "middle"},
	}
	for _, c := range cases {
		if got := Parse(c.title).Seniority; got != c.seniority {
			t.Errorf("Parse(%q).Seniority = %q, want %q", c.title, got, c.seniority)
		}
	}
}

// TestSpanishFallbackLeavesOtherLanguages pins titles from other languages that reach
// the same fallback: the Spanish table must not claim them.
func TestSpanishFallbackLeavesOtherLanguages(t *testing.T) {
	cases := []struct {
		title, category string
	}{
		{"Addetto vendita", ""},
		{"Tecnico manutentore", ""},
		{"Engenheiro de Projetos Mecânicos Sr", ""},
	}
	for _, c := range cases {
		if got := Parse(c.title).Category; got != c.category {
			t.Errorf("Parse(%q).Category = %q, want %q", c.title, got, c.category)
		}
	}
}

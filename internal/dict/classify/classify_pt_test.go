package classify

import "testing"

// TestPortugueseHeadsAndFields pins the Portuguese title vocabulary. A Portuguese title
// names a generic head and the field it works in ("Auxiliar de Serviços Gerais",
// "Analista de Departamento Pessoal"), so a role noun or a field resolves and a bare
// generic head does not. Every title here is a live posting from the Brazilian and
// Portuguese crawl (gupy, solides, quickin, catho, empregos, vagas, inhire, pandape,
// smartrecruiters, teamtailor, workday), measured 2026-10-27.
func TestPortugueseHeadsAndFields(t *testing.T) {
	cases := []struct {
		title, category, seniority string
	}{
		// The gender forms: parenthesised, slashed, the European notice.
		{"Auxiliar de Serviços Gerais", "personal_services", ""},
		{"AUXILIAR DE SERVIÇOS GERAIS / SP", "personal_services", ""},
		{"Consultor(a) de Vendas", "sales", ""},
		{"Consultor (a) de Atendimento e Vendas", "sales", ""},
		{"Operador/a de Armazém", "logistics", ""},
		{"Auxiliar de Armazém (m/f)", "logistics", ""},
		{"Técnico/a de Manutenção Fotovoltaica", "skilled_trades", ""},
		{"Pessoa Desenvolvedora Java Pleno/Sênior", "software_engineering", "senior"},

		// IT: the developer, the data seats, support, security, infrastructure.
		{"Desenvolvedor .NET Sênior", "software_engineering", "senior"},
		{"Engenheiro de Dados-Presencial-Pinhais-PR", "data_engineering", ""},
		{"Cientista de Dados", "data_science", ""},
		{"Analista de Dados", "data_analytics", ""},
		{"Analista de BI - Júnior", "data_analytics", "junior"},
		{"Analista de Suporte N2 Jr", "it_support", "junior"},
		{"Técnico em Informática I", "it_support", "junior"},
		{"Analista de Infraestrutura Cloud (AWS)", "devops", ""},
		{"Coordenador de Segurança da Informação", "security", "lead"},
		{"Analista de Qualidade de Software", "qa", ""},
		{"Analista de Desenvolvimento de Software III", "software_engineering", "senior"},
		{"Estágio Curricular em Engenharia Informática", "software_engineering", "intern"},
		{"Suporte Aplicacional", "it_support", ""},
		{"Pessoa Consultora SAP MM (Oil & Energy) - Sênior", "solutions_engineering", "senior"},
		// Human development is HR, the business developer sells.
		{"Analista de Desenvolvimento Humano", "hr", ""},
		{"Desenvolvedor de Negócios", "sales", ""},
		{"Desenvolvedor de Produtos Têxteis", "", ""},

		// Safety: the information, the occupational, the guard.
		{"Técnico de Segurança do Trabalho", "occupational_safety", ""},
		{"Técnico Superior de Segurança no Trabalho", "occupational_safety", ""},
		{"Controlador de Acesso - HOSPITAL ASSUNÇÃO - Noturno", "public_safety", ""},
		{"Fiscal de Prevenção de Perdas", "public_safety", ""},
		{"Bombeiro Civil", "public_safety", ""},

		// "Fiscal": the tax desk, the shop floor, the building site.
		{"Analista Fiscal Sênior", "finance", "senior"},
		{"Fiscal de Loja", "retail", ""},
		{"Fiscal de Obras Civil - PR", "skilled_trades", ""},
		// The lawyer outranks the tax field.
		{"Advogado Tributário", "legal", ""},
		{"INCRA Sede DF - Estágio em Direito", "legal", "intern"},

		// Finance and people.
		{"Assistente Financeiro", "finance", ""},
		{"Analista Financeiro - Contas a pagar", "finance", ""},
		{"Analista Contábil", "finance", ""},
		{"Recuperador de Crédito", "finance", ""},
		{"Assistente de Departamento Pessoal", "hr", ""},
		{"Assistente de DP - Cálculo Rescisório", "hr", ""},
		{"Analista de Recrutamento e Seleção", "recruiting", ""},

		// Sales, service, the shop.
		{"Executivo de Vendas", "sales", ""},
		{"Corretor de Imóveis", "sales", ""},
		{"Gerente de Contas", "sales", ""},
		{"Assistente de Atendimento", "support", ""},
		{"Gerente de Loja", "retail", ""},
		{"Operador/a Hipermercado Reposição", "retail", ""},
		{"Operadora de Caixa", "retail", ""},
		{"Chefe de Secção de Loja (40h) - Loulé", "retail", "lead"},

		// Logistics, production, the trades.
		{"Operador de Empilhadeira", "logistics", ""},
		{"Conferente de Mercadorias c/empilhador", "logistics", ""},
		{"Auxiliar de Almoxarifado", "logistics", ""},
		{"Auxiliar de Expedição", "logistics", ""},
		{"Auxiliar de Produção", "manufacturing", ""},
		{"Inspetor de Qualidade", "manufacturing", ""},
		{"Analista de PCP", "manufacturing", ""},
		{"Auxiliar de Manutenção", "skilled_trades", ""},
		{"Serralheiro", "skilled_trades", ""},
		{"Técnico de Refrigeração", "skilled_trades", ""},
		{"Técnico de Manutenção AVAC", "skilled_trades", ""},

		// Healthcare, education, food.
		{"Técnico de Enfermagem - UTI - Noturno", "healthcare", ""},
		{"Atendente de Farmácia", "healthcare", ""},
		{"Auxiliar de Saúde Bucal", "healthcare", ""},
		{"Higienista Oral (m/f)", "healthcare", ""},
		{"Técnico de Cardiopneumologia", "healthcare", ""},
		{"Coordenadora Pedagógica", "education", "lead"},
		{"Formador/a", "education", ""},
		{"Ajudante de Cozinha", "hospitality", ""},
		{"Oficial de Cozinha", "hospitality", ""},
		{"Atendente de Lanchonete", "hospitality", ""},
		{"Operador/a Restauração", "hospitality", ""},

		// Engineering seats, projects.
		{"Engenheiro Civil Sênior", "industrial_engineering", "senior"},
		{"Engenheiro de Vendas", "solutions_engineering", ""},
		{"Estágio em Engenharia Civil", "industrial_engineering", "intern"},
		{"Coordenador de Projetos", "project_management", "lead"},

		// A generic head with no field resolves nothing.
		{"Analista", "", ""},
		{"Auxiliar", "", ""},
		{"Técnico", "", ""},
		{"Engenheiro", "", ""},
		{"Banco de Talentos", "", ""},
	}
	for _, c := range cases {
		got := Parse(c.title)
		if got.Category != c.category || got.Seniority != c.seniority {
			t.Errorf("Parse(%q) = {%q, %q}, want {%q, %q}", c.title, got.Seniority, got.Category, c.seniority, c.category)
		}
	}
}

// TestPortugueseGrades pins the Brazilian grade spellings: the accented "Sênior" and
// "Júnior" the main table cannot see, "pleno" below "sênior" in a range, the ladder
// abbreviation "PL" and the Roman rungs after a Portuguese desk head, and the board
// directors. "Supervisor" and a plain "diretor" state no grade, as in English.
func TestPortugueseGrades(t *testing.T) {
	cases := []struct{ title, seniority string }{
		{"Analista Contábil Júnior", "junior"},
		{"Comprador(a) Sênior", "senior"},
		{"Analista Fiscal Pleno", "middle"},
		{"Desenvolvedor Full Stack Pleno/Sênior Web", "senior"},
		{"Advogado(a) Júnior/Pleno", "middle"},
		{"ANALISTA SEGUROS PL", "middle"},
		{"Analista Contabil PL", "middle"},
		{"Analista Financeiro I", "junior"},
		{"Operador de Produção II", "middle"},
		{"Analista III - Analista Administrativo", "senior"},
		{"Desenvolvedor FullStack III", "senior"},
		{"Jovem Aprendiz", "intern"},
		{"Estagiário(a) de RH", "intern"},
		{"Coordenador(a) de RH", "lead"},
		{"Chefe de Cozinha", "lead"},
		{"Diretor Geral", "c_level"},
		{"Diretora Financeira", "c_level"},
		{"Diretor Comercial", ""},
		{"Supervisor de Vendas", ""},
		{"Gerente de Projetos", ""},
		// An assistant TO a coordinator or a director states no grade.
		{"Assistente de Coordenação", ""},
		{"Secretária da Diretoria Executiva", ""},
		// "PL" and a Roman rung mean nothing without a Portuguese desk head before them.
		{"Oracle PL/SQL Developer", ""},
		{"Java Developer - Warsaw, PL", ""},
		{"Espec Santander II 8h", ""},
		{"Analista de PL/SQL", ""},
	}
	for _, c := range cases {
		if got := Parse(c.title).Seniority; got != c.seniority {
			t.Errorf("Parse(%q).Seniority = %q, want %q", c.title, got, c.seniority)
		}
	}
}

// TestPortugueseTableLeavesOtherLanguagesAlone pins titles from the Spanish, French and
// English crawl that reach the same fallback and must not be read as Portuguese.
func TestPortugueseTableLeavesOtherLanguagesAlone(t *testing.T) {
	cases := []struct{ title, category string }{
		// The Spanish table still answers first, and its vetoes still hold.
		{"Programador de mantenimiento", ""},
		{"Programador de Manutenção", ""},
		{"Vendedor de loja", "sales"},
		// Spanish "auxiliar de sala" waits tables; it is not a classroom aide.
		{"Auxiliar de Sala con Certificado de Discapacidad", ""},
		// A bare "DP" is DP World or a vessel's dynamic positioning.
		{"DP Operator", ""},
		{"Terminal Operator - DP World", ""},
	}
	for _, c := range cases {
		if got := Parse(c.title).Category; got != c.category {
			t.Errorf("Parse(%q).Category = %q, want %q", c.title, got, c.category)
		}
	}
}

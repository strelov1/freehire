package classify

import "regexp"

// Portuguese titles share the Spanish shape — a generic HEAD followed by the FIELD it
// works in ("Auxiliar de Serviços Gerais", "Analista de Departamento Pessoal",
// "Técnico de Segurança do Trabalho", "Operador/a de Armazém") — and they reach the
// Spanish fallback already: where the two languages spell a word alike ("vendedor",
// "programador", "comercial", "administrativo", "logística") the Spanish table reads
// it. Measured 2026-10-27 over 37,600 open Brazilian and Portuguese postings (gupy,
// solides, quickin, catho, empregos, vagas, inhire, recrutei, pandape, smartrecruiters,
// workday, teamtailor, recruitee, personio, landingjobs), 57% of the Brazilian titles
// resolved no category, and the unresolved mass was the Portuguese spelling of the
// same heads and fields — "vendas" not "ventas", "manutenção" not "mantenimiento",
// "limpeza" not "limpieza", "serviços gerais", "departamento pessoal".
//
// So this table is the Spanish table's sibling, ordered in the same bands (a head and
// field whose answer differs from the field's default; the role nouns; the fields;
// the desk heads), and it only carries what the Spanish table cannot already read. It
// runs AFTER the Spanish table, on the same folded title (foldSpanish: accents
// dropped, "/a", "(a)" and "(m/f)" cut), so no existing answer changes — including
// the Spanish table's vetoes: a title the Spanish table blinds stays blind.
//
// A generic head with no field — analista, auxiliar, assistente, técnico, operador,
// consultor, especialista, gerente, coordenador, engenheiro — resolves nothing, as
// "Specialist" and "Coordinator" resolve nothing in the main table. "Analista" alone
// heads one Brazilian title in twelve; its field is what names the work.
//
// Both standards are listed where they differ: Brazilian "equipe", "recepcionista",
// "trem", "ônibus", and European "equipa", "rececionista", "restauração" (food service),
// "empilhador" (the forklift). Every entry must be safe on the Spanish, English,
// Italian and French titles that reach the same fallback; the ones that need it are
// argued beside their entry.
var portugueseCategoryTable = []aliasEntry{
	// Band 1: a head and field whose answer is not the field's.

	// Software, before the business and product developer: "desenvolvedor" alone is
	// the software developer on the Brazilian crawl, but the business developer sells
	// and the product developer of a food or fashion house develops recipes.
	{"desenvolvedor de negocios", "sales"},
	{"desenvolvedora de negocios", "sales"},
	{"desenvolvimento de negocios", "sales"},
	{"desenvolvedor de produto", categoryNone},
	{"desenvolvedor de produtos", categoryNone},
	{"desenvolvedora de produtos", categoryNone},
	{"desenvolvedor imobiliario", categoryNone},
	// Human and organisational development is HR, not software.
	{"desenvolvimento humano", "hr"},
	{"desenvolvimento organizacional", "hr"},
	{"treinamento e desenvolvimento", "hr"},
	{"desenvolvedor", "software_engineering"},
	{"desenvolvedora", "software_engineering"},
	{"desenvolvedores", "software_engineering"},
	{"desenvolvimento de software", "software_engineering"},
	{"desenvolvimento de sistemas", "software_engineering"},
	{"engenharia de software", "software_engineering"},
	{"engenharia informatica", "software_engineering"}, // the European degree
	{"engenharia da computacao", "software_engineering"},
	{"ciencia da computacao", "software_engineering"},
	{"analise e desenvolvimento de sistemas", "software_engineering"},
	{"analise de sistemas", "software_engineering"},
	{"analista de ti", "software_engineering"}, // the main table's "IT analyst"
	{"analista de tecnologia", "software_engineering"},
	{"especialista em ti", "software_engineering"},
	{"especialista de ti", "software_engineering"},
	{"tecnologia da informacao", "software_engineering"},
	{"engenheiro de sistemas", "software_engineering"},

	// Data, QA, security, infrastructure, support.
	{"engenheiro de dados", "data_engineering"},
	{"engenheira de dados", "data_engineering"},
	{"engenharia de dados", "data_engineering"},
	{"cientista de dados", "data_science"},
	{"ciencia de dados", "data_science"},
	{"analista de dados", "data_analytics"},
	{"analise de dados", "data_analytics"},
	{"analista de bi", "data_analytics"},
	{"analista de processos", "business_analysis"},
	{"analista de negocios", "business_analysis"}, // the main table's "business analyst"
	{"analista de business", "business_analysis"},
	{"analista de requisitos", "business_analysis"},
	{"inteligencia artificial", "ml_ai"},
	{"engenheiro de ia", "ai_engineering"}, // the main table's "AI engineer"
	{"engenheira de ia", "ai_engineering"},
	{"qualidade de software", "qa"},
	{"analista de testes", "qa"},
	{"automacao de testes", "qa"},
	{"seguranca da informacao", "security"},
	{"seguranca de informacao", "security"},
	{"ciberseguranca", "security"},
	{"seguranca cibernetica", "security"},
	{"analista de infraestrutura", "devops"},
	{"engenheiro de infraestrutura", "devops"},
	{"infraestrutura de ti", "devops"},
	{"analista de redes", "network_engineering"},
	{"engenheiro de redes", "network_engineering"},
	{"telecomunicacoes", "network_engineering"},
	{"analista de suporte", "it_support"},
	{"tecnico de suporte", "it_support"},
	{"tecnica de suporte", "it_support"},
	{"tecnico em informatica", "it_support"},
	{"tecnico de informatica", "it_support"},
	{"suporte tecnico", "it_support"},
	{"suporte de ti", "it_support"},
	{"suporte ti", "it_support"},
	{"suporte aplicacional", "it_support"}, // the European application support desk
	{"tecnico de ti", "it_support"},        // the main table's "IT technician"
	{"tecnico em ti", "it_support"},
	{"service desk", "it_support"},
	{"arquiteto de solucoes", "architecture"},
	{"arquiteta de solucoes", "architecture"},
	{"arquiteto de software", "architecture"},
	{"arquiteto de dados", "architecture"},
	{"arquiteto cloud", "architecture"},
	// The ERP consultant, as the main table files the SAP one.
	{"consultora sap", "solutions_engineering"},
	{"consultor totvs", "solutions_engineering"},
	{"consultora totvs", "solutions_engineering"},
	{"consultor protheus", "solutions_engineering"},

	// Occupational safety, before the guard: "segurança" names all three, and the
	// information-security entries above already claimed the third.
	{"seguranca do trabalho", "occupational_safety"},
	{"seguranca no trabalho", "occupational_safety"},
	{"seguranca de trabalho", "occupational_safety"},
	{"saude e seguranca", "occupational_safety"},
	{"seguranca e saude", "occupational_safety"},
	{"tecnico de seguranca", "occupational_safety"}, // the Brazilian shorthand of the above
	{"tecnica de seguranca", "occupational_safety"},
	{"tecnico em seguranca", "occupational_safety"},
	{"sesmt", "occupational_safety"},
	{"meio ambiente", "occupational_safety"}, // the environment desk of an HSE team

	// Engineering seats: the engineer of a sales team pre-sells; of a plant, runs it.
	// A bare "engenheiro" names no discipline, as a bare "engineer" names none.
	{"engenheiro de vendas", "solutions_engineering"},
	{"engenheira de vendas", "solutions_engineering"},
	{"engenheiro de suporte", "it_support"},
	{"engenheiro de seguranca do trabalho", "occupational_safety"},
	{"engenheiro civil", "industrial_engineering"},
	{"engenheira civil", "industrial_engineering"},
	{"engenharia civil", "industrial_engineering"},
	{"engenheiro de producao", "industrial_engineering"},
	{"engenheira de producao", "industrial_engineering"},
	{"engenharia de producao", "industrial_engineering"},
	{"engenheiro de processos", "industrial_engineering"},
	{"engenheira de processos", "industrial_engineering"},
	{"engenheiro de qualidade", "industrial_engineering"},
	{"engenheira de qualidade", "industrial_engineering"},
	{"engenheiro de manutencao", "industrial_engineering"},
	{"engenheiro de projetos", "industrial_engineering"},
	{"engenheira de projetos", "industrial_engineering"},
	{"engenheiro de planejamento", "industrial_engineering"},
	{"engenheiro de obras", "industrial_engineering"},
	{"engenheiro de obra", "industrial_engineering"},
	{"engenheiro de comissionamento", "industrial_engineering"},
	{"engenheiro de meio ambiente", "industrial_engineering"},
	{"engenheiro de automacao", "industrial_engineering"},
	{"engenheiro eletricista", "industrial_engineering"},
	{"engenheiro eletrico", "industrial_engineering"},
	{"engenharia eletrica", "industrial_engineering"},
	{"engenheiro mecanico", "industrial_engineering"},
	{"engenheira mecanica", "industrial_engineering"},
	{"engenharia mecanica", "industrial_engineering"},
	{"engenheiro quimico", "industrial_engineering"},
	{"engenharia quimica", "industrial_engineering"},
	{"engenheiro ambiental", "industrial_engineering"},
	{"engenharia ambiental", "industrial_engineering"},
	{"engenheiro de campo", "industrial_engineering"},
	{"desenhista", "engineering_design"}, // the drafter
	{"cadista", "engineering_design"},

	// Projects, before a project engineer or analyst is read by its field.
	{"gerente de projetos", "project_management"},
	{"gerente de projeto", "project_management"},
	{"gestor de projetos", "project_management"},
	{"gestor de projeto", "project_management"},
	{"coordenador de projetos", "project_management"},
	{"coordenadora de projetos", "project_management"},
	{"analista de projetos", "project_management"},
	{"gestao de projetos", "project_management"},
	{"analista de produto", "product"},
	{"analista de produtos", "product"},
	{"gerente de produto", "product"},

	// Healthcare. The pharmacy counter is a pharmacy's, as the Spanish table files
	// "auxiliar de farmacia".
	{"enfermagem", "healthcare"}, // the field of the nursing technician and auxiliary
	{"enfermeira", "healthcare"},
	{"farmacia", "healthcare"},
	{"drogaria", "healthcare"},
	{"cirurgiao dentista", "healthcare"},
	{"cirurgia dentista", "healthcare"},
	{"medicina dentaria", "healthcare"}, // the European dentistry
	{"higienista oral", "healthcare"},
	{"saude bucal", "healthcare"}, // the dental assistant
	{"biomedico", "healthcare"},
	{"biomedica", "healthcare"},
	{"fonoaudiologo", "healthcare"},
	{"fonoaudiologa", "healthcare"},
	{"fisioterapia", "healthcare"},
	{"cardiopneumologia", "healthcare"},
	{"nutricao", "healthcare"},
	{"neurofisiologia", "healthcare"},
	{"ortotica", "healthcare"},
	{"ortoprotesia", "healthcare"},
	{"acao medica", "healthcare"}, // the European hospital auxiliary
	{"instrumentador cirurgico", "healthcare"},
	{"instrumentadora cirurgica", "healthcare"},
	{"cuidadores", "healthcare"},

	// Education and social services.
	{"assistente social", "social_services"},
	{"assistentes sociais", "social_services"},
	{"pedagogo", "education"},
	{"pedagoga", "education"},
	{"pedagogico", "education"},
	{"pedagogica", "education"},
	{"auxiliar de classe", "education"},
	{"auxiliar de turma", "education"},
	{"monitor escolar", "education"},
	{"instrutor", "education"},
	{"instrutora", "education"},
	{"formador", "education"}, // the European trainer
	{"formadora", "education"},
	{"coordenador de curso", "education"},
	{"coordenadora de curso", "education"},

	// Public safety. The access controller and the CCTV monitor are a security firm's.
	{"agente de seguranca", "public_safety"},
	{"seguranca patrimonial", "public_safety"},
	{"controlador de acesso", "public_safety"},
	{"controladora de acesso", "public_safety"},
	{"controle de acesso", "public_safety"},
	{"monitor de seguranca", "public_safety"},
	{"operador de monitoramento", "public_safety"},
	{"central de monitoramento", "public_safety"},
	{"bombeiro civil", "public_safety"},
	{"brigadista", "public_safety"},
	{"prevencao de perdas", "public_safety"}, // a shop's loss prevention
	{"fiscal de prevencao", "public_safety"},

	// Hospitality. European "restauração" is the food-service trade, not restoration.
	{"cozinheira", "hospitality"},
	{"cozinheiros", "hospitality"},
	{"cozinha", "hospitality"},
	{"chapeiro", "hospitality"},
	{"chapeira", "hospitality"},
	{"pizzaiolo", "hospitality"},
	{"sushiman", "hospitality"},
	{"confeiteiro", "hospitality"},
	{"confeiteira", "hospitality"},
	{"confeitaria", "hospitality"},
	{"padaria", "hospitality"},
	{"garcom", "hospitality"},
	{"garconete", "hospitality"},
	{"cumim", "hospitality"}, // the waiter's assistant
	{"copeira", "hospitality"},
	{"lanchonete", "hospitality"},
	{"cafeteria", "hospitality"},
	{"sorveteria", "hospitality"},
	{"fast food", "hospitality"},
	{"restauracao", "hospitality"},
	{"hotelaria", "hospitality"},
	{"camareiro", "hospitality"},
	{"governanta", "hospitality"}, // the hotel's housekeeping lead
	{"manobrista", "hospitality"}, // the valet, as the main table files "valet"

	// The trades.
	{"eletrotecnico", "skilled_trades"},
	{"eletromecanico", "skilled_trades"},
	{"tecnico de eletronica", "skilled_trades"},
	{"tecnico em eletronica", "skilled_trades"},
	{"edificacoes", "skilled_trades"},
	{"eletrica", "skilled_trades"}, // "Técnico em Elétrica", "Auxiliar de Elétrica"
	{"serralheiro", "skilled_trades"},
	{"marceneiro", "skilled_trades"},
	{"carpinteiro", "skilled_trades"},
	{"canalizador", "skilled_trades"}, // the European plumber
	{"caldeireiro", "skilled_trades"},
	{"torneiro", "skilled_trades"},
	{"funileiro", "skilled_trades"},
	{"vulcanizador", "skilled_trades"},
	{"borracheiro", "skilled_trades"},
	{"mecanica automotiva", "skilled_trades"},
	{"refrigeracao", "skilled_trades"},
	{"climatizacao", "skilled_trades"},
	{"avac", "skilled_trades"}, // the European HVAC
	{"ar condicionado", "skilled_trades"},
	{"ajudante de obras", "skilled_trades"},
	{"mestre de obras", "skilled_trades"},
	{"encarregado de obras", "skilled_trades"},
	{"encarregado de obra", "skilled_trades"},
	{"fiscal de obras", "skilled_trades"},
	{"fiscal de obra", "skilled_trades"},
	{"construcao civil", "skilled_trades"},
	{"oficial de manutencao", "skilled_trades"},

	// Logistics. The forklift is "empilhadeira" in Brazil and "empilhador" in Portugal.
	{"empilhadeira", "logistics"},
	{"empilhadeirista", "logistics"},
	{"empilhador", "logistics"},
	{"almoxarifado", "logistics"},
	{"almoxarife", "logistics"},
	{"conferente", "logistics"},
	{"estoque", "logistics"},
	{"armazem", "logistics"},
	{"expedicao", "logistics"},
	{"carga e descarga", "logistics"},
	{"ajudante de carga", "logistics"},
	{"separador de mercadorias", "logistics"},
	{"separacao de mercadorias", "logistics"},
	{"recebimento de mercadorias", "logistics"},
	{"operador de triagem", "logistics"},
	{"auxiliar de recebimento", "logistics"},
	{"estacionamento", "logistics"}, // the main table's "parking attendant"
	{"logistico", "logistics"},
	{"motoristas", "logistics"},
	{"comercio exterior", "logistics"},
	{"importacao", "logistics"},
	{"exportacao", "logistics"},
	{"transportes", "logistics"},

	// Retail. "Operador/a de loja" and "de caixa" are already in the main table; the
	// European supermarket writes the department after the head ("Operador/a
	// Hipermercado Reposição"), so the department alone is listed.
	{"operadora de caixa", "retail"},
	{"operadores de caixa", "retail"},
	{"fiscal de caixa", "retail"},
	{"fiscal de loja", "retail"},
	{"reposicao", "retail"},
	{"repositora", "retail"},
	{"acougue", "retail"},
	{"acougueiro", "retail"},
	{"hortifruti", "retail"},
	{"peixaria", "retail"},
	{"empacotador", "retail"},
	{"empacotadora", "retail"},
	{"balconista", "retail"},
	{"atendente de balcao", "retail"},

	// Personal services. "Serviços gerais" is the janitor ("Auxiliar de Serviços Gerais").
	{"servicos gerais", "personal_services"},
	{"limpeza", "personal_services"},
	{"higienizacao", "personal_services"},
	{"limpador", "personal_services"},
	{"faxineira", "personal_services"},
	{"zeladora", "personal_services"},
	{"cabeleireiro", "personal_services"},
	{"cabeleireira", "personal_services"},
	{"cabeleireiros", "personal_services"},
	{"barbeiro", "personal_services"},
	{"barbeiros", "personal_services"},
	{"manicure", "personal_services"},
	{"lavanderia", "personal_services"},
	{"empregada domestica", "personal_services"},
	{"jardinagem", "personal_services"},

	// Finance, legal, people. The lawyer comes first: an "Advogado Tributário" is a tax
	// lawyer, not a tax clerk. "Fiscal" is the TAX desk only after a desk head: a "fiscal
	// de loja" (above) guards a shop and a "fiscal de obras" inspects a building site.
	{"advogado", "legal"},
	{"advogada", "legal"},
	{"advogados", "legal"},
	{"juridico", "legal"},
	{"juridica", "legal"},
	{"analista fiscal", "finance"},
	{"assistente fiscal", "finance"},
	{"auxiliar fiscal", "finance"},
	{"especialista fiscal", "finance"},
	{"departamento fiscal", "finance"},
	{"escrita fiscal", "finance"},
	{"tributario", "finance"},
	{"tributaria", "finance"},
	{"contabil", "finance"},
	{"contabeis", "finance"},
	{"contabilidade", "finance"},
	{"controladoria", "finance"},
	{"tesouraria", "finance"},
	{"faturamento", "finance"},
	{"contas a pagar", "finance"},
	{"contas a receber", "finance"},
	{"cobranca", "finance"},
	{"recuperador de credito", "finance"},
	{"analista de credito", "finance"},
	{"credito e cobranca", "finance"},
	{"custos", "finance"},
	{"auditoria", "finance"},
	{"microcredito", "finance"},
	{"investimentos", "finance"},
	{"financeiro", "finance"},
	{"financeira", "finance"},
	{"financas", "finance"},
	{"direito", "legal"}, // "Estágio em Direito", the law intern
	{"paralegal", "legal"},
	{"recrutamento", "recruiting"},
	{"recrutador", "recruiting"},
	{"recrutadora", "recruiting"},
	{"departamento pessoal", "hr"},
	{"departamento de pessoal", "hr"},
	{"de dp", "hr"}, // the payroll desk, "Assistente de DP"; a bare "DP" is also DP World and a vessel's dynamic positioning
	{"rh", "hr"},
	{"gestao de pessoas", "hr"},
	{"remuneracao", "hr"},
	{"beneficios", "hr"},

	// Customer service and sales.
	{"televendas", "sales"},
	{"teleatendimento", "support"},
	{"atendimento ao cliente", "support"},
	{"atendimento a clientes", "support"},
	{"central de relacionamento", "support"},
	{"relacionamento com o cliente", "support"},
	{"relacionamento ao cliente", "support"},
	{"sac", "support"},
	{"corretor de imoveis", "sales"},
	{"corretora de imoveis", "sales"},
	{"corretor de seguros", "sales"},
	{"corretora de seguros", "sales"},
	{"executivo de contas", "sales"},
	{"executiva de contas", "sales"},
	{"gerente de contas", "sales"},
	{"consultor de negocios", "sales"},
	{"consultora de negocios", "sales"},
	{"gerente de negocios", "sales"},
	{"executivo de negocios", "sales"},
	{"representante de vendas", "sales"},
	{"vendas", "sales"},
	{"venda", "sales"},
	// The shop, after the sales field: a "Consultor de Vendas - Loja" sells, as the
	// main table files the sales associate.
	{"hipermercado", "retail"},
	{"varejo", "retail"},
	{"loja", "retail"},
	{"lojas", "retail"},
	// The service desk field, after the sales one: "Consultor de Atendimento e Vendas" sells.
	{"atendimento", "support"},
	{"midia", "marketing"},
	{"trafego pago", "marketing"},
	{"gestor de trafego", "marketing"},
	{"gestora de trafego", "marketing"},
	{"assessor de imprensa", "marketing"},
	{"assessoria de imprensa", "marketing"},
	{"comunicacao", "marketing"},
	{"suprimentos", "operations"},
	{"sucesso do cliente", "customer_success"},
	{"compradora", "operations"},

	// Band 3: the fields left once every role noun has had its say.
	{"producao audiovisual", "creative"},
	{"producao de conteudo", "creative"},
	{"producao", "manufacturing"},
	{"pcp", "manufacturing"}, // production planning and control
	{"operador industrial", "manufacturing"},
	{"operador de maquinas", "manufacturing"},
	{"operadora de maquinas", "manufacturing"},
	{"inspetor de qualidade", "manufacturing"},
	{"inspetora de qualidade", "manufacturing"},
	{"tecnico de qualidade", "manufacturing"},
	{"tecnica de qualidade", "manufacturing"},
	{"analista de qualidade", "manufacturing"},
	{"controle de qualidade", "manufacturing"},
	{"garantia da qualidade", "manufacturing"},
	{"manutencao", "skilled_trades"}, // the Spanish "mantenimiento"
	{"obras", "skilled_trades"},
	{"mecanica", "skilled_trades"},
	{"seguros", "finance"},      // the insurer's desk; its salespeople were read above
	{"operacoes", "operations"}, // the Spanish "operaciones"
	// The IT field, after every IT desk above has had its say: "Estágio em TI".
	{"de ti", "software_engineering"},
	{"em ti", "software_engineering"},
	{"laboratorio", "science"}, // the main table's "lab technician"
	{"saude", "healthcare"},
	{"hospitalar", "healthcare"},
	{"administracao", "administration"},
	{"auxiliar de escritorio", "administration"},
	{"assistente de diretoria", "administration"},
	{"rececionista", "administration"}, // the European spelling of the main table's alias
	{"recepcao", "administration"},
	{"auxiliar de arquivo", "administration"},
}

// matchPortugueseCategory resolves the category of a lowercased title the Spanish
// table left unresolved. A blind alias answers categoryNone; the caller translates it.
func matchPortugueseCategory(lower string) string {
	return matchOrdered(foldSpanish(lower), portugueseCategoryTable)
}

// portugueseSeniorityTable lists the Portuguese grade aliases, whole-word phrases on the
// folded title, in precedence order. It runs after the Spanish grade table, which
// already reads the trainee grades ("estágio", "estagiário", "aprendiz"), so a trainee
// outranks every grade below, as it does in Spanish.
//
// The accented grades "Sênior" and "Júnior" are the common Brazilian spelling, and the
// main table reads neither: "sênior" does not contain "senior". The folded title does.
//
// "Coordenador" and "chefe" lead, as the Spanish "jefe" does: in a Brazilian career
// ladder the coordinator sits above the specialist and below the manager. "Supervisor"
// is left alone — it is spelled the same in Spanish and English, whose tables state no
// grade for it, and this table also reads their titles. "Gerente" states no grade, as
// "manager" states none, and a plain "diretor" none, as a plain "director" none; only
// the general, executive and financial directors are the board.
var portugueseSeniorityTable = []aliasEntry{
	{"diretor geral", "c_level"},
	{"diretora geral", "c_level"},
	{"diretor executivo", "c_level"},
	{"diretora executiva", "c_level"},
	{"diretor presidente", "c_level"},
	{"diretora presidente", "c_level"},
	{"diretor financeiro", "c_level"},
	{"diretora financeira", "c_level"},
	{"coordenador", "lead"},
	{"coordenadora", "lead"},
	{"coordenadores", "lead"},
	{"chefe", "lead"},
	{"chefes", "lead"},
	// The grades in the main table's order: "Analista Pleno/Sênior" keeps its upper
	// bound, as "Mid/Senior" does.
	{"senior", "senior"},
	{"pleno", "middle"},
	{"plena", "middle"},
	{"junior", "junior"},
}

// assistantOfPortugueseLead is the assistant OF a lead or a director — "Assistente de
// Coordenação", "Auxiliar da Chefia", "Secretária da Diretoria Executiva", "Subchefe" —
// which states no grade of its own. It is cut before the Portuguese grade match.
var assistantOfPortugueseLead = regexp.MustCompile(`\b(?:(?:assistente|auxiliar|adjunt[oa]|secretari[oa])\s+(?:(?:d[oa]s?|a[os]?|ao)\s+)?(?:chef[ei]a?|coordenador[a]?|diretor[a]?\s+(?:geral|executiv[oa]|financeir[oa]|presidente)|diretoria)|subchefe|subcoordenador[a]?)\b`)

// brazilianGradeLadder reads the grade a Brazilian career ladder writes after the head
// and field: "Analista Fiscal PL", "Analista Financeiro II", "Técnico em Automação I".
// "PL" (pleno) and the Roman rungs say nothing on their own — "PL" is Poland and
// PL/SQL, "II" a product generation — so they count only after a Portuguese desk head
// within the same clause. I is the junior rung, II the middle and III the senior, the
// ladder the Brazilian employers in the crawl share.
var brazilianGradeLadder = regexp.MustCompile(`\b(?:analista|assistente|auxiliar|tecnic[oa]|especialista|desenvolvedor[a]?|engenheir[oa]|programador[a]?|consultor[a]?|operador[a]?|advogad[oa]|comprador[a]?|contador[a]?|projetista|atendente|agente|executiv[oa]|coordenador[a]?)\b[^|()\-–,:]*?[\s/](pl|iii|ii|i)(?:\s*$|\s*[|()\-–,/:]|\s+(?:remoto|presencial|hibrido|home|temporario|\d))`)

// plSQL is Oracle's language, whose "PL" is not the grade: "Analista de PL/SQL".
var plSQL = regexp.MustCompile(`\bpl[\s/-]?sql\b`)

// brazilianRung maps a brazilianGradeLadder capture to its grade.
var brazilianRung = map[string]string{"pl": "middle", "i": "junior", "ii": "middle", "iii": "senior"}

// matchPortugueseSeniority resolves the grade of a lowercased title the other tables
// left unresolved.
func matchPortugueseSeniority(lower string) string {
	folded := assistantOfPortugueseLead.ReplaceAllString(foldSpanish(lower), " ")
	if s := matchOrdered(folded, portugueseSeniorityTable); s != "" {
		return s
	}
	if m := brazilianGradeLadder.FindStringSubmatch(plSQL.ReplaceAllString(folded, "plsql")); m != nil {
		return brazilianRung[m[1]]
	}
	return ""
}

package classify

// coreCategoryTable11 is part 11 of 17 of coreCategoryTable; the parts concatenate in order.
var coreCategoryTable11 = []aliasEntry{
	{"инспектор по кадрам", "hr"},
	{"hr-менеджер", "hr"},
	{"office coordinator", "administration"},
	{"заведующий хозяйством", "administration"},
	{"заведующая хозяйством", "administration"},
	{"завхоз", "administration"},
	{"buyer", "operations"},
	{"buyers", "operations"},
	{"закупкам", "operations"},
	{"госзакупкам", "operations"},
	{"менеджер по мто", "operations"},
	// The rest of the Russian "менеджер ..." compounds the bare "менеджер" used to file.
	{"менеджер проектов", "project_management"},
	{"менеджер проекта", "project_management"},
	{"pr-менеджер", "marketing"},
	{"продаже", "sales"},
	{"ключевыми клиентами", "sales"},
	{"территориальный менеджер", "sales"},
	// Sales: the territory manager is a field-sales seat ("Territory Business Manager"
	// in pharma). The area and district manager is NOT — in this catalogue it is a
	// retail or restaurant chain's multi-unit operator — so it has no category and
	// reaches search through role_type.
	{"territory manager", "sales"},
	{"territory business manager", "sales"},
	// IT: the manager of a company's IT estate, beside "it administrator".
	{"it manager", "devops"},
	// Personal services. A groundskeeper is personal and facility services here, so the
	// Russian green-space worker is filed with it, and the culture organiser is a
	// recreation worker (SOC 39-9032).
	{"рабочий зеленого хозяйства", "personal_services"},
	{"рабочий зелёного хозяйства", "personal_services"},
	{"культорганизатор", "personal_services"},

	// The functional managers: a manager title whose craft is a business function the
	// table already files. Each was the residue of the retired fall-through in volume,
	// and each names its function as surely as "Finance Manager" always did.
	{"portfolio manager", "finance"},
	{"risk manager", "finance"},
	{"credit manager", "finance"},
	{"it audit manager", "security"},
	{"audit manager", "finance"},
	{"internal audit", "finance"},
	{"investor relations", "finance"},
	{"underwriting", "finance"},
	{"f&i manager", "finance"},
	{"finance and insurance manager", "finance"},
	{"finance & insurance manager", "finance"},
	{"people & culture", "hr"},
	{"people and culture", "hr"},
	{"commercial manager", "sales"},
	{"partnerships manager", "sales"},
	{"partnership manager", "sales"},
	{"partner manager", "sales"},
	{"bid manager", "sales"},
	{"proposal manager", "sales"},
	{"partner success manager", "customer_success"},
	{"success manager", "customer_success"},
	{"implementation manager", "customer_success"},
	{"growth manager", "marketing"},
	{"affiliate manager", "marketing"},
	{"communications manager", "marketing"},
	{"category manager", "operations"},
	{"sourcing manager", "operations"},
	{"strategic sourcing", "operations"},
	{"commodity manager", "operations"},
	{"operation manager", "operations"},
	{"facility manager", "operations"},
	{"administrative manager", "administration"},
	{"administration manager", "administration"},
	{"training manager", "hr"},
	{"compensation manager", "hr"},
	{"employee relations", "hr"},
	{"labor relations", "hr"},
	{"labour relations", "hr"},
	{"industrial relations", "hr"},
	{"human resource", "hr"},
	{"analytics manager", "data_analytics"},
	{"release manager", "devops"},
	{"bim manager", "engineering_design"},
	{"continuous improvement", "industrial_engineering"},
	{"commissioning manager", "industrial_engineering"},
	{"cost manager", "industrial_engineering"},
	{"environmental manager", "occupational_safety"},
	{"eh&s", "occupational_safety"},
	{"quality control manager", "manufacturing"},
	{"project controls", "project_management"},
	{"preconstruction", "skilled_trades"},
	{"distribution manager", "logistics"},
	{"inventory manager", "logistics"},
	{"materials manager", "logistics"},
	{"material manager", "logistics"},
	{"care manager", "healthcare"},
	{"clinical manager", "healthcare"},
	{"clinic manager", "healthcare"},
	{"pharmacy manager", "healthcare"},
	{"laboratory manager", "science"},
	{"lab manager", "science"},
	{"deli manager", "retail"},
	{"bakery manager", "retail"},
	{"meat manager", "retail"},
	{"produce manager", "retail"},
	{"perishable manager", "retail"},
	{"perishables manager", "retail"},
	{"beverage manager", "hospitality"},
	{"cafe manager", "hospitality"},
	{"café manager", "hospitality"},
	{"reservations manager", "hospitality"},
	{"spa manager", "personal_services"},
	{"fitness manager", "personal_services"},
	// Film and television: the post-production manager is the media craft, not the plant.
	{"post production", "creative"},
	{"post-production", "creative"},
	{"менеджер (в торговле)", "retail"},
	{"менеджер в торговле", "retail"},
	{"в коммерческой деятельности", "sales"},
	{"категорийный менеджер", "operations"},

	// PUBLIC SAFETY, the rest of the family (see its first block, above logistics). These
	// words name the employer as often as the job, so they reach only a title that
	// named no craft at all.
	{"police", "public_safety"},
	{"sheriff", "public_safety"},
	{"correctional officer", "public_safety"},
	{"corrections officer", "public_safety"},
	{"correctional sergeant", "public_safety"},
	{"correctional lieutenant", "public_safety"},
	{"correctional captain", "public_safety"},
	{"corrections sergeant", "public_safety"},
	{"corrections lieutenant", "public_safety"},
	{"corrections captain", "public_safety"},
	{"corrections deputy", "public_safety"},
	{"detention officer", "public_safety"},
	{"detention deputy", "public_safety"},
	{"jailer", "public_safety"},
	{"bailiff", "public_safety"},
	{"deputy marshal", "public_safety"},
	{"fire marshal", "public_safety"},
	{"patrol officer", "public_safety"},
	// Retail loss prevention is store security (SOC 33-9021/33-9032). The data-loss
	// prevention discipline is information security and is pinned first.
	{"data loss prevention", "security"},
	{"loss prevention", "public_safety"},
	{"asset protection", "public_safety"},
	{"охранник", "public_safety"},
	{"охранница", "public_safety"},
	{"сторож", "public_safety"},
	{"вохр", "public_safety"},
	{"полицейский", "public_safety"},
	{"сотрудник полиции", "public_safety"},
	{"участковый уполномоченный", "public_safety"},
	{"инспектор дпс", "public_safety"},
	{"следователь", "public_safety"},
	{"дознаватель", "public_safety"},
	{"пожарный", "public_safety"},
	{"пожарной части", "public_safety"},
	{"пожарной команды", "public_safety"},
	{"спасатель", "public_safety"},
	{"sicherheitsmitarbeiter", "public_safety"},
	{"sicherheitsmitarbeiterin", "public_safety"},
	{"sicherheitsdienst", "public_safety"},
	{"schutz und sicherheit", "public_safety"},
	{"objektschutz", "public_safety"},
	{"werkschutz", "public_safety"},
	{"polizist", "public_safety"},
	{"polizistin", "public_safety"},
	{"polizeibeamter", "public_safety"},
	{"polizeibeamtin", "public_safety"},
	{"polizeivollzugsbeamter", "public_safety"},
	{"polizeivollzugsbeamtin", "public_safety"},
	{"feuerwehrmann", "public_safety"},
	{"feuerwehrfrau", "public_safety"},
	{"berufsfeuerwehr", "public_safety"},
	{"werkfeuerwehr", "public_safety"},
	{"brandmeister", "public_safety"},
	// Brazilian Portuguese: "vigilante" is the security guard; "porteiro", the doorman,
	// stays with the personal services.
	{"vigilante", "public_safety"},

	// THE REMOTE TAIL. Measured 2026-10-19: of 462k open remote postings, 183k resolved
	// no category, and a posting without one never reaches search. Every spelling below
	// was read against that corpus and then against a 3% sample of the whole open
	// catalogue; together they place ~26k of the remote postings and ~107k overall. What
	// is left is long and flat, or names no craft at all ("General Application",
	// "Talent Pool", the "work from home opportunity" funnels), and stays unresolved.
	//
	// Left out on purpose: "AI Trainer" is the domain it trains in ("CAD Engineer - AI
	// Trainer"), not a craft of its own; "Product Engineer", "Tech Lead" and "Engineering
	// Lead" split between software and the plant floor, the same reason "Product Engineer"
	// is pinned elsewhere; bare "counselor" is financial, camp and admissions as often as
	// clinical; bare "journeyman" is also a US federal pay grade.
	//
	// Selling: real estate, insurance and the benefits funnels, which sell policies.
	{"real estate agent", "sales"},
	{"real estate showing agent", "sales"},
	{"real estate acquisition", "sales"},
	{"estate agent", "sales"},
	{"agent immobilier", "sales"},
	{"conseiller immobilier", "sales"},
	{"chasseur immobilier", "sales"},
	{"insurance agents", "sales"},
	{"insurance representative", "sales"},
	{"life insurance", "sales"},
	{"final expense", "sales"},
	{"insurance broker", "sales"},
	{"benefits representative", "sales"},
	{"client benefits", "sales"},
	{"appointment setter", "sales"},
	// Lending and billing.
	{"financial services professional", "finance"},
	{"financial adviser", "finance"},
	{"loan originator", "finance"},
	{"mortgage", "finance"},
	{"billing specialist", "finance"},
	{"premium auditor", "finance"},
	{"finance director", "finance"},
	// Travel booking is the hospitality trade, home-based or not.
	{"travel agent", "hospitality"},
	{"travel advisor", "hospitality"},
	{"travel consultant", "hospitality"},
	{"travel specialist", "hospitality"},
	{"travel planner", "hospitality"},
	{"travel coordinator", "hospitality"},
	{"reservation specialist", "hospitality"},
	// Clinical. The spa therapist is the treatment room, pinned ahead of the bare word.
	// No "case manager": it is a nurse as often as a social worker (see the suite).
	{"spa therapist", "personal_services"},
	{"beauty therapist", "personal_services"},
	{"behavior analyst", "healthcare"},
	{"bcba", "healthcare"},
	{"therapist", "healthcare"},
	{"mental health", "healthcare"},
	{"speech language", "healthcare"},
	{"medical science liaison", "healthcare"},
	{"clinical research", "healthcare"},
	{"clinical trial", "healthcare"},
	{"psychologist", "healthcare"},
	{"psychotherapist", "healthcare"},
	{"professional counselor", "healthcare"},
	{"clinical counselor", "healthcare"},
	{"dietitian", "healthcare"},
	{"optometric assistant", "healthcare"},
	{"attorneys", "legal"},
	// Interpreting sits with translation (see "translator"). A parts interpreter reads a
	// parts catalogue at a dealership counter.
	{"parts interpreter", "retail"},
	{"interpreter", "technical_writing"},
	{"interpreters", "technical_writing"},
	// Marketing and creative.
	{"creative director", "creative"},
	{"creative strategist", "marketing"},
	{"digital marketer", "marketing"},
	{"ppc specialist", "marketing"},
	{"ppc manager", "marketing"},
	{"content editor", "marketing"},
	// IT spellings the families above never wrote down.
	{"react engineer", "frontend"},
	{"laravel developer", "backend"},
	{"chief technology officer", "software_engineering"},
	{"product analyst", "data_analytics"},
	// The vehicle trades, including the Spanish mechanic.
	{"mecánico", "skilled_trades"},
	{"técnico de motores", "skilled_trades"},
	{"tire technician", "skilled_trades"},
	{"rv technician", "skilled_trades"},
	{"small engine technician", "skilled_trades"},
	// The rest. "Profesor" is the Spanish tutor and teacher, the bulk of the thin
	// Latin American WhatJobs accounts.
	{"supply chain", "logistics"},
	{"transaction coordinator", "administration"},
	{"intake specialist", "administration"},
	{"intake coordinator", "administration"},
	{"english trainer", "education"},
	{"profesor", "education"},

	// CENTRAL AND EASTERN EUROPE (freehire#3513). Measured 2026-10-24 over the open,
	// canonical postings located in si/pl/ua/bg/hu/cz/hr/rs/sk/ba: 43% resolved no
	// category, and almost every Polish, Czech, Slovak, Slovenian, Croatian, Serbian,
	// Ukrainian and Bulgarian title among them was one this table had no word for —
	// a pharmaceutical employer's company page showed 12 of its 82 open postings.
	//
	// Declared at the END, behind every block above, so a title any earlier alias
	// resolves keeps exactly the category it had: these lines only reach a title that
	// named nothing else. Each alias files the role the way its English twin is filed
	// above ("Medical Representative" with the pharmaceutical sales rep, the
	// storekeeper with the warehouse, the controller with finance), and where the
	// English twin is deliberately unresolved — the bare quality, compliance, tax and
	// audit specialists, the generic "specialist"/"associate" nouns (specjalista,
	// фахівець, sodelavec, suradnik, munkatárs), interns and student work, the talent
	// pools — the native one is left unresolved too.
	//
	// The matcher has no morphology, and these languages inflect for case and gender,
	// so every alias is a surface form the catalogue carries: the masculine and the
	// feminine of a role noun, and the CASE a domain noun takes in a title. A Polish
	// title names its desk in the genitive ("Specjalista ds. sprzedaży", "Kierownik
	// Działu Logistyki"), a Slovenian or Croatian one in the locative ("v Marketingu",
	// "u proizvodnji"); the nominative of the same word would match neither.
	//
	// Within the block the role nouns come first and the domain nouns last, because
	// the table resolves in declaration order, not title order: "Magazynier - obsługa
	// produkcji" is a warehouse worker, not production, and "Lekarz medycyny pracy" a
	// doctor whatever else the title says.

	// English: the pharmaceutical field-sales rep, as "Pharmaceutical Sales
	// Representative" already resolves. The plural is its own word to this matcher.
	{"medical representative", "sales"},
	{"medical representatives", "sales"},
	{"pharmaceutical representative", "sales"},
	{"pharmaceutical representatives", "sales"},
	{"pharmacy representative", "sales"},
	{"pharmacy representatives", "sales"},
	// The same seat in each language. Hungarian calls it the doctor's and the
	// pharmacy's "visitor".
	{"przedstawiciel medyczny", "sales"},
	{"przedstawicielka medyczna", "sales"},
	{"przedstawiciel farmaceutyczny", "sales"},
	{"przedstawicielka farmaceutyczna", "sales"},
	{"przedstawiciel naukowy", "sales"},
	{"przedstawicielka naukowa", "sales"},
	{"przedstawiciel handlowy", "sales"},
	{"przedstawicielka handlowa", "sales"},
	{"medicínský reprezentant", "sales"},
	{"medicínská reprezentantka", "sales"},
	{"obchodní zástupce", "sales"},
	{"regionální zástupce", "sales"},
	{"obchodný zástupca", "sales"},
	{"medicinski predstavnik", "sales"},
	{"prodajni predstavnik", "sales"},
	{"prodajni savjetnik", "sales"},
	{"prodajna savjetnica", "sales"},
	{"broker ubezpieczeń", "sales"},
	{"broker ubezpieczeniowy", "sales"},
	{"brokerka ubezpieczeniowa", "sales"},
	{"orvoslátogató", "sales"},
	{"patikalátogató", "sales"},
	{"területi képviselő", "sales"},
	{"медичний представник", "sales"},
	{"медична представниця", "sales"},
	{"медицински представител", "sales"},
	{"медицинский представитель", "sales"},
	{"търговски представител", "sales"},
	{"представител медицински", "sales"},

	// Software. The machine programmers are pinned first: CNC, PLC, CAM, CAD and robot
	// programming is not software. The CNC programmer is a production occupation (SOC
	// 51-9162) and is filed on the floor; the rest name no category. The bare programmer
	// nouns themselves are declared at the very END of this block (see there).
	{"programista cnc", "manufacturing"},
}

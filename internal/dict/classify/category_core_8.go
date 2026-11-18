package classify

// coreCategoryTable8 is part 8 of 17 of coreCategoryTable; the parts concatenate in order.
var coreCategoryTable8 = []aliasEntry{
	{"assistente executiva", "administration"},
	{"assistente executivo", "administration"},
	{"assistente administrativa", "administration"},
	{"assistente administrativo", "administration"},
	{"auxiliar administrativo", "administration"},
	{"recepcionista", "administration"},
	{"assistante de direction", "administration"},
	{"assistant de direction", "administration"},
	{"assistante administrative", "administration"},
	{"assistant administratif", "administration"},
	{"file clerk", "administration"},

	// PERSONAL AND FACILITY SERVICES.
	{"master stylist", "personal_services"},
	{"stylist", "personal_services"},
	{"barber", "personal_services"},
	{"esthetician", "personal_services"},
	{"aesthetician", "personal_services"},
	// The lifeguard and the security guard are protective service (SOC 33-9092 and 33-9032) and resolve `public_safety`:
	// the lifeguard in the public_safety block above, the guard in the security block beside the information-security
	// spellings they must not be confused with.
	{"janitor", "personal_services"},
	{"custodian", "personal_services"},
	{"housekeeper", "personal_services"},
	{"housekeeping", "personal_services"},

	// Industrial engineering: the seats a factory, plant, utility or field-service organisation staffs. They had nowhere to
	// be filed: `engineering_design` means draughting, and a Quality Engineer is not a draughtsman.
	// The IT lookalikes come first: each names a discipline of its own that no alias above would catch, and the bare
	// "engineer" at the bottom of this block would take them. `field application engineer` is the semiconductor pre-sales
	// title and goes to the customer-facing category, not to the plant.
	{"it engineer", "software_engineering"},
	{"database engineer", "devops"},
	{"business intelligence engineer", "data_analytics"},
	{"electronics engineer", "hardware"},
	{"field application engineer", "solutions_engineering"},

	// Then the seats themselves.
	{"project engineer", "industrial_engineering"},
	{"quality engineer", "industrial_engineering"},
	{"supplier quality engineer", "industrial_engineering"},
	{"process engineer", "industrial_engineering"},
	{"manufacturing engineer", "industrial_engineering"},
	{"production engineer", "industrial_engineering"},
	{"maintenance engineer", "industrial_engineering"},
	{"controls engineer", "industrial_engineering"},
	{"control engineer", "industrial_engineering"},
	{"instrumentation engineer", "industrial_engineering"},
	// Both were left unresolved by the IT wave. Here they read industrial: "Automation Engineer" sits beside "Controls
	// Engineer", and a QA automation engineer's title already carries "QA", which resolves far above.
	{"automation engineer", "industrial_engineering"},
	{"application engineer", "industrial_engineering"},
	{"applications engineer", "industrial_engineering"},
	// No "reliability engineer": the suite pins it to no category because mechanical reliability and site reliability
	// share the phrase, and only "site" tells them apart.
	{"commissioning engineer", "industrial_engineering"},
	{"validation engineer", "industrial_engineering"},
	{"industrial engineer", "industrial_engineering"},
	{"field service engineer", "industrial_engineering"},
	{"service engineer", "industrial_engineering"},
	{"field engineer", "industrial_engineering"},
	{"site engineer", "industrial_engineering"},
	{"plant engineer", "industrial_engineering"},
	{"facilities engineer", "industrial_engineering"},
	{"building engineer", "industrial_engineering"},
	// "Safety Engineer" is the HSE profession, not the plant seat around it; SOC's reported-title list for 19-5011 names it.
	// Its position here does not matter: the matcher takes contiguous phrases, so "software safety engineer" never
	// contained "software engineer" and resolves here wherever this sits.
	{"safety engineer", "occupational_safety"},
	{"environmental engineer", "industrial_engineering"},
	{"geotechnical engineer", "industrial_engineering"},
	{"planning engineer", "industrial_engineering"},
	{"resident engineer", "industrial_engineering"},
	// The physical engineering disciplines. nontech.go named them as non-technical, and with no category to file them
	// under the ingest filter turned every one away. A software role in the same domain does not contain the phrase
	// ("Electrical Systems Engineer" is pinned to no category far above), and a hardware one resolves through "hardware" first.
	{"electrical engineer", "industrial_engineering"},
	{"mechanical engineer", "industrial_engineering"},
	{"civil engineer", "industrial_engineering"},
	{"structural engineer", "industrial_engineering"},
	{"quantity surveyor", "industrial_engineering"},

	// The non-technical title dictionary's own vocabulary (nontech.go), filed. Those terms said a posting was not technical
	// but not what it WAS, and a posting placed nowhere is one ingest refuses and search excludes. Each term names the job
	// outright, so it earns the category it names.
	// Declared HERE, after every technical block and the industrial seats, so a title any alias above resolves keeps its
	// category: "HVAC Controls Engineer" stays industrial and "DevOps Engineer (HVAC IoT Platform)" stays devops.
	// Healthcare.
	{"behavior technician", "healthcare"},
	{"rbt", "healthcare"},
	// The ABA therapy line staff the "rbt" above names, spelled without the acronym ("Child Autism Support Professional").
	// They must not reach the bare "support", declared at the end of the table.
	{"autism support", "healthcare"},
	{"behavioral health", "healthcare"},
	{"language pathologist", "healthcare"},
	{"therapy assistant", "healthcare"},
	{"care aide", "healthcare"},
	{"care assistant", "healthcare"},
	{"nursing", "healthcare"},
	{"home health", "healthcare"},
	{"hospice", "healthcare"},
	{"veterinary", "healthcare"},
	{"dentist", "healthcare"},
	{"pharmacist", "healthcare"},
	{"paramedic", "healthcare"},
	{"medical scribe", "healthcare"},
	{"medical technologist", "healthcare"},
	{"patient care", "healthcare"},
	{"patient access", "healthcare"},
	{"surgical technologist", "healthcare"},
	{"scrub tech", "healthcare"},
	{"respiratory therapist", "healthcare"},
	{"speech therapist", "healthcare"},
	{"mental health technician", "healthcare"},
	{"mental health counselor", "healthcare"},
	{"mental health therapist", "healthcare"},
	{"mental health associate", "healthcare"},
	{"clinical research coordinator", "healthcare"},
	{"clinical research associate", "healthcare"},
	{"primary care provider", "healthcare"},
	{"advanced practice provider", "healthcare"},
	{"family medicine", "healthcare"},
	{"medical director", "healthcare"},
	{"care coordinator", "healthcare"},
	// The service advisor is the customer-facing desk of a car dealership, a bank branch or a member service: `support` in
	// every case. "Customer Service Advisor" resolves there through "customer service" far above.
	{"service advisor", "support"},
	// Trades and the production floor.
	{"ironworker", "skilled_trades"},
	{"iron worker", "skilled_trades"},
	{"laborer", "skilled_trades"},
	{"general labor", "skilled_trades"},
	{"pipefitter", "skilled_trades"},
	{"stone mason", "skilled_trades"},
	{"brick mason", "skilled_trades"},
	{"bricklayer", "skilled_trades"},
	{"roofer", "skilled_trades"},
	{"hvac", "skilled_trades"},
	{"boilermaker", "skilled_trades"},
	{"sheet metal", "skilled_trades"},
	{"automotive service", "skilled_trades"},
	{"auto body", "skilled_trades"},
	{"oil change", "skilled_trades"},
	{"lube technician", "skilled_trades"},
	// The production floor is `manufacturing` (SOC 51), not a trade. These four keep the precedence they were declared
	// with; the rest of that family is in the manufacturing block at the end of this table.
	{"production operator", "manufacturing"},
	{"production associate", "manufacturing"},
	{"production supervisor", "manufacturing"},
	{"machine operator", "manufacturing"},
	{"maintenance supervisor", "skilled_trades"},
	{"maintenance worker", "skilled_trades"},
	{"equipment operator", "skilled_trades"},
	{"heavy equipment", "skilled_trades"},
	// Hospitality.
	{"food service", "hospitality"},
	{"banquet", "hospitality"},
	{"waiter", "hospitality"},
	{"waitress", "hospitality"},
	{"concierge", "hospitality"},
	{"valet", "hospitality"},
	{"crew member", "hospitality"},
	{"restaurant team", "hospitality"},
	{"room attendant", "hospitality"},
	{"guest service", "hospitality"},
	{"dietary aide", "hospitality"},
	// Retail.
	{"stocker", "retail"},
	{"store associate", "retail"},
	{"store clerk", "retail"},
	{"car rental", "retail"},
	{"merchandise associate", "retail"},
	{"retail keyholder", "retail"},
	{"beauty advisor", "retail"},
	{"store mgr", "retail"},
	// Logistics and transport.
	{"forklift", "logistics"},
	{"order picker", "logistics"},
	{"picker", "logistics"},
	{"packer", "logistics"},
	{"material handler", "logistics"},
	{"package handler", "logistics"},
	{"flight attendant", "logistics"},
	{"parking attendant", "logistics"},
	// Education, childcare and group fitness (the instructors sit beside the swim and
	// chess instructors above).
	{"pilates instructor", "education"},
	{"yoga instructor", "education"},
	{"fitness instructor", "education"},
	{"teaching assistant", "education"},
	{"preschool", "education"},
	{"childcare", "education"},
	{"child care", "education"},
	{"daycare", "education"},
	{"camp counselor", "education"},
	{"paraprofessional", "education"},
	{"adjunct faculty", "education"},
	// Personal care and facilities.
	{"caretaker", "personal_services"},
	{"personal trainer", "personal_services"},
	{"cosmetologist", "personal_services"},
	{"hairstylist", "personal_services"},
	{"manicurist", "personal_services"},
	{"nail technician", "personal_services"},
	{"massage therapist", "personal_services"},
	{"janitorial", "personal_services"},
	{"cleaner", "personal_services"},
	{"custodial", "personal_services"},
	{"groundskeeper", "personal_services"},
	{"environmental services", "personal_services"},
	// Retail banking and insurance. An insurance agent sells policies (SOC 41-3021).
	{"bank teller", "finance"},
	{"teller", "finance"},
	{"loan officer", "finance"},
	{"underwriter", "finance"},
	{"claims adjuster", "finance"},
	{"personal banker", "finance"},
	{"relationship banker", "finance"},
	{"associate banker", "finance"},
	{"financial advisor", "finance"},
	{"insurance agent", "sales"},

	// The Brazilian-Portuguese half of the same vocabulary.
	{"operador de caixa", "retail"},
	{"operador de loja", "retail"},
	{"atendente de loja", "retail"},
	{"repositor", "retail"},
	{"frentista", "retail"},
	{"açougueiro", "retail"},
	{"estoquista", "logistics"},
	{"motorista", "logistics"},
	{"motoboy", "logistics"},
	{"entregador", "logistics"},
	{"cozinheiro", "hospitality"},
	{"auxiliar de cozinha", "hospitality"},
	{"padeiro", "hospitality"},
	{"garçom", "hospitality"},
	{"copeiro", "hospitality"},
	{"camareira", "hospitality"},
	{"pedreiro", "skilled_trades"},
	{"pintor", "skilled_trades"},
	{"soldador", "skilled_trades"},
	{"eletricista", "skilled_trades"},
	{"encanador", "skilled_trades"},
	{"mecânico", "skilled_trades"},
	{"servente", "skilled_trades"},
	{"ajudante geral", "skilled_trades"},
	{"técnico de enfermagem", "healthcare"},
	{"auxiliar de enfermagem", "healthcare"},
	{"enfermeiro", "healthcare"},
	{"cuidador", "healthcare"},
	{"auxiliar de limpeza", "personal_services"},
	{"faxineiro", "personal_services"},
	{"zelador", "personal_services"},
	{"porteiro", "personal_services"},
	{"jardineiro", "personal_services"},
	{"babá", "personal_services"},

	// 1С (the RU enterprise/ERP dev platform) resolves last so a more specific role word wins first ("Аналитик 1С" →
	// data_analytics, "Тестировщик 1С" → qa); a title whose only signal is 1С ("Программист 1С", "1С-разработчик") reads
	// as backend, server-side enterprise development. Bare tokens so any separator matches.
	{"1c", "backend"},
	// "Аналитик 1С" is a systems/business analyst on the 1C platform, not a data one ("Системный аналитик 1С"). It needs
	// its own entry now that the bare "аналитик" fall-through is gone: without it the title drops to the 1C rule below and
	// reads as backend.
	{"аналитик 1с", "business_analysis"},
	{"1с аналитик", "business_analysis"},
	{"1с", "backend"},

	// Russian software and administration, declared AFTER 1С: "Программист 1С" must stay backend, and a bare
	// "программист" declared earlier would take it.
	// Bare tokens, unlike the English entries: Russian puts the technology FIRST ("Java-разработчик",
	// "Инженер-программист") and a hyphen is a word boundary, so no qualified alias can stand in for the bare one. The more
	// specific role words (backend, аналитик, тестировщик) are declared far above, so they still win.
	{"сетевой администратор", "network_engineering"},
	{"системный администратор", "system_administration"},
	{"администратор баз данных", "devops"},
	{"программист", "software_engineering"},
	{"разработчик", "software_engineering"},

	// German administration/technician/tester/developer fused compounds. German joins role words into one word with no
	// separator, so the spaced English aliases cannot reach them and the alias must be the fused form itself. A hyphen or
	// space BEFORE the compound is still a boundary, so each bare alias already reaches an "IT-"/"IT "-prefixed title.
	{"systemadministrator", "system_administration"},
	{"netzwerkadministrator", "network_engineering"},
}

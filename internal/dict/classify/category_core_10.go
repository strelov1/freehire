package classify

// coreCategoryTable10 is part 10 of 17 of coreCategoryTable; the parts concatenate in order.
var coreCategoryTable10 = []aliasEntry{
	{"clinical laboratory scientist", "healthcare"},
	{"clinical lab scientist", "healthcare"},
	{"histotechnician", "healthcare"},
	{"histotechnologist", "healthcare"},
	{"histology technician", "healthcare"},
	{"med lab tech", "healthcare"},
	{"med lab scientist", "healthcare"},
	{"clinical lab assistant", "healthcare"},
	{"clinical laboratory assistant", "healthcare"},
	{"pathology lab assistant", "healthcare"},
	{"pathology laboratory assistant", "healthcare"},
	{"cath lab", "healthcare"},
	{"ep lab", "healthcare"},
	{"sleep lab", "healthcare"},
	// The school or campus computer lab is IT support, filed with "it technician".
	{"computer lab technician", "it_support"},
	{"computer laboratory technician", "it_support"},
	{"computer lab assistant", "it_support"},
	// There is deliberately NO bare "scientist" and NO bare "research associate".
	// "Research Scientist", "Applied Scientist", "Computer Scientist" and "Principal
	// NLP Scientist" are software and machine-learning roles as often as laboratory
	// ones, and "Research Associate" is an equity analyst at a bank; none of them
	// resolves a category today and a bare alias would file them all as lab science.
	// Only spellings that name the laboratory or the discipline are here. A "QA Lab
	// Technician" never reaches them: "qa" resolves far above.
	{"lab technician", "science"},
	{"laboratory technician", "science"},
	{"lab tech", "science"},
	{"lab assistant", "science"},
	{"laboratory assistant", "science"},
	{"lab analyst", "science"},
	{"laboratory analyst", "science"},
	{"laboratory associate", "science"},
	{"lab scientist", "science"},
	{"laboratory scientist", "science"},
	{"research technician", "science"},
	{"associate scientist", "science"},
	{"bench scientist", "science"},
	{"environmental scientist", "science"},
	{"food scientist", "science"},
	{"soil scientist", "science"},
	{"forensic scientist", "science"},
	{"materials scientist", "science"},
	{"chemist", "science"},
	{"chemists", "science"},
	{"biochemist", "science"},
	{"geochemist", "science"},
	{"biologist", "science"},
	{"microbiologist", "science"},
	{"geologist", "science"},
	{"hydrogeologist", "science"},
	{"geoscientist", "science"},
	{"geophysicist", "science"},
	{"hydrologist", "science"},
	{"physicist", "science"},
	{"toxicologist", "science"},
	{"epidemiologist", "science"},
	{"ecologist", "science"},
	{"agronomist", "science"},
	{"meteorologist", "science"},
	// Environmental monitoring, not the plant's quality line: declared above the
	// manufacturing "quality technician".
	{"water quality technician", "science"},
	{"лаборант", "science"},
	{"лаборантка", "science"},
	{"химик", "science"},
	{"биолог", "science"},
	{"микробиолог", "science"},
	{"геолог", "science"},
	{"chemiker", "science"},
	{"chemikerin", "science"},
	{"laborant", "science"},
	{"laborantin", "science"},
	{"chimiste", "science"},
	{"technicien de laboratoire", "science"},
	{"technicienne de laboratoire", "science"},
	{"técnico de laboratorio", "science"},
	{"técnico de laboratório", "science"},
	{"auxiliar de laboratório", "science"},
	{"laboratorista", "science"},

	// Manufacturing (SOC 51): the production floor. Engineers stay in
	// `industrial_engineering` ("production engineer" resolves there far above), and
	// the construction and repair trades — the heavy-equipment operator included —
	// stay in `skilled_trades`. "Сборщик заказов" is a warehouse picker and resolves
	// `logistics` above, so the bare "сборщик" here only reaches the assemblers.
	{"production worker", "manufacturing"},
	{"production technician", "manufacturing"},
	{"production lead", "manufacturing"},
	{"production line", "manufacturing"},
	{"line operator", "manufacturing"},
	{"line worker", "manufacturing"},
	{"manufacturing technician", "manufacturing"},
	{"manufacturing associate", "manufacturing"},
	{"manufacturing operator", "manufacturing"},
	{"manufacturing worker", "manufacturing"},
	{"manufacturing supervisor", "manufacturing"},
	{"process operator", "manufacturing"},
	{"plant operator", "manufacturing"},
	{"press operator", "manufacturing"},
	{"assembler", "manufacturing"},
	{"assemblers", "manufacturing"},
	{"assembly technician", "manufacturing"},
	{"assembly operator", "manufacturing"},
	{"assembly worker", "manufacturing"},
	{"assembly associate", "manufacturing"},
	{"assembly line", "manufacturing"},
	{"cnc operator", "manufacturing"},
	{"packaging operator", "manufacturing"},
	{"packaging technician", "manufacturing"},
	{"quality inspector", "manufacturing"},
	{"quality control inspector", "manufacturing"},
	{"qc inspector", "manufacturing"},
	{"quality technician", "manufacturing"},
	{"quality control technician", "manufacturing"},
	{"qc technician", "manufacturing"},
	{"fabricator", "manufacturing"},
	{"fabricators", "manufacturing"},
	{"оператор производственной линии", "manufacturing"},
	{"оператор линии", "manufacturing"},
	{"оператор производства", "manufacturing"},
	{"оператор станка", "manufacturing"},
	{"оператор станков", "manufacturing"},
	{"оператор чпу", "manufacturing"},
	{"рабочий на производство", "manufacturing"},
	{"рабочий производства", "manufacturing"},
	{"рабочий в цех", "manufacturing"},
	{"контролер отк", "manufacturing"},
	{"контролёр отк", "manufacturing"},
	{"сборщик", "manufacturing"},
	{"сборщица", "manufacturing"},
	{"produktionsmitarbeiter", "manufacturing"},
	{"produktionsmitarbeiterin", "manufacturing"},
	{"produktionshelfer", "manufacturing"},
	{"produktionshelferin", "manufacturing"},
	{"maschinenbediener", "manufacturing"},
	{"maschinenbedienerin", "manufacturing"},
	{"maschinenführer", "manufacturing"},
	{"maschinenführerin", "manufacturing"},
	{"anlagenführer", "manufacturing"},
	{"anlagenführerin", "manufacturing"},
	{"opérateur de production", "manufacturing"},
	{"opératrice de production", "manufacturing"},
	{"agent de production", "manufacturing"},
	{"operador de producción", "manufacturing"},
	{"operario de producción", "manufacturing"},
	{"trabajador de producción", "manufacturing"},
	{"operador de produção", "manufacturing"},
	{"auxiliar de produção", "manufacturing"},
	{"ajudante de produção", "manufacturing"},
	{"operador de máquina", "manufacturing"},
	{"operador de máquinas", "manufacturing"},
	// Russian production floor, the trudvsem tail: a seamstress (SOC 51-6031).
	{"швея", "manufacturing"},
	{"швеи", "manufacturing"},

	// MANAGERS THAT NAME THEIR CRAFT, and the second pass over the uncategorised tail.
	//
	// There is no `management` category any more (see the note where the bare "manager"
	// fall-through used to be). A manager title resolves the craft it manages — a
	// Restaurant Manager is hospitality, a Plant Manager manufacturing, a Tax Manager
	// finance — and the `role_type` facet answers whether it manages people. The generic
	// titles that name no craft ("General Manager", "Assistant Manager", "District
	// Manager") are deliberately NOT here: no category is true for them, and roletype
	// marks them people managers, which is what keeps them searchable.
	//
	// Declared at the very end, like every block since the IT waves: a title any alias
	// above resolves keeps its category. Every spelling was measured against a 2% sample
	// of the open catalogue before it was written down.
	//
	// Hospitality.
	{"restaurant manager", "hospitality"},
	{"restaurant general manager", "hospitality"},
	{"kitchen manager", "hospitality"},
	{"food and beverage manager", "hospitality"},
	{"food & beverage manager", "hospitality"},
	{"f&b manager", "hospitality"},
	{"catering manager", "hospitality"},
	{"bar manager", "hospitality"},
	{"hotel manager", "hospitality"},
	{"laundry attendant", "hospitality"},
	{"laundry aide", "hospitality"},
	{"laundry worker", "hospitality"},
	// The porter. Bare, it is the hotel, restaurant and kitchen porter, which is the
	// volume; the surname and the beer do not occur as titles. The day porter cleans a
	// building, and the dealership porter moves cars, so both are pinned first.
	{"day porter", "personal_services"},
	{"service porter", "logistics"},
	{"lot porter", "logistics"},
	{"dealership porter", "logistics"},
	{"car porter", "logistics"},
	{"porter", "hospitality"},
	{"porters", "hospitality"},
	{"горничная", "hospitality"},
	{"горничной", "hospitality"},
	{"мойщик посуды", "hospitality"},
	{"мойщица посуды", "hospitality"},
	{"буфетчик", "hospitality"},
	{"буфетчица", "hospitality"},
	{"кастелянша", "hospitality"},
	{"гардеробщик", "hospitality"},
	{"гардеробщица", "hospitality"},
	// Retail. "Store Manager" is also a non-tech title term, so without this entry the
	// ingest filter would refuse it once the bare "manager" stopped placing it.
	{"store manager", "retail"},
	{"shop manager", "retail"},
	{"retail manager", "retail"},
	{"retail assistant manager", "retail"},
	// German retail. The gender forms are separate words; "/in", "*in" and "(m/w/d)"
	// are word boundaries and need nothing. An apprenticeship names the role it trains
	// for ("Ausbildung Kaufmann im Einzelhandel"), so it resolves through the role;
	// "Ausbildung" itself is never an alias, nor "Initiativbewerbung", which names no
	// role at all. A Filialleiter runs a store of a chain — at Aldi, Lidl and dm, the
	// boards that carry the title — and is filed with the store, as "store manager" is.
	{"verkäufer", "retail"},
	{"verkäuferin", "retail"},
	{"fachverkäufer", "retail"},
	{"fachverkäuferin", "retail"},
	{"kaufmann im einzelhandel", "retail"},
	{"kauffrau im einzelhandel", "retail"},
	{"kaufleute im einzelhandel", "retail"},
	{"einzelhandelskaufmann", "retail"},
	{"einzelhandelskauffrau", "retail"},
	{"kassierer", "retail"},
	{"kassiererin", "retail"},
	{"filialleiter", "retail"},
	{"filialleiterin", "retail"},
	{"filialleitung", "retail"},
	{"handelsfachwirt", "retail"},
	{"handelsfachwirtin", "retail"},
	{"mitarbeiter verkauf", "retail"},
	{"mitarbeiterin verkauf", "retail"},
	{"verkaufsmitarbeiter", "retail"},
	{"verkaufsmitarbeiterin", "retail"},
	// Manufacturing. A live event's production manager runs a show, not a line, and is
	// filed with the events the marketing block already holds.
	{"event production manager", "marketing"},
	{"events production manager", "marketing"},
	{"plant manager", "manufacturing"},
	{"production manager", "manufacturing"},
	{"manufacturing manager", "manufacturing"},
	// Quality management runs the plant's quality system (ISO 9001), the function a
	// "Quality Engineer" already sits in, so it is filed there rather than on the floor.
	{"quality manager", "industrial_engineering"},
	// The cost estimator is the US twin of the quantity surveyor, which this table files
	// as industrial engineering. The collision and body-shop estimator prices a car
	// repair and stays with the auto-body trade.
	{"collision estimator", "skilled_trades"},
	{"collision repair estimator", "skilled_trades"},
	{"body shop estimator", "skilled_trades"},
	{"estimator", "industrial_engineering"},
	{"estimators", "industrial_engineering"},
	// Construction. The construction and site manager and the superintendent run the
	// trades on one site, so they are filed with the trades, as one construction market.
	// A school district's superintendent is its chief executive and is education; a
	// ship's marine superintendent is shipping.
	{"construction manager", "skilled_trades"},
	{"site manager", "skilled_trades"},
	{"maintenance manager", "skilled_trades"},
	{"superintendent of schools", "education"},
	{"school superintendent", "education"},
	{"assistant superintendent of", "education"},
	{"assistant superintendent for", "education"},
	{"marine superintendent", "logistics"},
	{"superintendent", "skilled_trades"},
	{"superintendents", "skilled_trades"},
	{"котельной", "skilled_trades"}, // "оператор (газовой) котельной", the boiler room
	{"монтер пути", "skilled_trades"},
	{"монтёр пути", "skilled_trades"},
	{"дорожный рабочий", "skilled_trades"},
	// Logistics.
	{"supply chain manager", "logistics"},
	{"fleet manager", "logistics"},
	{"logistics manager", "logistics"},
	{"warehouse manager", "logistics"},
	{"air traffic controller", "logistics"},
	{"inventory controller", "logistics"},
	{"stock controller", "logistics"},
	{"materials controller", "logistics"},
	{"логист", "logistics"},
	{"логистике", "logistics"},
	// "Диспетчер" files with the English "dispatcher": the transport and production
	// dispatcher is the volume. The emergency dispatcher ("ЕДДС", the unified duty
	// dispatch service, and "пожарной связи", the fire line) is public safety, and a
	// university's scheduling clerk is administration; both are pinned first.
	{"еддс", "public_safety"},
	{"пожарной связи", "public_safety"},
	{"диспетчер факультета", "administration"},
	{"диспетчер", "logistics"},
	// Finance. "Controller" bare is the financial controller in every language this
	// catalogue carries ("Assistant Controller", "Plant Controller", "Controller
	// (m/w/d)"); the controllers of documents, stock and air traffic are pinned above
	// and below by what they control.
	{"tax manager", "finance"},
	{"tax senior manager", "finance"},
	{"relationship manager", "finance"},
	{"document controller", "administration"},
	{"quality controller", "manufacturing"},
	{"process controller", "manufacturing"},
	{"operations controller", "operations"},
	{"flight controller", categoryNone},
	{"plc controller", "industrial_engineering"},
	{"controller", "finance"},
	{"controllers", "finance"},
	{"экономист", "finance"},
	// Healthcare. A practice manager runs a medical, dental or veterinary practice.
	{"practice manager", "healthcare"},
	{"medical case manager", "healthcare"},
	{"psychiatrist", "healthcare"},
	{"psychiatric", "healthcare"},
	{"фармацевт", "healthcare"},
	{"провизор", "healthcare"},
	{"рентгенолаборант", "healthcare"},
	{"лечебной физкультуре", "healthcare"},
	{"медицинский статистик", "healthcare"},
	// A clinical psychologist treats patients and is healthcare. The unqualified
	// psychologist is filed with social work, as ISCO-08 does (2634 beside 2635 in
	// "social and religious professionals"): in the Russian public sector the title is
	// the social-services and education psychologist far more often than the clinic's.
	{"медицинский психолог", "healthcare"},
	{"клинический психолог", "healthcare"},
	{"психолог", "social_services"},
	// Education. SOC files the librarian with education (25-4022, Educational
	// Instruction and Library Occupations).
	{"тьютор", "education"},
	{"музыкальный руководитель", "education"},
	{"мастер производственного обучения", "education"},
	{"библиотекарь", "education"},
	{"librarian", "education"},
	// Science: the agronomist is a soil and plant scientist (SOC 19-1013), as the English
	// "agronomist" already is, and the zootechnician an animal scientist (19-1011). The
	// livestock worker, "животновод", is a farmworker (SOC 45) and has no category here.
	{"агроном", "science"},
	{"зоотехник", "science"},
	// HR, administration, operations.
	{"специалист по кадрам", "hr"},
}

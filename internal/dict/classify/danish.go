package classify

// Danish titles glue their role into the LAST morpheme of a compound, as German and
// Dutch do: a lager|medarbejder, a social|pædagog, a software|udvikler, a
// salgs|assistent|elev. Measured 2026-10-26 over the 62,102 open postings of the three
// Danish boards (jobindex, jobnet, jobdanmark), 88% resolved no category — the
// commonest unresolved words were Danish heads: pædagog, sygeplejerske,
// sundhedsassistent, tømrer, montør, rengøringsassistent.
//
// The tables follow german.go exactly (matchHeads): an alias written "-pædagog" is a
// head and matches any word that ends in it, an alias without the dash is a whole-word
// phrase, a head resolves per word and precedence is table order. They run after the
// main, German and Dutch tables have resolved nothing, so no existing answer changes —
// and every head here must also be safe on any other title that reaches the same
// fallback. Most heads carry an æ, ø or å or a spelling no other catalogue language
// shares; the ones that do not are argued beside their entry.
//
// Danish plurals add -e/-er/-r ("pædagoger", "tømrere", "sygeplejersker"); a head
// matches only the word's tail, so the plural forms that appear in live titles are
// listed as heads of their own.
//
// A generic head with no category of its own — "medarbejder" (employee), "assistent",
// "medhjælper" (helper), "konsulent", "tekniker", "ingeniør", "rådgiver",
// "sagsbehandler" (a caseworker of social benefits, building permits or tax alike) — is
// deliberately absent: only the compound that names the trade resolves
// ("lagermedarbejder", "køkkenmedhjælper", "salgskonsulent"). Shift words are never
// heads: "aftenvagt" and "nattevagt" are the evening and night SHIFT, not a guard.

// danishCategoryTable lists the Danish category aliases in precedence order.
var danishCategoryTable = []aliasEntry{
	// Vetoes: words whose tail is a head below and whose role is not that head's.
	{"-kirketjener", categoryNone}, // the verger, not a waiter

	// The machine shop, before the IT block: a "CNC-programmør" or a
	// "Laseroperatør/-programmør" programs a machining centre, not software, and a
	// PLC programmer automates a plant.
	{"-cnc-programmør", "manufacturing"},
	{"cnc programmør", "manufacturing"},
	{"-fræseoperatør", "manufacturing"},
	{"-laseroperatør", "manufacturing"},
	{"-plc-programmør", "industrial_engineering"},
	{"-logistikoperatør", "logistics"},
	{"-logistikoperatører", "logistics"},

	// IT. The developer heads are listed one by one: a bare "-udvikler" would also
	// read the business developer ("forretningsudvikler"), the property developer
	// ("projektudvikler") and the course developer.
	{"-softwareudvikler", "software_engineering"},
	{"-softwareudviklere", "software_engineering"},
	{"-systemudvikler", "software_engineering"},
	{"-systemudviklere", "software_engineering"},
	{"-webudvikler", "software_engineering"},
	{"-appudvikler", "software_engineering"},
	{"-it-udvikler", "software_engineering"},
	{"-programmør", "software_engineering"},
	{"-softwareingeniør", "software_engineering"},
	{"-systemadministrator", "devops"}, // the main table files the system administrator here
	{"-netværksadministrator", "network_engineering"},
	{"-dataanalytiker", "data_analytics"},
	{"-it-arkitekt", "architecture"},
	{"-løsningsarkitekt", "architecture"},
	{"-softwarearkitekt", "architecture"},
	{"-cybersikkerhed", "security"},
	{"-it-sikkerhed", "security"},
	{"-informationssikkerhed", "security"},
	{"-it-supporter", "it_support"},
	{"-it-support", "it_support"},
	{"-it-medarbejder", "it_support"},

	// Engineering. The engineer is named by the discipline in front of "-ingeniør";
	// a bare "ingeniør" says no more than the English "Engineer", which the main table
	// leaves alone too. The construction architect draws the building.
	{"-maskiningeniør", "industrial_engineering"},
	{"-elingeniør", "industrial_engineering"},
	{"-el-ingeniør", "industrial_engineering"},
	{"-konstruktionsingeniør", "industrial_engineering"},
	{"-bygningsingeniør", "industrial_engineering"},
	{"-anlægsingeniør", "industrial_engineering"},
	{"-vejingeniør", "industrial_engineering"},
	{"-procesingeniør", "industrial_engineering"},
	{"-produktionsingeniør", "industrial_engineering"},
	{"-kvalitetsingeniør", "industrial_engineering"},
	{"-bygningskonstruktør", "engineering_design"},
	{"-bygningskonstruktører", "engineering_design"},

	// Occupational safety, before every trade head.
	{"-arbejdsmiljøkoordinator", "occupational_safety"},
	{"-arbejdsmiljøkonsulent", "occupational_safety"},
	{"-arbejdsmiljørådgiver", "occupational_safety"},

	// Healthcare. SOSU is the social and health care training; SSA and SSH are its
	// assistant and helper grades, and the abbreviations are how the care homes post
	// them ("SSA og SSH til aftenvagt"). They are matched only beside a Danish word,
	// never bare: "SSH" is also the shell protocol and "SSA" a region ("Order to Cash
	// Leader SSA (Sub Sahara Africa)") on titles that reach this same fallback.
	{"-sygeplejerske", "healthcare"},
	{"-sygeplejersker", "healthcare"},
	{"-sundhedsassistent", "healthcare"},
	{"-sundhedsassistenter", "healthcare"},
	{"-sundhedshjælper", "healthcare"},
	{"-sundhedshjælpere", "healthcare"},
	{"sosu", "healthcare"},
	{"-sosu-assistent", "healthcare"},
	{"-sosu-hjælper", "healthcare"},
	{"ssa og", "healthcare"},
	{"ssa eller", "healthcare"},
	{"ssa til", "healthcare"},
	{"ssa søges", "healthcare"},
	{"ssa søger", "healthcare"},
	{"og ssa", "healthcare"},
	{"eller ssa", "healthcare"},
	{"som ssa", "healthcare"},
	{"en ssa", "healthcare"},
	{"ssa'er", "healthcare"},
	{"ssa-elev", "healthcare"},
	{"ssh og", "healthcare"},
	{"ssh eller", "healthcare"},
	{"ssh til", "healthcare"},
	{"ssh søges", "healthcare"},
	{"ssh søger", "healthcare"},
	{"og ssh", "healthcare"},
	{"eller ssh", "healthcare"},
	{"som ssh", "healthcare"},
	{"en ssh", "healthcare"},
	{"ssh'er", "healthcare"},
	{"ssh-elev", "healthcare"},
	{"ssa i", "healthcare"},
	{"ssa med", "healthcare"},
	{"ssa dagvagt", "healthcare"},
	{"ssa aftenvagt", "healthcare"},
	{"ssa nattevagt", "healthcare"},
	{"ssa-kollega", "healthcare"},
	{"ssa´er", "healthcare"},
	{"ssa’er", "healthcare"},
	{"nye ssa", "healthcare"},
	{"fast ssa", "healthcare"},
	{"uddannet ssa", "healthcare"},
	{"udekørende ssa", "healthcare"},
	{"ufaglært/ssa", "healthcare"},
	{"ssh i", "healthcare"},
	{"ssh med", "healthcare"},
	{"ssh dagvagt", "healthcare"},
	{"ssh aftenvagt", "healthcare"},
	{"ssh nattevagt", "healthcare"},
	{"ssh-kollega", "healthcare"},
	{"ssh´er", "healthcare"},
	{"ssh’er", "healthcare"},
	{"nye ssh", "healthcare"},
	{"fast ssh", "healthcare"},
	{"uddannet ssh", "healthcare"},
	{"udekørende ssh", "healthcare"},
	{"ufaglært/ssh", "healthcare"},
	{"ssa/ssh", "healthcare"},
	{"ssh/ssa", "healthcare"},
	{"-hjemmehjælper", "healthcare"},
	{"-læge", "healthcare"}, // Læge, Overlæge, Speciallæge, Tandlæge, Dyrlæge
	{"-læger", "healthcare"},
	{"-terapeut", "healthcare"}, // Fysioterapeut, Ergoterapeut
	{"-terapeuter", "healthcare"},
	{"-psykolog", "healthcare"},
	{"-psykologer", "healthcare"},
	{"-jordemoder", "healthcare"},
	{"-radiograf", "healthcare"},
	{"-farmaceut", "healthcare"},
	{"-farmakonom", "healthcare"},
	{"-klinikassistent", "healthcare"}, // the dental nurse
	{"-klinikassistenter", "healthcare"},
	{"-tandplejer", "healthcare"},
	{"-ambulancebehandler", "healthcare"},
	{"-paramediciner", "healthcare"},

	// Social services, before education: a "socialpædagog" works in disability and
	// residential care. The companion ("ledsager") accompanies a disabled citizen;
	// the pastor is filed here as the main table files him.
	{"-socialpædagog", "social_services"},
	{"-socialpædagoger", "social_services"},
	{"-socialrådgiver", "social_services"},
	{"-socialrådgivere", "social_services"},
	{"-handicaphjælper", "social_services"},
	{"-handicaphjælpere", "social_services"},
	{"-ledsager", "social_services"},
	{"-ledsagere", "social_services"},
	{"-støttekontaktperson", "social_services"},
	{"-familiebehandler", "social_services"},
	{"-misbrugsbehandler", "social_services"},
	{"-præst", "social_services"}, // Sognepræst

	// Education: the pedagogue and the pedagogue's assistant staff the day care.
	{"-pædagog", "education"},
	{"-pædagoger", "education"},
	{"-pædagogmedhjælper", "education"},
	{"-pædagogmedhjælpere", "education"},
	{"pædagogisk assistent", "education"},
	{"pædagogisk leder", "education"},
	{"-dagplejer", "education"},
	{"-dagplejere", "education"},
	{"-lærer", "education"}, // Lærer, Matematiklærer, Dansklærer
	{"-lærere", "education"},
	{"-underviser", "education"},
	{"-undervisere", "education"},
	{"-lektor", "education"},
	{"-bibliotekar", "education"},

	// Public safety. The guard is the "vægter", never "-vagt" (a shift).
	{"-vægter", "public_safety"},
	{"-sikkerhedsvagt", "public_safety"},
	{"-fængselsbetjent", "public_safety"},
	{"-politibetjent", "public_safety"},
	{"-brandmand", "public_safety"},

	// Personal services: cleaning, the caretaker, the hairdresser.
	{"-rengøringsassistent", "personal_services"},
	{"-rengøringsassistenter", "personal_services"},
	{"-rengøringsmedarbejder", "personal_services"},
	{"-rengøringsmedarbejdere", "personal_services"},
	{"rengøring", "personal_services"},
	{"-husassistent", "personal_services"},
	{"-vinduespudser", "personal_services"},
	{"-vicevært", "personal_services"},
	{"-pedel", "personal_services"},
	{"-ejendomsfunktionær", "personal_services"},
	{"-ejendomsservicetekniker", "personal_services"}, // before the service technician
	{"-frisør", "personal_services"},
	{"-frisører", "personal_services"},
	{"-massør", "personal_services"},

	// Construction and the trades. The site manager ("byggeleder") is filed with the
	// trades, as the Dutch "uitvoerder" is. The landscaper lays paving in Denmark; a
	// bare "gartner" is left alone, because it is also a company name.
	{"-tømrer", "skilled_trades"},
	{"-tømrere", "skilled_trades"},
	{"-tømrersvend", "skilled_trades"},
	{"-murer", "skilled_trades"},
	{"-murere", "skilled_trades"},
	{"-murersvend", "skilled_trades"},
	// The painter by compound only: as a bare tail "-maler" reads German adjectives
	// ("normaler", "optimaler").
	{"-bygningsmaler", "skilled_trades"},
	{"-bygningsmalere", "skilled_trades"},
	{"-malersvend", "skilled_trades"},
	{"-smed", "skilled_trades"}, // Klejnsmed, Maskinsmed
	{"-smede", "skilled_trades"},
	{"-svejser", "skilled_trades"},
	{"-svejsere", "skilled_trades"},
	{"-snedker", "skilled_trades"},
	{"-snedkere", "skilled_trades"},
	{"-rørlægger", "skilled_trades"},
	{"-rørlæggere", "skilled_trades"},
	{"-blikkenslager", "skilled_trades"},
	// The Swedish HVAC design engineer and consultant share the "VVS" abbreviation
	// and draw the system rather than fit it.
	{"vvs-ingenjör", categoryNone},
	{"vvs-ingenjörer", categoryNone},
	{"vvs-konstruktör", categoryNone},
	{"vvs-konsult", categoryNone},
	{"vvs", "skilled_trades"},
	{"-vvs-montør", "skilled_trades"},
	{"-montør", "skilled_trades"}, // Montør, Dækmontør, Elmontør
	{"-montører", "skilled_trades"},
	{"-elektrikere", "skilled_trades"},
	{"-elinstallatør", "skilled_trades"},
	{"-brolægger", "skilled_trades"},
	{"-betonarbejder", "skilled_trades"},
	{"-betonarbejdere", "skilled_trades"},
	{"-tagdækker", "skilled_trades"},
	{"-glarmester", "skilled_trades"},
	{"-mekaniker", "skilled_trades"}, // Automekaniker, Lastvognsmekaniker
	{"-mekanikere", "skilled_trades"},
	{"-autolakerer", "skilled_trades"},
	{"-karrosseritekniker", "skilled_trades"},
	{"-servicetekniker", "skilled_trades"},
	{"-serviceteknikere", "skilled_trades"},
	{"-håndværker", "skilled_trades"},
	{"-håndværkere", "skilled_trades"},
	{"-maskinfører", "skilled_trades"}, // the construction plant operator
	{"-kranfører", "skilled_trades"},
	{"-anlægsgartner", "skilled_trades"},
	{"-anlægsgartnere", "skilled_trades"},
	{"-byggeleder", "skilled_trades"},

	// Manufacturing. The industrial technician is the CNC machinist of a factory floor.
	{"-produktionsmedarbejder", "manufacturing"},
	{"-produktionsmedarbejdere", "manufacturing"},
	{"-produktionsarbejder", "manufacturing"},
	{"-specialarbejder", "manufacturing"},
	{"-specialarbejdere", "manufacturing"},
	{"-maskinarbejder", "manufacturing"},
	{"-operatør", "manufacturing"}, // Operatør, Procesoperatør, CNC-operatør
	{"-operatører", "manufacturing"},
	{"-industritekniker", "manufacturing"},
	{"-industriteknikere", "manufacturing"},
	{"-produktionstekniker", "manufacturing"},
	{"-kvalitetstekniker", "manufacturing"},

	// Logistics. "Lager" is the warehouse, as in German.
	{"-lagermedarbejder", "logistics"},
	{"-lagermedarbejdere", "logistics"},
	{"-lagerarbejder", "logistics"},
	{"-lagerassistent", "logistics"},
	{"lager", "logistics"},
	{"-logistikmedarbejder", "logistics"},
	{"-chauffør", "logistics"}, // Chauffør, Lastbilchauffør, Buschauffør
	{"-chauffører", "logistics"},
	{"-truckfører", "logistics"},
	{"-postbud", "logistics"},
	{"-flyttemand", "logistics"},

	// Hospitality. The nutrition assistant cooks for a hospital or a canteen.
	{"-kokke", "hospitality"},
	{"-tjener", "hospitality"}, // Tjener, Overtjener
	{"-tjenere", "hospitality"},
	{"-serveringsmedarbejder", "hospitality"},
	{"-køkkenmedhjælper", "hospitality"},
	{"-køkkenmedhjælpere", "hospitality"},
	{"-køkkenassistent", "hospitality"},
	{"-køkkenmedarbejder", "hospitality"},
	{"-køkkenchef", "hospitality"},
	{"-opvasker", "hospitality"},
	{"-restaurantchef", "hospitality"},
	{"-cafémedarbejder", "hospitality"},
	{"-ernæringsassistent", "hospitality"},
	{"-ernæringsassistenter", "hospitality"},

	// Retail. "Salgsassistent" is the shop assistant on these boards (Netto, Føtex,
	// Bilka), not the sales desk's assistant.
	{"-salgsassistent", "retail"},
	{"-salgsassistenter", "retail"},
	{"-salgsassistentelev", "retail"},
	{"-butiksassistent", "retail"},
	{"-butiksmedarbejder", "retail"},
	{"-butiksmedhjælper", "retail"},
	{"-butikschef", "retail"},
	{"-butikssouschef", "retail"},
	{"-ekspedient", "retail"},
	{"-kassemedarbejder", "retail"},
	{"-mercher", "retail"}, // the merchandiser

	// Finance.
	{"-bogholder", "finance"},
	{"-bogholdere", "finance"},
	{"-regnskabsmedarbejder", "finance"},
	{"-regnskabschef", "finance"},
	{"-økonomimedarbejder", "finance"},
	{"-økonomichef", "finance"},
	{"-økonomiassistent", "finance"},
	{"-økonomikonsulent", "finance"},
	{"-revisor", "finance"},
	{"-kreditmedarbejder", "finance"},

	// Law.
	{"-advokat", "legal"},
	{"-advokatfuldmægtig", "legal"},
	{"juridisk medarbejder", "legal"},

	// People.
	{"-hr-medarbejder", "hr"},
	{"-hr-konsulent", "hr"},
	{"-rekrutteringskonsulent", "recruiting"},

	// Science: the laboratory.
	{"-bioanalytiker", "science"},
	{"-bioanalytikere", "science"},
	{"-laboratorietekniker", "science"},

	// The office. The medical secretary is filed here, as the main table files her.
	{"-sekretær", "administration"},
	{"-sekretærer", "administration"},
	{"-kontorassistent", "administration"},
	{"-kontorelev", "administration"},
	{"-kontormedarbejder", "administration"},
	{"administrativ medarbejder", "administration"},

	// Operations: the operations manager and the buyer.
	{"-driftsleder", "operations"},
	{"-driftschef", "operations"},
	{"-indkøber", "operations"},

	// Projects.
	{"-projektleder", "project_management"},
	{"-projektledere", "project_management"},
	{"-projektkoordinator", "project_management"},

	// Sales and customer service.
	{"-kundeservicemedarbejder", "support"},
	{"-kundeservicemedarbejdere", "support"},
	{"kundeservice", "support"},
	{"-sælger", "sales"},
	{"-sælgere", "sales"},
	{"-salgskonsulent", "sales"},
	{"-salgschef", "sales"},
	{"-salgsleder", "sales"},
	{"-ejendomsmægler", "sales"},
}

// danishSeniorityTable lists the Danish grade aliases in precedence order.
var danishSeniorityTable = []aliasEntry{
	// The trainee grades outrank the lead heads, as in German: a "Projektlederelev"
	// is an apprentice. "Elev" is the apprentice of the vocational training, attached
	// to its trade ("Salgsassistentelev", "Kontorelev").
	{"-elev", "intern"},
	{"-elever", "intern"},
	{"-lærling", "intern"},
	{"-lærlinge", "intern"},
	{"-praktikant", "intern"},
	{"-praktikanter", "intern"},
	{"praktik", "intern"},
	{"-studentermedhjælper", "intern"},
	{"-studentermedhjælpere", "intern"},
	{"-studentermedarbejder", "intern"},
	{"studiejob", "intern"},
	{"-vejleder", categoryNone}, // a counsellor or a trainer, whose word merely ends in "leder"
	{"-vejledere", categoryNone},
	{"administrerende direktør", "c_level"},
	{"-overlæge", "senior"},
	{"ledende", "lead"},
	{"-leder", "lead"}, // Teamleder, Afdelingsleder, Projektleder, as the German "-leiter"
	{"-ledere", "lead"},
	{"-værkfører", "lead"},
	{"-formand", "lead"}, // the site foreman
}

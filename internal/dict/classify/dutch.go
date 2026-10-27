package classify

import (
	"regexp"
	"strings"
)

// Dutch titles, like German ones, name their role in the LAST morpheme of a compound:
// a servicedesk|medewerker, a wijk|verpleegkundige, an applicatie|beheerder. Where the
// qualifier is a separate word ("Functioneel Beheerder", "Business Analist") it is a
// phrase the main tables never learned, because the head is spelled the Dutch way.
// Measured 2026-10-25 over the 42,176 open Dutch postings of eures, recruitee and
// whatjobs-nl, 45% resolved no category, and the commonest unresolved words were
// Dutch heads — medewerker, analist, beheerder, marketeer, adviseur, begeleider.
//
// The tables follow german.go exactly (matchHeads): an alias written "-beheerder" is
// a head and matches any word that ends in it, an alias without the dash is a
// whole-word phrase, a head resolves per word and precedence is table order. They run
// after the main AND the German tables have resolved nothing, so no existing answer
// changes — and every head here must therefore also be safe on an English, French or
// German title that reaches the same fallback ("Bangkok" ends in "-kok", "Stage
// Manager" is not an intern).
//
// A generic head with no category of its own — "medewerker" (employee), "adviseur",
// "coördinator", "specialist" — is deliberately absent: only the compound or phrase
// that names the trade resolves ("magazijnmedewerker", "HR adviseur").

// dutchCategoryTable lists the Dutch category aliases in precedence order.
var dutchCategoryTable = []aliasEntry{
	// Vetoes: words whose tail is a head below and whose role is not that head's.
	{"-bangkok", categoryNone},
	{"-beleidsontwikkelaar", categoryNone}, // a policy developer
	{"-projectontwikkelaar", categoryNone}, // a property developer
	{"-vastgoedontwikkelaar", categoryNone},
	{"-gebiedsontwikkelaar", categoryNone},
	{"-planontwikkelaar", categoryNone},
	{"-conceptontwikkelaar", categoryNone},
	{"-organisatieontwikkelaar", categoryNone},
	{"-onderwijsontwikkelaar", categoryNone},
	{"-leerontwikkelaar", categoryNone},
	{"-vervoerontwikkelaar", categoryNone},
	{"-spoorontwikkelaar", categoryNone},
	{"-productontwikkelaar", categoryNone},
	{"-modelontwikkelaar", categoryNone},
	{"-talentontwikkelaar", categoryNone},
	{"-businessontwikkelaar", categoryNone},
	{"business ontwikkelaar", categoryNone},
	{"-filmprogrammeur", categoryNone}, // a venue's programme curator, not a coder
	{"-podiumprogrammeur", categoryNone},
	{"-theaterprogrammeur", categoryNone},
	{"-muziekprogrammeur", categoryNone},
	{"live programmeur", categoryNone},
	{"-landschapsarchitect", categoryNone},
	{"-interieurarchitect", categoryNone},
	{"-procesbegeleider", categoryNone}, // a facilitator, not a care worker
	{"-projectbegeleider", categoryNone},
	{"-uitvoeringsbegeleider", categoryNone},
	{"-reisbegeleider", categoryNone},
	{"-klantbegeleider", categoryNone},
	{"-praktijkbegeleider", categoryNone},
	{"-fitnessbegeleider", categoryNone},

	// Security: the information-security officer, before the guard's "-beveiliger".
	{"-informatiebeveiliger", "security"},
	{"-informatiebeveiligingsspecialist", "security"},
	{"-informatiebeveiligingsconsultant", "security"},
	{"-informatiebeveiligingsadviseur", "security"},
	{"-informatiebeveiligingsfunctionaris", "security"},
	{"informatiebeveiliging", "security"},

	// IT. The functional administrator sits between the business and the system; the
	// main table files the analyst who does the same work under business_analysis.
	{"functioneel beheerder", "business_analysis"},
	{"functioneel applicatiebeheerder", "business_analysis"},
	{"-functioneelbeheerder", "business_analysis"},
	{"business analist", "business_analysis"},
	{"informatie analist", "business_analysis"},
	{"-businessanalist", "business_analysis"},
	{"-bedrijfsanalist", "business_analysis"},
	{"-informatieanalist", "business_analysis"},
	{"-procesanalist", "business_analysis"},
	{"data analist", "data_analytics"},
	{"bi analist", "data_analytics"},
	{"bi ontwikkelaar", "data_analytics"},
	{"-data-analist", "data_analytics"},
	{"-dataanalist", "data_analytics"},
	{"-bi-analist", "data_analytics"},
	{"-webanalist", "data_analytics"},
	{"-bi-ontwikkelaar", "data_analytics"},
	{"-data-engineer", "data_engineering"},
	{"-testanalist", "qa"},
	{"-testautomatiseerder", "qa"},
	{"-softwaretester", "qa"},
	{"-testcoördinator", "qa"},
	{"-testmanager", "qa"},
	{"-testengineer", "qa"},
	{"-testconsultant", "qa"},
	{"financieel analist", "finance"},
	{"-risicoanalist", "finance"},
	{"-risico-analist", "finance"},
	{"-kredietanalist", "finance"},
	{"-beleggingsanalist", "finance"},
	{"chemisch analist", "science"},
	{"-laboratoriumanalist", "science"},
	{"netwerk engineer", "network_engineering"},
	{"netwerk specialist", "network_engineering"},
	{"netwerk beheerder", "network_engineering"},
	{"-netwerkbeheerder", "network_engineering"},
	{"-netwerkengineer", "network_engineering"},
	{"-netwerk-engineer", "network_engineering"},
	{"-netwerkspecialist", "network_engineering"},
	{"-systeembeheerder", "devops"}, // the main table files the system administrator here
	{"-databasebeheerder", "devops"},
	{"-ict-beheerder", "devops"},
	{"ict beheerder", "devops"},
	{"it beheerder", "devops"},
	// The developer, before the administrator: an "Ontwikkelaar/Applicatiebeheerder"
	// writes the code. The CNC/CAM programmer runs a machining shop's machines, not
	// software.
	{"cad/cam programmeur", "manufacturing"},
	{"cadcam programmeur", "manufacturing"},
	{"cnc programmeur", "manufacturing"},
	{"cam programmeur", "manufacturing"},
	{"-cnc-programmeur", "manufacturing"},
	{"-cam-programmeur", "manufacturing"},
	{"-cadcam-programmeur", "manufacturing"},
	{"-laserprogrammeur", "manufacturing"},
	{"-ontwikkelaar", "software_engineering"},
	{"-programmeur", "software_engineering"},
	// The service desk and the application administrator who supports its users are
	// it_support; the customer-service desk beside them, and the bare
	// "supportmedewerker" (as often the customer side), stay support.
	{"-servicedeskmedewerker", "it_support"},
	{"-helpdeskmedewerker", "it_support"},
	{"-supportmedewerker", "support"},
	{"servicedesk", "it_support"},
	{"-klantenservicemedewerker", "support"},
	{"klantenservice", "support"},
	{"-callcentermedewerker", "support"},
	{"-klantcontactmedewerker", "support"},
	{"-werkplekbeheerder", "it_support"},
	{"-werkplekondersteuner", "it_support"},
	{"-gebruikersondersteuner", "it_support"},
	{"-applicatiebeheerder", "it_support"},
	{"-architect", "architecture"}, // Domeinarchitect, Informatiearchitect

	// Marketing.
	{"-marketeer", "marketing"},
	{"-marketer", "marketing"},

	// Healthcare, before the support worker: a "Verpleegkundige/Begeleider" is a nurse.
	{"-verpleegkundige", "healthcare"},
	{"verpleegkundig specialist", "healthcare"},
	{"-verloskundige", "healthcare"},
	{"-verzorgende", "healthcare"}, // Verzorgende IG, Kraamverzorgende
	{"helpende", "healthcare"},
	{"-tandartsassistent", "healthcare"},
	{"-doktersassistent", "healthcare"},
	{"-apothekersassistent", "healthcare"},
	{"-zorgassistent", "healthcare"},
	{"-operatieassistent", "healthcare"},
	{"-anesthesiemedewerker", "healthcare"},
	{"-praktijkondersteuner", "healthcare"}, // the GP practice nurse (POH)
	{"-mondhygiënist", "healthcare"},
	{"-tandarts", "healthcare"},
	{"-huisarts", "healthcare"},
	{"-basisarts", "healthcare"},
	{"-bedrijfsarts", "healthcare"},
	{"-oogarts", "healthcare"},
	{"-sportarts", "healthcare"},
	{"-verslavingsarts", "healthcare"},
	{"-ziekenhuisarts", "healthcare"},
	{"specialist ouderengeneeskunde", "healthcare"},
	{"-psycholoog", "healthcare"},
	{"-psychiater", "healthcare"},
	{"-zorgcoördinator", "healthcare"},
	{"-ambulancechauffeur", "healthcare"},

	// Education, before the support worker: the pedagogical staff of a day care.
	{"pedagogisch medewerker", "education"},
	{"pedagogisch medewerkster", "education"},
	{"pedagogisch professional", "education"},
	{"pedagogisch begeleider", "education"},
	{"-huiswerkbegeleider", "education"},
	{"-onderwijsbegeleider", "education"},
	{"-onderwijsassistent", "education"},
	{"-docent", "education"},
	{"-leraar", "education"},
	{"-lerares", "education"},
	{"-leerkracht", "education"},
	{"-gastouder", "education"},

	// Social services: the support worker of disability and youth care.
	{"-orthopedagoog", "social_services"},
	{"-agoog", "social_services"},
	{"-jeugdzorgwerker", "social_services"},
	{"maatschappelijk werker", "social_services"},
	{"-ervaringsdeskundige", "social_services"},
	{"-begeleider", "social_services"},
	{"-begeleidster", "social_services"},

	// Occupational safety — the KAM (quality, safety, environment) and HSE seats —
	// before the guard below.
	{"-veiligheidskundige", "occupational_safety"},
	{"-kam-coördinator", "occupational_safety"},
	{"kam coördinator", "occupational_safety"},
	{"-veiligheidscoördinator", "occupational_safety"},
	{"-preventiemedewerker", "occupational_safety"},
	{"-arbo-adviseur", "occupational_safety"},
	{"arbo adviseur", "occupational_safety"},

	// Public safety.
	{"-beveiliger", "public_safety"},
	{"-beveiligingsmedewerker", "public_safety"},
	{"-beveiligingsbeambte", "public_safety"},
	{"-handhaver", "public_safety"},
	{"-brandweerman", "public_safety"},
	{"-politieagent", "public_safety"},

	// Construction and the trades. The work planner draws and schedules the job; the
	// calculator prices it, as the main table's estimator does.
	{"-werkvoorbereider", "engineering_design"},
	{"-tekenaar", "engineering_design"},
	{"-constructeur", "engineering_design"},
	{"-calculator", "industrial_engineering"},
	{"-uitvoerder", "skilled_trades"}, // the site manager, filed with the trades
	{"-timmerman", "skilled_trades"},
	{"-timmervrouw", "skilled_trades"},
	{"-elektricien", "skilled_trades"},
	{"-loodgieter", "skilled_trades"},
	{"-schilder", "skilled_trades"},
	{"-metselaar", "skilled_trades"},
	{"-lasser", "skilled_trades"},
	{"-stukadoor", "skilled_trades"},
	{"-tegelzetter", "skilled_trades"},
	{"-dakdekker", "skilled_trades"},
	{"-plaatwerker", "skilled_trades"},
	{"-autotechnicus", "skilled_trades"},
	{"-servicetechnicus", "skilled_trades"},
	{"-onderhoudstechnicus", "skilled_trades"},
	{"-storingstechnicus", "skilled_trades"},
	{"-fietstechnicus", "skilled_trades"},
	{"-autoschadetechnicus", "skilled_trades"},

	// Manufacturing.
	{"-productiemedewerker", "manufacturing"},
	{"-montagemedewerker", "manufacturing"},
	{"-assemblagemedewerker", "manufacturing"},
	{"-productieplanner", "manufacturing"},

	// Logistics.
	{"-chauffeur", "logistics"},
	{"-magazijnmedewerker", "logistics"},
	{"logistiek medewerker", "logistics"},
	{"logistiek medewerkster", "logistics"},
	{"medewerker logistiek", "logistics"},
	{"-orderpicker", "logistics"},
	{"-bezorger", "logistics"},
	{"-koerier", "logistics"},
	{"-transportplanner", "logistics"},

	// Hospitality.
	{"-kok", "hospitality"}, // Chefkok, Lunchkok, Zelfstandig werkend kok
	{"-horecamedewerker", "hospitality"},
	{"-keukenmedewerker", "hospitality"},
	{"-bedieningsmedewerker", "hospitality"},
	{"medewerker bediening", "hospitality"},
	{"-afwasser", "hospitality"},
	{"-barista", "hospitality"},
	{"-barmedewerker", "hospitality"},
	{"-gastheer", "hospitality"},
	{"-gastvrouw", "hospitality"},

	// Retail.
	{"-verkoopmedewerker", "retail"},
	{"-verkoopmedewerkster", "retail"},
	{"-winkelmedewerker", "retail"},
	{"-kassamedewerker", "retail"},
	{"-filiaalmedewerker", "retail"},
	{"-vulploegmedewerker", "retail"},
	{"-verkoper", "retail"},
	{"-verkoopster", "retail"},
	{"-caissière", "retail"},
	{"-winkelmanager", "retail"},
	{"-filiaalmanager", "retail"},
	{"-filiaalleider", "retail"},
	{"-storemanager", "retail"},

	// Personal services.
	{"-schoonmaker", "personal_services"},
	{"-schoonmaakster", "personal_services"},
	{"-schoonmaakmedewerker", "personal_services"},
	{"medewerker schoonmaak", "personal_services"},
	{"-glazenwasser", "personal_services"},
	{"-kapper", "personal_services"},
	{"huishoudelijke hulp", "personal_services"},
	{"-huismeester", "personal_services"},
	{"-conciërge", "personal_services"},

	// Finance, ahead of the office clerk: a financial administrative clerk keeps books.
	{"financieel medewerker", "finance"},
	{"financieel administratief medewerker", "finance"},
	{"financieel administrateur", "finance"},
	{"medewerker financiële administratie", "finance"},
	{"-salarisadministrateur", "finance"}, // the main table files payroll here
	{"-boekhouder", "finance"},
	{"-belastingadviseur", "finance"},
	{"-hypotheekadviseur", "finance"},
	{"-pensioenadviseur", "finance"},
	{"-fiscalist", "finance"},
	{"-crediteurenmedewerker", "finance"},
	{"-debiteurenmedewerker", "finance"},

	// Law.
	{"-advocaat", "legal"},
	{"-notaris", "legal"},
	{"juridisch medewerker", "legal"},
	{"juridisch adviseur", "legal"},

	// People.
	{"hr medewerker", "hr"},
	{"medewerker hr", "hr"},
	{"hr adviseur", "hr"},
	{"-hr-medewerker", "hr"},
	{"-hr-adviseur", "hr"},
	{"-personeelsadviseur", "hr"},

	// The office.
	{"administratief medewerker", "administration"},
	{"administratief medewerkster", "administration"},
	{"medewerker administratie", "administration"},
	{"-administratiemedewerker", "administration"},
	{"-secretaresse", "administration"},
	{"-managementassistent", "administration"},
	{"-managementassistente", "administration"},
	{"-directieassistent", "administration"},
	{"-managementondersteuner", "administration"},
	{"-receptioniste", "administration"},
	{"-receptionist", "administration"},
	{"-telefoniste", "administration"},
	{"-baliemedewerker", "administration"},
	{"-receptiemedewerker", "administration"},
	{"-frontdeskmedewerker", "administration"},
	{"-officemanager", "operations"}, // the main table's office manager

	// Projects.
	{"-projectleider", "project_management"},
	{"-projectmanager", "project_management"},
	{"-programmamanager", "project_management"},
	{"-projectcoördinator", "project_management"},

	// Sales: the account manager, the inside-sales desk, the estate agent.
	{"-accountmanager", "sales"},
	{"-relatiebeheerder", "sales"},
	{"-relatiemanager", "sales"},
	{"-vertegenwoordiger", "sales"},
	{"-binnendienstmedewerker", "sales"},
	{"-buitendienstmedewerker", "sales"},
	{"medewerker binnendienst", "sales"},
	{"commercieel medewerker", "sales"},
	{"-salesmedewerker", "sales"},
	{"-makelaar", "sales"},

	// The generic IT employee, last: an "Administratief Medewerker ICT" is the clerk.
	{"-ict-medewerker", "it_support"},
	{"ict medewerker", "it_support"},
	{"medewerker ict", "it_support"},
	{"it medewerker", "it_support"},
	{"medewerker it", "it_support"},
}

// dutchSeniorityTable lists the Dutch grade aliases in precedence order.
var dutchSeniorityTable = []aliasEntry{
	// "-leider" leads only where it leads: a companion, a trainer, an air-traffic
	// controller and a tour guide do not.
	{"-begeleider", categoryNone},
	{"-begeleidster", categoryNone},
	{"-opleider", categoryNone},
	{"-verkeersleider", categoryNone},
	{"-reisleider", categoryNone},
	// "stage" is the Dutch (and French) internship, and also an English word for a
	// theatre's stage, a rocket's and a clinical trial's. It is therefore a whole word
	// only, never a head — as a tail it read German "Urlaubstage" (holidays) and
	// "Justage" (calibration) as internships — and its English uses are vetoed.
	{"stage manager", categoryNone},
	{"stage management", categoryNone},
	{"stage technician", categoryNone},
	{"first stage", categoryNone}, // "First Stage Researcher (R1)", the EU's PhD grade
	{"early stage", categoryNone},
	{"early-stage", categoryNone},
	{"late stage", categoryNone},
	{"late-stage", categoryNone},
	{"clinical stage", categoryNone},
	{"growth stage", categoryNone},
	{"growth-stage", categoryNone},
	{"stage 0", categoryNone},
	{"stage 1", categoryNone},
	{"stage 2", categoryNone},
	{"stage 3", categoryNone},
	{"stage i", categoryNone}, // a broker's licensing tier: "Financial Solutions Advisor Stage I"
	{"stage ii", categoryNone},
	{"data stage", categoryNone}, // IBM InfoSphere DataStage
	// The board and the statutory director. A plain "directeur" is left alone, as the
	// main table leaves the English "director": a Vestigingsdirecteur runs a branch.
	{"adjunct-directeur", "lead"},
	{"adjunct directeur", "lead"},
	{"algemeen directeur", "c_level"},
	{"algemeen directrice", "c_level"},
	{"directeur-bestuurder", "c_level"},
	{"directrice-bestuurder", "c_level"},
	{"statutair directeur", "c_level"},
	{"financieel directeur", "c_level"},
	{"commercieel directeur", "c_level"},
	{"operationeel directeur", "c_level"},
	{"technisch directeur", "c_level"},
	{"raad van bestuur", "c_level"},
	// The trainee grades outrank the lead heads, as in German: a "Stagiair
	// Projectleider" is an intern.
	{"stage", "intern"},
	{"-afstudeerstage", "intern"},
	{"-meewerkstage", "intern"},
	{"-snuffelstage", "intern"},
	{"-onderzoeksstage", "intern"},
	{"-onderzoekstage", "intern"},
	{"-stagiair", "intern"},
	{"-stagiaire", "intern"},
	{"stageplaats", "intern"},
	{"stageplek", "intern"},
	{"stageopdracht", "intern"},
	{"afstudeeropdracht", "intern"},
	{"afstudeerder", "intern"},
	{"-traineeship", "intern"},
	{"bbl", "intern"}, // the work-and-learn apprenticeship
	{"bbl'er", "intern"},
	{"leerling", "intern"}, // "Leerling Kok", "(Leerling) Allround timmerman"
	{"medior", "middle"},
	{"leidinggevende", "lead"},
	{"hoofd", "lead"},
	{"-hoofd", "lead"}, // Teamhoofd, Afdelingshoofd
	{"-leider", "lead"},
	{"-leidster", "lead"},
	{"-teamlead", "lead"},
	{"-voorman", "lead"},
	{"-voorvrouw", "lead"},
}

// assistantOfDutchLead is the assistant OF a lead — "Assistent Bedrijfsleider",
// "Assistent Teamleider", "Assistent Filiaalleider" — which states no lead grade. It is
// cut before the Dutch grade match, as assistantOfLead is before the German one.
var assistantOfDutchLead = regexp.MustCompile(`\bassistente?\s+\p{L}*(?:leider|leidster|hoofd)\b`)

// foldDutchSeniority prepares a lowercased title for the Dutch grade match.
func foldDutchSeniority(lower string) string {
	return assistantOfDutchLead.ReplaceAllString(strings.Join(strings.Fields(genderMarker.ReplaceAllString(lower, " ")), " "), " ")
}

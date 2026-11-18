package classify

// coreCategoryTable7 is part 7 of 17 of coreCategoryTable; the parts concatenate in order.
var coreCategoryTable7 = []aliasEntry{
	// Field-facing delivery work: the seats between engineering and the customer. IT-profile, unlike most of what the later
	// waves resolve. The hyphenated FDE is a correctness fix: roletag declares "forward-deployed engineer" and classify did
	// not, so the role fired while the category stayed empty (the same word-boundary trap as the automotive plurals).
	{"forward-deployed engineer", "solutions_engineering"},
	{"professional services engineer", "solutions_engineering"},
	{"professional services consultant", "solutions_engineering"},
	{"partner engineer", "solutions_engineering"},
	{"deployment strategist", "solutions_engineering"},
	{"presales consultant", "solutions_engineering"},
	{"pre-sales consultant", "solutions_engineering"},
	{"delivery consultant", "solutions_engineering"},
	{"integration consultant", "solutions_engineering"},
	// The volume behind the bare phrase is platform work (ServiceNow, Salesforce, Dynamics, Oracle Technical
	// Consultants), not general management consulting.
	{"technical consultant", "solutions_engineering"},

	// The ERP and CRM functional consultants. An SAP FICO consultant configures a product for a customer, which is what
	// this category names; the plant-engineering wave is not their home. The developer and administrator spellings go
	// elsewhere on purpose, as in the Salesforce and ServiceNow families: a developer writes for the platform, and SAP
	// Basis is its infrastructure rather than its business logic.
	{"sap basis administrator", "devops"},
	{"dynamics 365 developer", "software_engineering"},
	{"dynamics 365 consultant", "solutions_engineering"},
	{"dynamics 365", "solutions_engineering"},
	{"microsoft dynamics", "solutions_engineering"},
	{"hubspot crm administrator", "solutions_engineering"},
	{"sap consultant", "solutions_engineering"},
	{"sap fico", "solutions_engineering"},
	{"netsuite developer", "software_engineering"},
	{"netsuite engineer", "software_engineering"},
	{"netsuite administrator", "software_engineering"},
	{"netsuite consultant", "solutions_engineering"},
	{"sap sd consultant", "solutions_engineering"},
	{"sap mm consultant", "solutions_engineering"},
	// No bare "crm specialist": an unqualified one is a marketing-operations seat at least as often as a platform one.
	// The qualified spellings above carry the volume.

	// The consumer industries: healthcare, skilled trades, retail and hospitality, the residue a broad multi-industry ATS
	// crawl brings in.
	// Declared before the industrial block because two members are qualified spellings of words that block owns: a
	// "Medication Technician" is healthcare and a "Field Service Technician" is a trade, both settled before any bare
	// technician or engineer word is reached.
	// A virtual assistant who works for a clinic is still a virtual assistant and is filed in `administration`. Declared
	// above the healthcare block because "Virtual Medical Assistant" contains "medical assistant", which would take it.
	// ("Medical Virtual Assistant" needs no entry: it contains "virtual assistant" and nothing above that.)
	{"virtual medical assistant", "administration"},

	// HEALTHCARE first, because its qualified spellings would otherwise fall to the trades.
	{"medication technician", "healthcare"},
	{"pharmacy technician", "healthcare"},
	{"patient care technician", "healthcare"},
	{"veterinary technician", "healthcare"},
	{"surgical technician", "healthcare"},
	{"registered nurse", "healthcare"},
	{"nurse practitioner", "healthcare"},
	{"licensed practical nurse", "healthcare"},
	{"nurse", "healthcare"},
	{"rn", "healthcare"},
	{"lpn", "healthcare"},
	{"cna", "healthcare"},
	{"caregiver", "healthcare"},
	{"home health aide", "healthcare"},
	{"medical assistant", "healthcare"},
	{"dental hygienist", "healthcare"},
	{"dental assistant", "healthcare"},
	{"patient coordinator", "healthcare"},
	{"phlebotomist", "healthcare"},
	{"phlebotomy", "healthcare"},
	// The imaging room. These spellings are already in nontech.go's craft list, but had nowhere to be FILED, which left them
	// unfilterable.
	{"x-ray technologist", "healthcare"},
	{"xray technologist", "healthcare"},
	{"x-ray technician", "healthcare"},
	{"radiologic technologist", "healthcare"},
	{"radiology technologist", "healthcare"},
	{"mri technologist", "healthcare"},
	{"sonographer", "healthcare"},
	{"physical therapist", "healthcare"},
	{"occupational therapist", "healthcare"},
	{"veterinarian", "healthcare"},
	{"physician", "healthcare"},
	{"optometrist", "healthcare"},

	// SKILLED TRADES. The qualified technician spellings come first; the bare word
	// closes the family.
	{"field service technician", "skilled_trades"},
	{"installation technician", "skilled_trades"},
	{"service technician", "skilled_trades"},
	{"diesel technician", "skilled_trades"},
	{"automotive technician", "skilled_trades"},
	{"maintenance technician", "skilled_trades"},
	{"hvac technician", "skilled_trades"},
	{"automotive mechanic", "skilled_trades"},
	// The PLURAL spellings. wordmatch matches whole words and has no morphology, so a singular alias cannot reach a plural
	// title ("Automotive Mechanics", "Automotive Tire Technicians").
	{"automotive mechanics", "skilled_trades"},
	{"automotive technicians", "skilled_trades"},
	{"tire technicians", "skilled_trades"},
	{"alignment technicians", "skilled_trades"},
	{"service technicians", "skilled_trades"},
	{"mechanics", "skilled_trades"},
	{"mechanic", "skilled_trades"},
	{"electrician", "skilled_trades"},
	{"plumber", "skilled_trades"},
	{"welder", "skilled_trades"},
	{"machinist", "skilled_trades"},
	{"millwright", "skilled_trades"},
	{"carpenter", "skilled_trades"},

	// RETAIL. The grocery clerk family is shop floor, not office administration; filing it by the word "clerk" would put a
	// supermarket's staff in the same facet as a receptionist.
	{"deli clerk", "retail"},
	{"grocery clerk", "retail"},
	{"produce clerk", "retail"},
	{"bakery clerk", "retail"},
	{"meat clerk", "retail"},
	{"store driver", "retail"},
	{"sales associate", "retail"},
	{"retail associate", "retail"},
	{"retail service specialist", "retail"},
	// The factory-floor spelling of the shop-floor title. Declared above the bare "team member", because the production
	// block below sits too late to take it.
	{"production team member", "manufacturing"},
	{"team member", "retail"},
	{"cashier", "retail"},
	{"merchandiser", "retail"},
	{"merchandising", "retail"},
	{"brand ambassador", "retail"},
	{"product demonstrator", "retail"},
	{"store leader", "retail"},
	{"stock associate", "retail"},

	// HOSPITALITY.
	{"banquet server", "hospitality"},
	{"server", "hospitality"},
	{"host/hostess", "hospitality"},
	{"hostess", "hospitality"},
	// Titles exactly "Host" are common. The bare word collides with web hosting, but every hosting title names the thing
	// it hosts ("Hosting Engineer", "Web Host") and resolves far above; this block is declared late, so what reaches it
	// names nothing else.
	{"host", "hospitality"},
	{"line cook", "hospitality"},
	{"prep cook", "hospitality"},
	{"cook", "hospitality"},
	{"chef", "hospitality"},
	{"barista", "hospitality"},
	{"bartender", "hospitality"},
	{"dishwasher", "hospitality"},
	{"busser", "hospitality"},
	{"kitchen assistant", "hospitality"},

	// PUBLIC SAFETY: the protective services, SOC major group 33 (police, sheriffs, corrections, firefighters, guards,
	// lifeguards, crossing guards). Non-technical (vocab.NonTechCategories).
	// The family is declared in three places. The guard spellings sit in the security block, beside the information-security
	// spellings they must not be confused with. The entries here must precede the logistics block, whose bare "dispatcher"
	// would take the emergency dispatchers (a 911 dispatcher dispatches police and fire, though SOC files them under
	// 43-5031), and the healthcare "paramedic", which would take "Firefighter/Paramedic" (a firefighter with a paramedic
	// licence, SOC 33-2011). Police, sheriffs, corrections and loss prevention are declared at the END of the table,
	// because those words name the EMPLOYER as often as the job: "Food Service Worker - Sheriff's Office" is a cook, and
	// "Sheriff's Office Nurse" a nurse.
	// Bare "guard" is not an alias: the National Guard and the Coast Guard are military, and SOC 55 has no category here.
	{"public safety dispatcher", "public_safety"},
	{"public safety telecommunicator", "public_safety"},
	{"public safety communications", "public_safety"},
	{"911 dispatcher", "public_safety"},
	{"emergency dispatcher", "public_safety"},
	{"police dispatcher", "public_safety"},
	{"fire dispatcher", "public_safety"},
	{"telecommunicator", "public_safety"},
	{"firefighter", "public_safety"},
	{"firefighters", "public_safety"},
	{"fire fighter", "public_safety"},
	// SOC 33-9092. A lifeguard is a protective service, not a personal one.
	{"lifeguard", "public_safety"},
	{"lifeguards", "public_safety"},
	{"life guard", "public_safety"},
	{"crossing guard", "public_safety"},

	// The service sectors: the last clusters in the catalogue that have a shape.
	// LOGISTICS. "Store Driver" is declared in the retail block above (a shop's own driver stays with the shop), so the bare
	// driver aliases here cannot take it.
	{"delivery specialist", "logistics"},
	{"delivery driver", "logistics"},
	{"commercial driver", "logistics"},
	{"cdl driver", "logistics"},
	{"truck driver", "logistics"},
	{"driver", "logistics"},
	{"courier", "logistics"},
	{"warehouse associate", "logistics"},
	{"warehouse operator", "logistics"},
	{"warehouse supervisor", "logistics"},
	{"warehouse assistant", "logistics"},
	// A DATA warehouse is the analytics store, never a building ("Data Warehouse Developer"). The architect resolves
	// above, so only the remainder reaches these.
	{"data warehouse", "data_engineering"},
	{"data warehousing", "data_engineering"},
	{"warehouse", "logistics"},
	{"forklift operator", "logistics"},
	{"truck unloader", "logistics"},
	{"fulfillment associate", "logistics"},
	{"dispatcher", "logistics"},

	// EDUCATION. No bare "coach": "Agile Coach" resolves to project management far above, but "Career Coach" and "Sales
	// Coach" are genuinely ambiguous; the volume here is private sports coaching, which the qualified spellings reach.
	{"swim instructor", "education"},
	{"chess instructor", "education"},
	{"soccer coach", "education"},
	{"basketball coach", "education"},
	{"fitness coach", "education"},
	{"preschool teacher", "education"},
	{"substitute teacher", "education"},
	{"teacher", "education"},
	{"tutor", "education"},
	// No bare "instructor": the suite pins "Safe Driving Instructor" to no category (the guard that keeps a bare "safe" off
	// the SAFe agile alias), and a bare instructor alias would override it.
	{"lecturer", "education"},
	{"professor", "education"},

	// ADMINISTRATION: the front desk and the paperwork, not an IT company's back-office (that is `operations`; filing a
	// court clerk there would muddy a facet that works).
	{"office manager", "administration"},
	{"office assistant", "administration"},
	{"receptionist", "administration"},
	{"legal secretary", "administration"},
	{"medical secretary", "administration"},
	{"secretary", "administration"},
	{"data entry", "administration"},
	// The abbreviated and coordinator/specialist spellings, plus the virtual-assistant and front-desk titles of a broad ATS
	// crawl. The bare alias "assistant" is never added: in live titles it states a GRADE ("Assistant Controller") or
	// qualifies a non-administrative trade ("Maintenance Assistant", "Clinic Assistant") far more often than admin work.
	{"admin assistant", "administration"},
	{"administrative coordinator", "administration"},
	{"administrative specialist", "administration"},
	{"front desk", "administration"},
	{"virtual assistant", "administration"},
	// The rest of the assistant family, in every language the catalogue carries it. Executive and administrative assistants
	// resolve here, from entries declared up beside `operations` so they keep their precedence.
	// The abbreviation "VA" is never an alias: it is far more often Virginia ("Arlington VA") or the US Veterans Affairs,
	// and in Swedish titles the water-and-sewage trade ("VA-projektör"). The spelled-out phrase carries the volume.
	{"online assistant", "administration"},
	{"asistente virtual", "administration"},
	{"assistente virtual", "administration"},
	{"assistant virtuel", "administration"},
	{"assistante virtuelle", "administration"},
	{"virtuelle assistenz", "administration"},
	{"virtueller assistent", "administration"},
	{"asistente ejecutiva", "administration"},
	{"asistente ejecutivo", "administration"},
	{"asistente administrativa", "administration"},
	{"asistente administrativo", "administration"},
}

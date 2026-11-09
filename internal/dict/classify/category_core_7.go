package classify

// coreCategoryTable7 is part 7 of 17 of coreCategoryTable; the parts concatenate in order.
var coreCategoryTable7 = []aliasEntry{
	// Field-facing delivery work — the seats between engineering and the customer.
	// Small (1 180 open) but IT-profile, unlike most of what the later waves resolve.
	//
	// The hyphenated FDE is a correctness fix, not an addition: roletag declares
	// "forward-deployed engineer" explicitly and classify did not, so the role fired
	// while the category stayed empty. The same word-boundary trap as the automotive
	// plurals, hiding inside a title the catalogue already considered covered.
	{"forward-deployed engineer", "solutions_engineering"},
	{"professional services engineer", "solutions_engineering"},
	{"professional services consultant", "solutions_engineering"},
	{"partner engineer", "solutions_engineering"},
	{"deployment strategist", "solutions_engineering"},
	{"presales consultant", "solutions_engineering"},
	{"pre-sales consultant", "solutions_engineering"},
	{"delivery consultant", "solutions_engineering"},
	{"integration consultant", "solutions_engineering"},
	// The volume behind the bare phrase is platform work — ServiceNow, Salesforce,
	// Dynamics and Oracle Technical Consultants — not general management consulting.
	{"technical consultant", "solutions_engineering"},

	// The ERP and CRM functional consultants. The IT wave deferred them "to the
	// industrial wave" for want of a home and that wave never took them — it was for
	// plant engineering, and this is platform implementation work. They sit here, with
	// the platform consultants above: an SAP FICO consultant configures a product for
	// a customer, which is what this category names.
	//
	// The developer and the administrator spellings go elsewhere on purpose, the same
	// split the Salesforce and ServiceNow families already carry: a developer writes
	// for the platform, and SAP Basis is its infrastructure rather than its business
	// logic.
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
	// No bare "crm specialist": an unqualified one is a marketing-operations seat at
	// least as often as a platform one, and the IT wave declined it for that reason.
	// The qualified spellings above carry the volume.

	// The consumer industries: healthcare, skilled trades, retail and hospitality.
	// 225 000 open postings that were filterable by nothing at all — the residue a
	// broad multi-industry ATS crawl brings in with the boards it wants.
	//
	// Declared before the industrial block because two of its members are qualified
	// spellings of words that block owns: a "Medication Technician" is healthcare and
	// a "Field Service Technician" is a trade, and both must be settled before any
	// bare technician or engineer word is reached.
	//
	// A virtual assistant who works for a clinic is still a virtual assistant — remote
	// scheduling, refills, prior authorisations — and is filed with the rest of that
	// family in `administration`. Declared above the healthcare block because the
	// "Virtual Medical Assistant" spelling contains "medical assistant", which would
	// otherwise take it. ("Medical Virtual Assistant" needs no entry: it contains
	// "virtual assistant" and nothing above that.)
	{"virtual medical assistant", "administration"},

	// HEALTHCARE first, because its qualified spellings are the ones that would
	// otherwise fall to the trades.
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
	// The imaging room. These spellings are already named in nontech.go's craft list, so
	// the catalogue knew they were not technical; it had nowhere to FILE them, which left
	// them unfilterable — exactly the gap this consumer block exists to close.
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
	// The PLURAL spellings. wordmatch matches whole words and has no morphology, so a
	// singular alias cannot reach a plural title — a gap nothing in an alias list
	// shows, and it left the three largest automotive spellings in the catalogue
	// ("Automotive Mechanics" 1292, "AUTOMOTIVE TIRE TECHNICIANS" 1248, "Automotive
	// Alignment Technicians" 490) resolving to nothing while their singular forms
	// resolved fine.
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

	// RETAIL. The grocery clerk family is shop floor, not office administration —
	// filing it by the word "clerk" would put a supermarket's whole staff in the same
	// facet as a receptionist.
	{"deli clerk", "retail"},
	{"grocery clerk", "retail"},
	{"produce clerk", "retail"},
	{"bakery clerk", "retail"},
	{"meat clerk", "retail"},
	{"store driver", "retail"},
	{"sales associate", "retail"},
	{"retail associate", "retail"},
	{"retail service specialist", "retail"},
	// The factory-floor spelling of the shop-floor title. Declared here, above the bare
	// "team member", because the production block below sits too late to take it.
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
	// 3 446 open postings are titled exactly "Host". The bare word collides with web
	// hosting, but every hosting title in this catalogue names the thing it hosts
	// ("Hosting Engineer", "Web Host") and resolves far above through its own
	// discipline — this block is declared late, so what reaches it names nothing else.
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

	// PUBLIC SAFETY — the protective services, SOC major group 33: police, sheriffs,
	// corrections, firefighters, guards, lifeguards and crossing guards. Non-technical
	// (vocab.NonTechCategories), and the home the guards had lacked: they sat in
	// `personal_services` beside the barbers, and a "Security Guard" once resolved to
	// the information-security facet.
	//
	// The family is declared in three places, each for a reason. The guard spellings
	// sit in the security block, beside the information-security spellings they must not
	// be confused with. The entries here must precede the logistics block, whose bare
	// "dispatcher" would otherwise take the emergency dispatchers — SOC files those under
	// 43-5031 with the office clerks, but this table files every other dispatcher by the
	// field it dispatches for, and a 911 dispatcher dispatches police and fire — and the
	// healthcare "paramedic", which would otherwise take "Firefighter/Paramedic": that is
	// a firefighter who also holds the paramedic licence (SOC 33-2011). The lifeguard
	// keeps the position its old personal_services entry had. Everything else — police,
	// sheriffs, corrections, loss prevention — is declared at the END of the table,
	// because those words name the EMPLOYER as often as the job: "Food Service Worker -
	// Sheriff's Office" is a cook, and "Sheriff's Office Nurse" a nurse.
	//
	// Bare "guard" is not an alias at all — the National Guard and the Coast Guard are
	// military, and SOC 55 has no category here.
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
	// SOC 33-9092. A lifeguard watches a pool for the swimmer who gets into trouble — a
	// protective service, not the personal one it was filed under.
	{"lifeguard", "public_safety"},
	{"lifeguards", "public_safety"},
	{"life guard", "public_safety"},
	{"crossing guard", "public_safety"},

	// The service sectors: the last clusters in the catalogue that have a shape.
	// LOGISTICS. "Store Driver" is declared in the retail block above — a shop's own
	// driver stays with the shop — so the bare driver aliases here cannot take it.
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
	// A DATA warehouse is the analytics store, never a building: "Data Warehouse
	// Developer" and "Architekt IT – Data Warehouse" were filed with the forklift
	// drivers. The architect resolves above, so only the remainder reaches these.
	{"data warehouse", "data_engineering"},
	{"data warehousing", "data_engineering"},
	{"warehouse", "logistics"},
	{"forklift operator", "logistics"},
	{"truck unloader", "logistics"},
	{"fulfillment associate", "logistics"},
	{"dispatcher", "logistics"},

	// EDUCATION. No bare "coach": "Agile Coach" resolves to project management far
	// above, but "Career Coach" and "Sales Coach" are genuinely ambiguous, and the
	// volume here is private sports coaching, which the qualified spellings reach.
	{"swim instructor", "education"},
	{"chess instructor", "education"},
	{"soccer coach", "education"},
	{"basketball coach", "education"},
	{"fitness coach", "education"},
	{"preschool teacher", "education"},
	{"substitute teacher", "education"},
	{"teacher", "education"},
	{"tutor", "education"},
	// No bare "instructor": the suite pins "Safe Driving Instructor" to no category —
	// it is the guard that keeps a bare "safe" off the SAFe agile alias — and a bare
	// instructor alias overrides it. The volume here is private sports and swim
	// coaching, which the qualified spellings above already reach.
	{"lecturer", "education"},
	{"professor", "education"},

	// ADMINISTRATION — the front desk and the paperwork, not an IT company's
	// back-office (that is `operations`, and filing a court clerk there would muddy a
	// facet that works).
	{"office manager", "administration"},
	{"office assistant", "administration"},
	{"receptionist", "administration"},
	{"legal secretary", "administration"},
	{"medical secretary", "administration"},
	{"secretary", "administration"},
	{"data entry", "administration"},
	// The abbreviated and coordinator/specialist spellings, plus the virtual-assistant
	// and front-desk titles a broad ATS crawl's admin/VA segment actually carries. The
	// bare alias "assistant" is deliberately never added: in live titles it states a
	// GRADE ("Assistant Controller") or qualifies a non-administrative trade
	// ("Maintenance Assistant", "Clinic Assistant") far more often than it names admin
	// work, so only these qualified phrases earn an entry.
	{"admin assistant", "administration"},
	{"administrative coordinator", "administration"},
	{"administrative specialist", "administration"},
	{"front desk", "administration"},
	{"virtual assistant", "administration"},
	// The rest of the assistant family, in every language the catalogue carries it.
	// Executive and administrative assistants used to resolve to `operations`, which
	// split one job across two facets: "Virtual Assistant" in administration,
	// "Executive Assistant" in operations. They now resolve here too, from entries
	// declared up beside `operations` so they keep the precedence they had.
	//
	// The abbreviation "VA" is deliberately never an alias. In this catalogue it is far
	// more often Virginia ("Vienna, VA office", "Arlington VA") or the US Veterans
	// Affairs, and in Swedish titles it is the water-and-sewage trade ("VA-projektör").
	// The spelled-out phrase carries the volume.
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

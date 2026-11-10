package classify

// coreCategoryTable3 is part 3 of 17 of coreCategoryTable; the parts concatenate in order.
var coreCategoryTable3 = []aliasEntry{
	// The hyphenated spelling is the industry's own and a hyphen is a word boundary, so
	// it needs its own entry — the same trap "middle-east" and "ai-product engineer"
	// document elsewhere in this file.
	{"mixed-signal design engineer", "hardware"},
	{"digital design engineer", "hardware"},
	// "Digital design" alone names three crafts in this catalogue — chip RTL, screen
	// graphics, and BIM building design — so the bare phrase stays with the bare
	// "design" below and only the forms that state the chip side are listed here.
	// Measured 2026-10-25 over 619 open titles: these were the chip spellings the bare
	// alias was filing as product design.
	{"digital design engineers", "hardware"},
	{"digital design engineering", "hardware"},
	{"digital design verification", "hardware"},
	{"digital design and verification", "hardware"},
	{"digital design & verification", "hardware"},
	{"digital design/verification", "hardware"},
	{"digital design / design verification", "hardware"},
	{"digital design rtl", "hardware"},
	{"digital design and signal processing", "hardware"},
	{"digital design and implementation", "hardware"},
	{"digital design power engineer", "hardware"},
	{"digital design lead engineer", "hardware"},
	{"digital design electrical engineer", "hardware"},
	{"digital design-for-test", "hardware"},
	{"analog/digital design", "hardware"},
	{"analog & digital design", "hardware"},
	{"analog and digital design", "hardware"},
	{"mixed signal digital design", "hardware"},
	{"rf digital design", "hardware"},
	{"hw digital design", "hardware"},
	{"serdes digital design", "hardware"},
	{"pcie digital design", "hardware"},
	{"phy digital design", "hardware"},
	{"soc & digital design", "hardware"},
	{"digital design, soc", "hardware"},
	{"engineer, digital design", "hardware"},
	{"engineer digital design", "hardware"},
	{"engineer - digital design", "hardware"},
	{"engineer-digital design", "hardware"},
	{"engineer for digital design", "hardware"},
	{"engineer (digital design)", "hardware"},
	{"engineer ii-digital design", "hardware"},
	{"asic / digital design", "hardware"},
	{"asic security digital design", "hardware"},
	{"digital design - pd", "hardware"},
	{"digital design - mixed-signal", "hardware"},
	{"digital design member of technical staff", "hardware"},
	{"wireless and digital design", "hardware"},
	{"dft design engineer", "hardware"},
	{"rf design engineer", "hardware"},
	{"rfic design", "hardware"},
	{"analogue design engineer", "hardware"},
	{"silicon design", "hardware"},
	{"memory design engineer", "hardware"},
	{"standard cell design", "hardware"},
	{"vlsi design", "hardware"},
	{"chip design", "hardware"},
	{"asic design", "hardware"},
	{"soc design", "hardware"},
	{"ic design", "hardware"},
	{"semiconductor design", "hardware"},
	{"product design engineer", "design"},
	{"design systems engineer", "design"},
	{"design system engineer", "design"},
	{"ux design engineer", "design"},
	{"ui design engineer", "design"},
	{"ui/ux design engineer", "design"},
	{"web design engineer", "design"},
	{"design engineer, product", "design"},
	// Design disciplines of their own, on the product side of the split.
	{"service design engineer", "design"},
	{"experience design engineer", "design"},
	// The two audio spellings that have to be declared here, above the draughting
	// block: both end in "design engineer", so left below they fall through to the bare
	// alias and land in draughting. They move out of `design` with the rest of audio —
	// leaving them behind would scatter one craft across three categories.
	{"sound design engineer", "creative"},
	{"audio design engineer", "creative"},
	{"game design engineer", "design"},
	// Then engineering design. The bare "design engineer" closes the block, and it
	// carries every qualified "<discipline> design engineer" form with it — those need
	// no entry of their own, since they resolve to the same category. Only the titles
	// the bare alias CANNOT see are listed: the "…designer" nouns, the design-less
	// phrases ("pcb design"), and the draughting words. The bare form routes here
	// because that population is overwhelmingly mechanical and industrial in this
	// catalogue — a product hybrid has to state one of the markers above.
	{"mechanical designer", "engineering_design"},
	{"electrical designer", "engineering_design"},
	{"civil designer", "engineering_design"},
	{"structural designer", "engineering_design"},
	{"piping designer", "engineering_design"},
	{"plumbing designer", "engineering_design"},
	{"hvac designer", "engineering_design"},
	{"cad designer", "engineering_design"},
	{"design technician", "engineering_design"},
	// The BIM / architectural-draughting family. "architectural" does not contain the
	// whole word "architect", so it cannot reach the software-architecture category
	// below. Bare "drafter"/"draftsman" is the profession itself, in any discipline.
	{"architectural designer", "engineering_design"},
	{"bim designer", "engineering_design"},
	{"bim coordinator", "engineering_design"},
	{"bim modeler", "engineering_design"},
	{"bim specialist", "engineering_design"},
	{"revit designer", "engineering_design"},
	// "Digital design" is also what consultancies call BIM delivery of a building's
	// engineering systems ("Electrical Digital Design Lead (Buildings)"). Only the
	// forms that name a building discipline are listed; the bare phrase stays design.
	{"electrical digital design", "engineering_design"},
	{"structural digital design", "engineering_design"},
	{"civil digital design", "engineering_design"},
	{"mechanical digital design", "engineering_design"},
	{"bim digital design", "engineering_design"},
	{"bim-digital design", "engineering_design"},
	{"ec&i digital design", "engineering_design"},
	{"security digital design specialist", "engineering_design"},
	{"security digital design coordinator", "engineering_design"},
	{"digital design (bim)", "engineering_design"},
	{"digital design specialist (bim)", "engineering_design"},
	{"digital design specialist (electrical", "engineering_design"},
	{"digital design specialist (structural", "engineering_design"},
	{"digital design lead (structural", "engineering_design"},
	{"digital design lead (mechanical", "engineering_design"},
	{"digital design lead (buildings)", "engineering_design"},
	{"digital design coordinator (electrical", "engineering_design"},
	{"buildings design delivery", "engineering_design"},
	// No bare "layout designer": magazine and print layout is the product-design craft,
	// and the phrase names both.
	{"tool designer", "engineering_design"},
	{"mold designer", "engineering_design"},
	{"die designer", "engineering_design"},
	{"drafter", "engineering_design"},
	{"draftsman", "engineering_design"},
	{"draughtsman", "engineering_design"},
	// Russian: the draughting profession. "инженер-конструктор" needs no entry of its
	// own — the hyphen is a word boundary, so the bare form resolves it.
	{"конструктор", "engineering_design"},
	{"design engineer", "engineering_design"},

	// Audio is the only media-production craft that has to be declared HERE, above the
	// bare "designer" alias: it is the only one whose title contains that word, which
	// is the whole reason a Sound Designer was filed with product designers. Every
	// other creative alias is declared at the very END of this table — see the block
	// there for why.
	{"sound designer", "creative"},
	{"audio designer", "creative"},
	{"designer", "design"},
	{"design", "design"},
	{"ux", "design"},
	{"ui", "design"},
	{"дизайнер", "design"},
	{"дизайн", "design"},
	{"product manager", "product"},
	{"product owner", "product"},
	{"продакт", "product"},
	{"продукт-менеджер", "product"},
	{"project manager", "project_management"},
	{"delivery manager", "project_management"},
	{"program manager", "project_management"},
	{"programme manager", "project_management"},
	{"project coordinator", "project_management"},
	{"program coordinator", "project_management"},
	{"project administrator", "project_management"},
	{"scrum master", "project_management"},
	{"scrum-master", "project_management"},
	{"agile coach", "project_management"},
	{"release train engineer", "project_management"},
	{"agile transformation lead", "project_management"},
	{"agile transformation manager", "project_management"},
	// Only qualified SAFe phrases resolve — bare "safe" is a common English word
	// (e.g. "Safe Driving Instructor"), a false-positive risk not worth taking.
	// "SAFe Scrum Master" needs no entry of its own: it already contains "scrum
	// master" above.
	{"scaled agile framework", "project_management"},
	{"safe practitioner", "project_management"},
	{"проджект", "project_management"},
	{"проект-менеджер", "project_management"},
	{"скрам-мастер", "project_management"},
	{"скрам мастер", "project_management"},
	// "IT Infrastructure" names an ESTATE, not a role, and its slot is the whole
	// ruling. It sits BELOW the technical role nouns, so the security engineer and the
	// project manager who happen to work on that estate keep their own discipline
	// ("IT Infrastructure Security Engineer", "IT Infrastructure Project Manager") —
	// in the devops block it outran both. It sits ABOVE the business functions, so
	// what is left over stays with the estate rather than drifting into back-office
	// ("IT Infrastructure & Operations Manager" is devops, not operations; "IT
	// Infrastructure Asset and Contract Manager" is devops, not legal). The residue
	// this costs is the handful of genuine sales titles on the estate ("Pre Sales
	// Specialist IT Infrastructure"), measured at ~4 against ~13 saved. Bare
	// "infrastructure" gets no entry at all: it names civil, rail, energy and telecom
	// work.
	{"it infrastructure", "devops"},
	// The engineering manager. On this board the unqualified title manages a SOFTWARE
	// team far more often than anything else, so it resolves `software_engineering` — the
	// craft the team practises, the same answer "Software Engineering Manager" gives —
	// and the `role_type` facet (internal/dict/roletype) is what says it manages people.
	// There is no management category: it was a seniority statement filed as a craft.
	//
	// The physical disciplines name their own craft between the two words, and the bare
	// alias would otherwise read every one of them as software. Each spelling below is
	// one the open catalogue carries; a discipline named elsewhere in the title
	// ("Engineering Manager (Food Manufacturing)") is not reachable by a phrase and stays
	// with the bare reading.
	{"sales engineering manager", "solutions_engineering"},
	{"solutions engineering manager", "solutions_engineering"},
	{"security engineering manager", "security"},
	{"mechanical engineering manager", "industrial_engineering"},
	{"electrical engineering manager", "industrial_engineering"},
	{"civil engineering manager", "industrial_engineering"},
	{"structural engineering manager", "industrial_engineering"},
	{"chemical engineering manager", "industrial_engineering"},
	{"environmental engineering manager", "industrial_engineering"},
	{"project engineering manager", "industrial_engineering"},
	{"manufacturing engineering manager", "industrial_engineering"},
	{"process engineering manager", "industrial_engineering"},
	{"industrial engineering manager", "industrial_engineering"},
	{"plant engineering manager", "industrial_engineering"},
	{"facilities engineering manager", "industrial_engineering"},
	{"facility engineering manager", "industrial_engineering"},
	{"maintenance engineering manager", "industrial_engineering"},
	{"reliability engineering manager", "industrial_engineering"},
	{"supplier quality engineering manager", "industrial_engineering"},
	{"application engineering manager", "industrial_engineering"},
	{"applications engineering manager", "industrial_engineering"},
	{"substation engineering manager", "industrial_engineering"},
	{"hospital engineering manager", "industrial_engineering"},
	{"engineering manager", "software_engineering"},
	{"marketing", "marketing"},
	{"smm", "marketing"},
	{"маркетолог", "marketing"},
	{"маркетинг", "marketing"},
	{"seo", "marketing"},
	{"search engine optimization", "marketing"},
	{"social media", "marketing"},
	{"контент-маркетолог", "marketing"},
	{"copywriter", "marketing"},
	{"content writer", "marketing"},
	{"brand manager", "marketing"},
	{"public relations", "marketing"},
	// The disciplines the coarse block did not name. Most were not merely unresolved
	// — the generic "manager" alias that used to sit further down claimed them. Each
	// is a phrase on purpose: the bare nouns ("growth", "content", "performance",
	// "geo") name technical roles too, and a standalone alias would take "Growth
	// Engineer" out of the tech categories and off the enrichment budget.
	{"demand generation", "marketing"},
	{"growth marketer", "marketing"},
	{"paid social", "marketing"},
	{"paid media", "marketing"},
	{"paid search", "marketing"},
	{"media buyer", "marketing"},
	{"pr manager", "marketing"},
	{"pr specialist", "marketing"},
	{"link building", "marketing"},
	{"content creator", "marketing"},
	{"growth hacker", "marketing"},
	// Event marketing: bare "event"/"events" is too common a word to trust alone
	// (an "Event-Driven Architecture Engineer" would false-positive), so only the
	// role-noun-qualified forms resolve.
	{"event manager", "marketing"},
	{"event coordinator", "marketing"},
	{"events manager", "marketing"},
	{"events coordinator", "marketing"},
	// App store growth (mobile) and web conversion growth. Bare "aso"/"cro" are both
	// overloaded elsewhere (ASO also names an Application Security Officer, CRO a
	// Chief Revenue Officer or a Contract Research Organization), so only the
	// spelled-out and fully role-qualified forms resolve.
	{"app store optimization", "marketing"},
	{"aso specialist", "marketing"},
	{"conversion rate optimization", "marketing"},
	// Generative-engine optimization: the industry names one job three ways. Only the
	// spelled-out forms and the bound abbreviation resolve — a bare "geo" is
	// geography, and "Geo Data Analyst" must stay with the analysts.
	{"generative engine optimization", "marketing"},
	{"answer engine optimization", "marketing"},
	{"generative search optimization", "marketing"},
	{"geo specialist", "marketing"},
	{"geo manager", "marketing"},
	{"aeo specialist", "marketing"},
	{"aeo manager", "marketing"},
	// Russian marketing titles, as full surface forms — the matcher needs word
	// boundaries, so a stem would not match. The hyphenated compounds were claimed
	// by the bare "менеджер" alias before this block existed.
	{"таргетолог", "marketing"},
	{"контент-менеджер", "marketing"},
	{"бренд-менеджер", "marketing"},
	{"пиар-менеджер", "marketing"},
	{"пиар-специалист", "marketing"},
	{"копирайтер", "marketing"},
	// solutions_engineering (technical pre-sales) before bare "sales" so "Sales
	// Engineer" wins over sales. "solutions architect" stays in architecture (above).
	{"solutions engineer", "solutions_engineering"},
}

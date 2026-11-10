package classify

// coreCategoryTable1 is part 1 of 17 of coreCategoryTable; the parts concatenate in order.
var coreCategoryTable1 = []aliasEntry{
	{"full stack", "fullstack"},
	{"full-stack", "fullstack"},
	{"fullstack", "fullstack"},
	{"фулстек", "fullstack"},
	{"фуллстак", "fullstack"},
	{"data engineer", "data_engineering"},
	{"data engineering", "data_engineering"},
	{"дата-инженер", "data_engineering"},
	{"инженер данных", "data_engineering"},
	// Platform/governance/stewardship work on the data estate itself, distinct from
	// the analytics-facing "analytics engineer" below and from generic "devops".
	{"data platform", "data_engineering"},
	{"data governance", "data_engineering"},
	{"data steward", "data_engineering"},
	{"etl developer", "data_engineering"},
	// Hungarian: "adattárház" is the data warehouse.
	{"adattárház fejlesztő", "data_engineering"},
	{"data scientist", "data_science"},
	{"data science", "data_science"},
	// "data scien" fires only on a title truncated mid-word ("Senior Data Scien…"),
	// which ATS feeds produce: the full forms above win the boundary check in any
	// complete title, so this alias is their truncated-tail recall, not a duplicate.
	{"data scien", "data_science"},
	{"дата-сайентист", "data_science"},
	{"data analyst", "data_analytics"},
	{"data analytics", "data_analytics"},
	// dbt-era title: builds governed, tested data models for analysts/BI to consume —
	// analytics-facing output, unlike the raw pipeline work in data_engineering above.
	{"analytics engineer", "data_analytics"},
	{"аналитик данных", "data_analytics"},
	{"data аналитик", "data_analytics"},
	// Hungarian, noun and adjective form. Both are closed compounds on "adat" (data)
	// and neither has a non-analytical sense in the live sample.
	{"adatelemző", "data_analytics"},
	{"adatelemzési", "data_analytics"},
	// BI is reporting/dashboards/metrics — the analytics side, so it routes here
	// rather than to a thin bi_analytics category.
	{"business intelligence analyst", "data_analytics"},
	{"bi analyst", "data_analytics"},
	{"business intelligence developer", "data_analytics"},
	{"bi developer", "data_analytics"},
	{"power bi developer", "data_analytics"},
	{"аналитик bi", "data_analytics"},
	{"bi-аналитик", "data_analytics"},
	// Hungarian. "riportfejlesztő" is a closed compound, so "bi fejlesztő" cannot
	// reach it on a word boundary.
	{"bi fejlesztő", "data_analytics"},
	{"bi riportfejlesztő", "data_analytics"},
	// Classic ML and explicitly ML-carrying combined forms first, so a mixed
	// "ML/AI Engineer" resolves to ml_ai before the bare AI terms below can claim it.
	{"machine learning", "ml_ai"},
	{"deep learning", "ml_ai"},
	// Classic ML sub-disciplines that name neither "machine learning" nor "ai" —
	// unambiguous on their own, so they resolve here rather than falling to the
	// generic software_engineering catch-all at the bottom of the table.
	{"computer vision engineer", "ml_ai"},
	{"nlp engineer", "ml_ai"},
	{"ml engineer", "ml_ai"},
	// Same reason as the "<discipline> developer" spellings below: redundant for
	// tagging ("machine learning" already resolves it), needed by search, where the
	// label reads "ML Engineer" and the query "machine learning engineer" reached
	// nothing. 7,199 open postings carry the full phrase.
	{"machine learning engineer", "ml_ai"},
	{"ml/ai", "ml_ai"},
	{"ai/ml", "ml_ai"},
	// AI-application terms (RAG/agents/LLM/prompt/applied AI) → ai_engineering.
	{"generative ai", "ai_engineering"},
	{"genai", "ai_engineering"},
	{"llm engineer", "ai_engineering"},
	{"prompt engineer", "ai_engineering"},
	{"applied ai", "ai_engineering"},
	{"rag engineer", "ai_engineering"},
	// AI titles whose two anchor words are separated by a noun: "ai engineer" below
	// matches only adjacent words, so each spread form is its own alias. Agent work,
	// AI-product work and workflow automation all BUILD ON models rather than train
	// them, which is what separates ai_engineering from ml_ai above. "AI Research
	// Engineer" is the borderline case — deliberately applied, since outside the
	// labs it names product research on existing models.
	{"ai product engineer", "ai_engineering"},
	{"ai agent engineer", "ai_engineering"},
	{"agent engineer", "ai_engineering"},
	{"ai research engineer", "ai_engineering"},
	{"ai software engineer", "ai_engineering"},
	{"ai automation engineer", "ai_engineering"},
	// Hyphenated spellings. A hyphen is a word boundary, so the spaced aliases above
	// cannot reach them. "ai-agent engineer" needs no entry — the bare "agent engineer"
	// already matches it, the hyphen serving as its left boundary. Every alias stays
	// anchored on the role noun so "Algebrik.ai-Product Manager" (a domain followed by
	// "product manager") is not read as an AI-product role.
	{"ai-product engineer", "ai_engineering"},
	{"ai-research engineer", "ai_engineering"},
	{"ai-automation engineer", "ai_engineering"},
	{"ai engineer", "ai_engineering"},
	{"llm", "ai_engineering"},
	{"devops", "devops"},
	{"девопс", "devops"},
	{"platform engineer", "devops"},
	// The discipline/team-noun form: "Platform Engineering Team Leader" carries no
	// "platform engineer" substring, so it needs its own entry.
	{"platform engineering", "devops"},
	{"infrastructure engineer", "devops"},
	// "it infrastructure" belongs to this category but is NOT listed here — it names
	// an estate, not a role, so it resolves with the other domain nouns further down,
	// below the technical role nouns. Search for it there.
	{"cloud engineer", "devops"},
	// The estate-facing analyst titles: 82 and 18 live postings.
	{"infrastructure analyst", "devops"},
	{"cloud analyst", "devops"},
	{"system administrator", "devops"},
	// The plural is the far more common surface form in prod titles ("Systems
	// Administrator") and does not contain "system administrator" as a substring
	// (the trailing "s" breaks the word boundary), so it needs its own entry.
	{"systems administrator", "devops"},
	{"sysadmin", "devops"},
	{"database administrator", "devops"},
	{"linux administrator", "devops"},
	{"windows administrator", "devops"},
	{"it administrator", "devops"},
	// Hungarian. "rendszergazda" is the sysadmin, a closed compound with no second
	// sense: all 17 occurrences among the 7,207 open Hungarian postings are this role.
	//
	// The operator noun "üzemeltető" is the "fejlesztő" problem again and is NOT here
	// bare — it is whoever runs a thing, and the same sample spends it on machines
	// ("Gépész üzemeltető"), electrical plant ("Villamos Üzemeltető"), buildings
	// ("Épületüzemeltetési mérnök"), facilities ("Létesítményüzemeltetési koordinátor")
	// and fuel stations. Only the qualified forms resolve. Hungarian writes both the
	// closed compound and the spaced pair and prod carries both spellings of each, so
	// both are listed: a closed compound never contains its spaced twin on a word
	// boundary, which is also why "szoftverüzemeltető" needs its own entry.
	{"rendszergazda", "devops"},
	{"rendszerüzemeltető", "devops"},
	{"alkalmazásüzemeltető", "devops"},
	{"alkalmazás üzemeltető", "devops"},
	{"szoftverüzemeltető", "devops"},
	{"it üzemeltető", "devops"},
	{"informatikai üzemeltető", "devops"},
	{"infrastruktúra üzemeltető", "devops"},
	// MLOps is DevOps practice specialized to ML artifacts (CI/CD, deployment,
	// monitoring for models) — the operational lifecycle, not the modeling itself,
	// which stays in ml_ai/ai_engineering above.
	{"mlops", "devops"},
	{"ml ops", "devops"},
	{"sre", "sre"},
	{"site reliability", "sre"},
	{"network engineer", "network_engineering"},
	{"network engineering", "network_engineering"},
	{"network administrator", "network_engineering"},
	{"сетевой инженер", "network_engineering"},
	{"сетевой администратор", "network_engineering"},
	// Hungarian. Neither the bare adjective "hálózati" nor the compound
	// "hálózatüzemeltető" is here: a "hálózat" is ANY network. The adjective covers a
	// retail chain's ("Országos Üzlethálózati Tréner") and the compound covers a gas
	// utility's — three of its four live occurrences are "Hálózatüzemeltető
	// /Gázszerelő" at OPUS TIGÁZ, a gas fitter, and the fourth says "Cisco", which the
	// dictionary reads on its own. Only the engineering seats resolve.
	{"hálózati mérnök", "network_engineering"},
	{"hálózati rendszermérnök", "network_engineering"},
	// The back end of a can plant, a body shop or a store is an area of the site, and
	// the trade working it says so: Crown Holdings' "Back End Operator/Mechanic/
	// Maintainer", a collision shop's "Back-End Estimator", a "Backend Retail
	// Assistant". Each is filed with its English twin, ahead of the bare alias below —
	// the "front end cashier" pins' shape. The SHIFT spelling ("Back End Days") is not
	// a pin but a cut (backEndShiftPatterns), because the role beside it varies. A
	// bare "back-end associate" is deliberately absent: "associate" names no category
	// of its own, and prod carries "Back-end Developer Associate" too.
	{"back end operator", "manufacturing"},
	{"back-end operator", "manufacturing"},
	{"back end mechanic", "skilled_trades"},
	{"back-end mechanic", "skilled_trades"},
	{"back end maintainer", "skilled_trades"},
	{"back-end maintainer", "skilled_trades"},
	{"back end estimator", "skilled_trades"},
	{"back-end estimator", "skilled_trades"},
	{"back end collision estimator", "skilled_trades"},
	{"back-end collision estimator", "skilled_trades"},
	{"backend retail", "retail"},
	{"back end retail", "retail"},
	{"back-end retail", "retail"},
	{"backend", "backend"},
	{"back-end", "backend"},
	{"back end", "backend"},
	{"бэкенд", "backend"},
	{"бекенд", "backend"},
	// The "<discipline> developer" spellings are redundant for tagging — the bare
	// alias above already resolves them — and exist for the search side, which reads
	// this table through CategoryAliases. Its matcher needs every word of the query to
	// appear, so "backend developer" reaches nothing when the only alias is "backend"
	// and the label says "Engineer". Measured: 8,870 open postings titled that way,
	// and the query returned no suggestion at all.
	{"backend developer", "backend"},
	{"back-end developer", "backend"},
	// In a supermarket the "front end" is the checkout lane, and a grocery chain's
	// "Front End Cashier" is the single largest spelling the bare alias below would
	// file as a frontend developer. These were harmless while the ingest filter turned
	// every cashier away; once a categorised title is kept, they would reach the
	// catalogue as technical. The heavy-equipment "front end loader" is the same trap.
	{"front end cashier", "retail"},
	{"front-end cashier", "retail"},
	{"front end clerk", "retail"},
	{"front end assistant", "retail"},
	{"front end associate", "retail"},
	{"front end loader", "skilled_trades"},
	// In chip design the "front end" is the RTL half of the flow, not a web UI:
	// "ASIC Research Engineer - Front-End Digital Design".
	{"front-end digital design", "hardware"},
	{"front end digital design", "hardware"},
	{"front-end asic", "hardware"},
	{"front end asic", "hardware"},
	// The same stage named by the silicon or radio it belongs to — "RF Front-End
	// Design Engineer", "IC Front-End Architect", "CPU (Front-End) Verification" — or
	// by a step of the chip flow after it ("Front-End CAD", "Front End RTL Designer").
	// The qualifier must sit right beside the word: "Software Engineer Frontend IC" is
	// an individual contributor and "Frontend Developer — RTL UI" right-to-left text.
	// "Digital front-end", "front-end design engineer" and "front-end implementation"
	// stay web — prod carries "Digital Frontend Developer", "Front-End Design Engineer -
	// UX/UI" and "Camera Frontend Implementation Engineer" beside the chip spellings.
	{"rf front-end", "hardware"},
	{"rf front end", "hardware"},
	{"rf frontend", "hardware"},
	{"asic front-end", "hardware"},
	{"asic front end", "hardware"},
	{"asic frontend", "hardware"},
	{"asic (front-end)", "hardware"},
	{"asic (front end)", "hardware"},
	{"rtl front-end", "hardware"},
	{"rtl front end", "hardware"},
	{"rtl frontend", "hardware"},
	{"ic front-end", "hardware"},
	{"ic front end", "hardware"},
	{"ic frontend", "hardware"},
	{"soc front-end", "hardware"},
	{"soc front end", "hardware"},
	{"soc frontend", "hardware"},
	{"cpu front-end", "hardware"},
	{"cpu front end", "hardware"},
	{"cpu frontend", "hardware"},
	{"cpu (front-end)", "hardware"},
	{"cpu (front end)", "hardware"},
	{"gpu front-end", "hardware"},
	{"gpu front end", "hardware"},
	{"gpu frontend", "hardware"},
	{"vlsi front-end", "hardware"},
	{"vlsi front end", "hardware"},
	{"vlsi frontend", "hardware"},
	{"silicon front-end", "hardware"},
	{"silicon front end", "hardware"},
	{"silicon frontend", "hardware"},
	{"semiconductor front-end", "hardware"},
	{"semiconductor front end", "hardware"},
	{"semiconductor frontend", "hardware"},
	{"hardware front-end", "hardware"},
	{"hardware front end", "hardware"},
	{"hardware frontend", "hardware"},
	{"front-end silicon", "hardware"},
	{"front end silicon", "hardware"},
	{"frontend silicon", "hardware"},
	{"front-end semiconductor", "hardware"},
	{"front end semiconductor", "hardware"},
	{"frontend semiconductor", "hardware"},
	{"front-end cad", "hardware"},
	{"front end cad", "hardware"},
	{"frontend cad", "hardware"},
	{"front-end methodology", "hardware"},
	{"front end methodology", "hardware"},
	{"frontend methodology", "hardware"},
	{"front-end verification", "hardware"},
	{"front end verification", "hardware"},
	{"frontend verification", "hardware"},
	{"front-end design verification", "hardware"},
	{"front end design verification", "hardware"},
	{"frontend design verification", "hardware"},
	{"front-end design/verification", "hardware"},
	{"front end design/verification", "hardware"},
	{"frontend design/verification", "hardware"},
	{"front-end design (rtl)", "hardware"},
	{"front end design (rtl)", "hardware"},
	{"frontend design (rtl)", "hardware"},
	{"front-end rtl", "hardware"},
	{"front end rtl", "hardware"},
	{"frontend rtl", "hardware"},
	{"frontend", "frontend"},
	{"front-end", "frontend"},
	{"front end", "frontend"},
	{"фронтенд", "frontend"},
	{"фронт", "frontend"},
	{"frontend developer", "frontend"},
	{"front-end developer", "frontend"},
	// Frontend-only frameworks named in a "<Framework> Developer" title — the
	// framework itself states the discipline, so this is not a guess the way a
	// bare language ("Java Developer") would be.
	{"react developer", "frontend"},
	{"react.js developer", "frontend"},
	{"reactjs developer", "frontend"},
	{"angular developer", "frontend"},
	{"vue developer", "frontend"},
	{"vue.js developer", "frontend"},
	{"vuejs developer", "frontend"},
	// NO bare "mobile", and no bare "мобильный"/"мобильная"/"мобильных" — the same trap
	// the bare "analyst" fall-through sprang, measured on the live catalogue 2026-09-18.
	// Outside software, "mobile" is the word for work that TRAVELS to the customer, and
	// that meaning is far the commoner one: of 28 650 open postings the bare alias held,
	// 18 483 named no mobile platform at all. It was claiming a phone carrier's shop
	// floor (4 500+ "Mobile Associate - Retail Sales"), a bank's travelling
	// representative (1 053 "Мобильный банкир"), field service ("Mobile Service
	// Technician", "Mobile Diesel Technician"), facilities ("Mobile Building Engineer")
	// and clinicians who drive to the patient ("Mobile Phlebotomist", "Mobile X-Ray
	// Technologist") — and because `mobile` is a technical category, `is_tech` followed
	// it, so 1 538 retail-sales postings were enqueued for LLM enrichment as engineering.
	//
	// The qualified spellings below are DERIVED from that catalogue rather than imagined,
	// and they are contiguous phrases on purpose: "Mobile Building Engineer" and "Mobile
	// Maintenance Engineer" both carry "mobile" and "engineer" without ever carrying
	// "mobile engineer", so phrase matching declines them for free where a bare word
	// could not. A title that names the platform ("Android", "iOS", "React Native",
	// "Flutter") is answered by its own alias below and needs nothing here.
	//
	// What this deliberately gives up: "Software Engineer (Mobile)" and its ~60 siblings,
	// which put the qualifier after the noun, now resolve to `software_engineering`
	// instead. That is less specific and still true — and still technical — which is the
	// trade this vocabulary always makes over guessing.
	{"mobile developer", "mobile"},
}

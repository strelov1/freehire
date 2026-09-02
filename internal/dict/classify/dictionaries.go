package classify

// aliasEntry pairs a lowercase title alias with its enrich-vocabulary canonical.
// The ordered table is the single source of truth: its order encodes precedence
// (the most specific / highest-rank alias first, so a title carrying several terms
// resolves the stronger one) and the canonical is read straight off the entry, so
// the alias set and its mapping cannot drift out of parity. Aliases are lowercase;
// multi-word and hyphenated forms are explicit.
type aliasEntry struct {
	alias     string
	canonical string
}

// gradeBlindPhrases are role names that CONTAIN a seniority word without stating a
// grade: "Member of Technical Staff" is the generic IC title at Oracle/xAI/Pure
// Storage, not the staff grade, and "Lead Generation" names a marketing function.
// They are cut from the title before the seniority match, so the remaining words
// state the real grade — "Senior Member of Technical Staff" is senior, and a bare
// MTS carries no grade at all. Without the mask the table's precedence made this
// actively wrong: "staff" outranks "senior", so SMTS resolved to staff.
//
// Only phrases that shadow a seniorityTable alias belong here — the abbreviations
// (MTS/SMTS/DMTS) contain no grade word, so masking them would remove nothing.
// The longer form is listed first so it is cut whole.
var gradeBlindPhrases = []string{
	"member of the technical staff",
	"member of technical staff",
	"lead generation",
	// A model-training stage, the sibling of pre-/post-training. AI labs post
	// "Member of Technical Staff, Mid-training"; the hyphen is a word boundary, so
	// the bare "mid" alias matched inside it and read the posting as middle grade.
	"mid-training",
	// A region, not a grade — and the costliest of these by far: 142 of 217 prod
	// titles carrying "Middle East" were graded middle on the geography alone.
	"middle east",
	// The hyphenated spellings of the two phrases above. A hyphen is a word boundary,
	// so the spaced forms cannot mask them and the grade word inside stays exposed
	// ("Enterprise Account Executive (Middle-East & Africa)" read as middle).
	"middle-east",
	"lead-generation",
}

// categoryNone is the sentinel canonical for a "blind" alias: a phrase that CONTAINS a
// categoryTable alias while naming no category of its own. "Software Design Engineer"
// is software engineering — "design" qualifies what is engineered, it is not the craft
// — and this vocabulary has no value for a software generalist (a bare "Staff Software
// Engineer" resolves none either). Emitting nothing is the honest answer; is_tech comes
// from the tech-title detector instead.
//
// It is a table entry rather than a pre-match mask on purpose. A mask (the shape
// gradeBlindPhrases uses for grades) is wrong here on both counts: cutting the span
// would expose whatever alias sits further down the table, so "Software Design
// Engineer - Sales Tools" resolved to `sales` and lost its enrichment, and cutting is
// boundary-blind where every matcher in this package is boundary-aware. As an entry it
// simply wins the first-match walk, and matchCategory translates it to "".
//
// Only phrases with no better category belong here. Where one exists, route the title
// to it instead — "cloud design engineer" → devops, "design engineer in test" → qa.
// And keep the phrases narrow: "systems design engineer" was listed here once, which
// blanked the category of every "HVAC/Mechanical Systems Design Engineer" and, with it,
// the placement that vetoes deletion in ConfirmedNonTech.
const categoryNone = "-"

// seniorityTable lists seniority aliases in precedence order (most specific /
// highest rank first), each paired with its vocab.SeniorityValues canonical.
var seniorityTable = []aliasEntry{
	{"head of", "c_level"},
	{"chief", "c_level"},
	{"cto", "c_level"},
	{"cpo", "c_level"},
	{"ceo", "c_level"},
	{"vp", "c_level"},
	{"vice president", "c_level"},
	{"директор", "c_level"},
	{"руководитель", "c_level"},
	{"principal", "principal"},
	{"staff", "staff"},
	{"lead", "lead"},
	{"ведущий", "lead"},
	{"тимлид", "lead"},
	{"teamlead", "lead"},
	{"team lead", "lead"},
	{"senior", "senior"},
	// "sr" already covers the dotted "Sr." form: '.' is a non-word boundary, so a
	// separate "sr." alias could never match anything "sr" does not.
	{"sr", "senior"},
	{"старший", "senior"},
	{"синьор", "senior"},
	{"сеньор", "senior"},
	{"middle", "middle"},
	{"mid", "middle"},
	{"mid-level", "middle"},
	{"mid level", "middle"},
	{"средний", "middle"},
	{"мидл", "middle"},
	{"junior", "junior"},
	// Like "sr", the bare "jr" covers "Jr." — a dotted alias would be dead.
	{"jr", "junior"},
	{"младший", "junior"},
	{"джуниор", "junior"},
	{"джун", "junior"},
	{"intern", "intern"},
	{"internship", "intern"},
	{"trainee", "intern"},
	{"стажёр", "intern"},
	{"стажер", "intern"},
	{"стажировка", "intern"},
}

// categoryTable lists category aliases in precedence order — multi-word and more
// specific terms first, so "data analyst" wins over a bare "data" and "fullstack"
// is not shadowed by "backend"/"frontend" — each paired with its
// vocab.CategoryValues canonical.
var categoryTable = []aliasEntry{
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
	// BI is reporting/dashboards/metrics — the analytics side, so it routes here
	// rather than to a thin bi_analytics category.
	{"business intelligence analyst", "data_analytics"},
	{"bi analyst", "data_analytics"},
	{"business intelligence developer", "data_analytics"},
	{"bi developer", "data_analytics"},
	{"power bi developer", "data_analytics"},
	{"аналитик bi", "data_analytics"},
	{"bi-аналитик", "data_analytics"},
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
	{"cloud engineer", "devops"},
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
	{"mobile", "mobile"},
	{"android", "mobile"},
	{"ios", "mobile"},
	// React Native is mobile-only, unlike bare "react" above.
	{"react native developer", "mobile"},
	{"мобильный", "mobile"},
	{"мобильная", "mobile"},
	{"мобильных", "mobile"},
	// Penetration-testing titles must precede the QA block's bare "tester" fall-through
	// right below — it would otherwise claim "Penetration Tester" for qa.
	{"penetration tester", "security"},
	{"penetration testing", "security"},
	{"pentester", "security"},
	{"pentest", "security"},
	{"qa", "qa"},
	{"quality assurance", "qa"},
	{"tester", "qa"},
	{"test engineer", "qa"},
	{"test automation", "qa"},
	{"sdet", "qa"},
	{"тестировщик", "qa"},
	{"тестирование", "qa"},
	// Physical security is not information security, and the bare alias below cannot
	// tell them apart: "Security Guard" and "Security Officer" have been resolving to
	// the infosec facet, which is wrong in both directions — a guard is not findable
	// where guards are looked for, and an infosec filter returns him.
	// Physical security is not information security, and the bare alias below cannot
	// tell them apart: "Security Guard" was resolving to the infosec facet, wrong in
	// both directions — a guard is not findable where guards are looked for, and an
	// infosec filter returned him.
	//
	// "Security Officer" is deliberately NOT here, though it is a common guard title.
	// `Categories()` returns EVERY matching alias rather than the strongest, so an
	// entry for it tagged "Chief Information Security Officer" with `personal_services`
	// on the multi-category CV path no matter what order the table declares — ordering
	// only decides `Parse`. The phrase is genuinely ambiguous, so it is dropped rather
	// than guessed, which is the same call `design systems` and bare `engineer` got.
	{"security guard", "personal_services"},
	{"armed guard", "personal_services"},
	{"security", "security"},
	{"infosec", "security"},
	{"appsec", "security"},
	{"cybersecurity", "security"},
	{"cyber security", "security"},
	{"безопасность", "security"},
	{"безопасности", "security"},
	{"кибербезопасность", "security"},
	// Narrower technical niches within security. Deliberately no bare "compliance" —
	// sampled live titles are dominated by non-IT banking/legal/customs compliance
	// (that population already routes to `legal` via "compliance officer/manager/
	// analyst" above); a bare entry here would be a GTM-style word-trap.
	{"iam", "security"},
	{"identity and access management", "security"},
	{"grc", "security"},
	{"vulnerability management", "security"},
	{"vulnerability analyst", "security"},
	{"incident response", "security"},
	{"red team", "security"},
	{"red teamer", "security"},
	{"blue team", "security"},
	{"threat intelligence", "security"},
	{"threat intel", "security"},
	// "chief information security officer" needs no entry of its own: the bare
	// "security" alias above already catches it as a whole word.
	{"ciso", "security"},
	// IT-specific audit (controls/access/compliance review), unlike the bare
	// "internal auditor"/"auditor" nouns this file deliberately omits — those span
	// every industry's financial and quality audit functions and are not IT-anchored.
	{"it auditor", "security"},
	// DevSecOps stays security, not devops: the security responsibility (SAST/DAST,
	// container/IaC scanning, policy-as-code) is why the title exists, not incidental.
	{"devsecops", "security"},
	{"embedded", "embedded"},
	{"firmware", "embedded"},
	{"встраиваемые", "embedded"},
	{"встраиваемых", "embedded"},
	// French: "logiciel embarqué" is embedded software, unambiguous.
	{"logiciel embarqué", "embedded"},
	{"blockchain", "blockchain"},
	{"блокчейн", "blockchain"},
	// "Web3"/"smart contract" name the blockchain domain as unambiguously as the
	// word "blockchain" itself, so these resolve here rather than the generic
	// software_engineering catch-all.
	{"web3 developer", "blockchain"},
	{"smart contract developer", "blockchain"},
	{"hardware", "hardware"},
	{"fpga", "hardware"},
	{"solutions architect", "architecture"},
	{"software architect", "architecture"},
	{"enterprise architect", "architecture"},
	{"cloud architect", "architecture"},
	{"architect", "architecture"},
	{"архитектор", "architecture"},
	// technical_writing before design/ux so "UX Writer"/"Content Designer" (product
	// documentation & content craft) win over the bare "ux"/"designer" design entries.
	// Promotional "copywriter"/"content writer" deliberately stay in marketing (below).
	{"technical writer", "technical_writing"},
	{"technical writing", "technical_writing"},
	{"technical communicator", "technical_writing"},
	{"documentation specialist", "technical_writing"},
	{"documentation manager", "technical_writing"},
	{"documentation engineer", "technical_writing"},
	{"information developer", "technical_writing"},
	{"content designer", "technical_writing"},
	{"ux writer", "technical_writing"},
	{"content strategist", "technical_writing"},
	{"localization specialist", "technical_writing"},
	{"localization manager", "technical_writing"},
	{"localization engineer", "technical_writing"},
	// Instructional/curriculum design is a content craft, not product design — it
	// builds courses and learning materials, not interfaces — so it must precede the
	// bare "designer" entry below for the same reason "content designer" does. It
	// stays out of `hr`'s "learning and development" alias on purpose: L&D is the
	// internal-training FUNCTION, this is the authoring CRAFT, and it shows up at
	// edtech/product companies with no L&D team at all.
	{"instructional designer", "technical_writing"},
	{"instructional design", "technical_writing"},
	{"curriculum designer", "technical_writing"},
	{"learning designer", "technical_writing"},
	{"learning experience designer", "technical_writing"},
	{"e-learning developer", "technical_writing"},
	{"elearning developer", "technical_writing"},
	// Bare "translator"/"переводчик": the role noun names one unambiguous craft in
	// every industry, so no qualifying phrase is needed the way
	// software_engineering's language-anchored forms are. NOT "translation" — that
	// noun also names an NLP/MT discipline ("Machine Translation Engineer",
	// "Translation Engineer" are ml_ai/software roles, not human translators), so a
	// bare entry for it would misfile them the same way a bare "growth"/"compliance"
	// would elsewhere in this file.
	{"translator", "technical_writing"},
	{"технический писатель", "technical_writing"},
	{"техписатель", "technical_writing"},
	{"технический редактор", "technical_writing"},
	{"разработчик документации", "technical_writing"},
	{"специалист по документации", "technical_writing"},
	{"ux-редактор", "technical_writing"},
	{"переводчик", "technical_writing"},
	// The word "design" names two unrelated crafts. Everything below down to the
	// engineering block is a title whose "… design …" is NOT product design: it is
	// engineering draughting (mechanical/electrical/civil), chip and board design, or
	// a network role. They must precede the bare "designer"/"design" entries, which
	// would otherwise claim them by virtue of the word alone — the defect this split
	// fixes (a mining-equipment "Design Engineer" filed under product design).
	//
	// First, the titles that state a craft of their own and must NOT go to the
	// engineering-design bucket the next block builds.
	{"software design engineer in test", "qa"},
	{"design engineer in test", "qa"},
	{"software design engineer", categoryNone},
	{"software design engineering", categoryNone},
	{"network design engineer", "network_engineering"},
	{"cloud design engineer", "devops"},
	{"solution design engineer", "solutions_engineering"},
	{"solutions design engineer", "solutions_engineering"},
	// Silicon and board design belong to `hardware`, which already owns the rest of
	// that team through the earlier "hardware"/"fpga" aliases. Routing them to
	// engineering draughting would split one discipline across two facets and drop
	// them out of the technical treatment (enrichment, embeddings) they have today.
	// The list has to name the whole family: whatever is missing here falls through to
	// the bare "design engineer" at the bottom of the block and lands in draughting.
	{"pcb design", "hardware"},
	{"pcb designer", "hardware"},
	{"pcb layout", "hardware"},
	{"physical design engineer", "hardware"},
	{"analog design engineer", "hardware"},
	{"rtl design engineer", "hardware"},
	{"mixed signal design engineer", "hardware"},
	// The hyphenated spelling is the industry's own and a hyphen is a word boundary, so
	// it needs its own entry — the same trap "middle-east" and "ai-product engineer"
	// document elsewhere in this file.
	{"mixed-signal design engineer", "hardware"},
	{"digital design engineer", "hardware"},
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
	{"engineering manager", "management"},
	{"team manager", "management"},
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
	// — the generic "manager" alias further down claimed them for `management`. Each
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
	{"solution engineer", "solutions_engineering"},
	{"sales engineer", "solutions_engineering"},
	{"presales engineer", "solutions_engineering"},
	{"pre-sales engineer", "solutions_engineering"},
	{"solutions consultant", "solutions_engineering"},
	{"solution consultant", "solutions_engineering"},
	{"sales applications engineer", "solutions_engineering"},
	{"forward deployed engineer", "solutions_engineering"},
	{"пресейл", "solutions_engineering"},
	{"пресейл-инженер", "solutions_engineering"},
	// GTM engineering builds the outbound data pipeline rather than selling, but the
	// split from RevOps is not title-separable, so it rides with the rest of that
	// cluster. Only the phrase resolves — the bare "gtm" names Google Tag Manager in
	// a requirements list, and that meaning belongs to the skill dictionary.
	{"gtm engineer", "sales"},
	{"go-to-market engineer", "sales"},
	{"go to market engineer", "sales"},
	{"sales", "sales"},
	{"account executive", "sales"},
	{"business development", "sales"},
	{"account manager", "sales"},
	{"sdr", "sales"},
	{"bdr", "sales"},
	// RevOps/Sales Ops belong to the GTM cluster; the finance-side rev-rec meaning is
	// not title-separable, so the commercial default routes here (not to finance/ops).
	{"revenue operations", "sales"},
	{"revops", "sales"},
	{"sales operations", "sales"},
	{"продажи", "sales"},
	{"продаж", "sales"},
	{"продажам", "sales"},
	{"support", "support"},
	{"customer service", "support"},
	{"help desk", "support"},
	// The unspaced compound: "help desk" above cannot reach it, the same trap the
	// "back-end"/"back end"/"backend" trio guards against.
	{"helpdesk", "support"},
	{"call center", "support"},
	{"call-центр", "support"},
	{"колл-центр", "support"},
	{"contact center", "support"},
	{"customer care", "support"},
	{"поддержка", "support"},
	{"поддержки", "support"},
	{"техподдержка", "support"},
	{"техподдержки", "support"},
	// customer_success (proactive post-sale: adoption/renewals) is distinct from the
	// reactive helpdesk "support" above; "account manager" stays in sales.
	{"customer success", "customer_success"},
	{"client success", "customer_success"},
	{"customer onboarding", "customer_success"},
	{"onboarding specialist", "customer_success"},
	{"onboarding manager", "customer_success"},
	{"implementation specialist", "customer_success"},
	{"implementation consultant", "customer_success"},
	// NOT "onboarding engineer": unlike "implementation engineer" above (still a
	// customer-facing role, the technical sibling of "implementation
	// specialist"/"consultant"), "onboarding engineer" is genuinely dual-use — it is
	// also a real internal-platform title ("Developer Onboarding Engineer" builds
	// onboarding tooling for a company's own engineers), so a bare entry would
	// misfile that population the way a bare "growth"/"compliance" would elsewhere
	// in this file.
	{"implementation engineer", "customer_success"},
	{"renewals manager", "customer_success"},
	{"renewal manager", "customer_success"},
	{"менеджер по успеху клиентов", "customer_success"},
	{"менеджер по работе с клиентами", "customer_success"},
	{"специалист по адаптации клиентов", "customer_success"},
	// IT-company back-office roles. All are multi-word/anchored and placed ABOVE the
	// terminal "manager"→management and "analyst"→data_analytics fall-throughs so a
	// functional title ("Financial Analyst", "Operations Manager") is not stolen by
	// them. hr precedes operations so "People Operations Manager" → hr, not operations.
	{"recruiter", "recruiting"},
	{"tech recruiter", "recruiting"},
	{"technical recruiter", "recruiting"},
	{"it recruiter", "recruiting"},
	{"talent acquisition", "recruiting"},
	{"talent sourcer", "recruiting"},
	{"recruitment consultant", "recruiting"},
	{"recruitment specialist", "recruiting"},
	{"sourcer", "recruiting"},
	{"talent partner", "recruiting"},
	// Employer branding sells the company to candidates, not customers — the
	// recruiting-side twin of the marketing "brand manager" alias above, which this
	// phrase does not contain a substring of ("employer branding manager" has no
	// "brand manager" inside it), so it needs its own entry rather than falling to
	// the generic manager->management catch-all.
	{"employer branding", "recruiting"},
	{"рекрутер", "recruiting"},
	{"рекрутёр", "recruiting"},
	{"специалист по подбору персонала", "recruiting"},
	{"human resources", "hr"},
	{"hr manager", "hr"},
	{"hr generalist", "hr"},
	{"hr business partner", "hr"},
	{"hrbp", "hr"},
	{"people partner", "hr"},
	{"people operations", "hr"},
	{"people ops", "hr"},
	{"hr director", "hr"},
	{"head of people", "hr"},
	{"chro", "hr"},
	{"learning and development", "hr"},
	{"compensation and benefits", "hr"},
	{"менеджер по персоналу", "hr"},
	{"специалист по персоналу", "hr"},
	{"директор по персоналу", "hr"},
	{"эйчар", "hr"},
	{"chief financial officer", "finance"},
	{"cfo", "finance"},
	{"head of finance", "finance"},
	{"financial controller", "finance"},
	{"finance controller", "finance"},
	{"financial analyst", "finance"},
	{"finance analyst", "finance"},
	{"fp&a", "finance"},
	{"accountant", "finance"},
	{"accounting", "finance"},
	{"accounts payable", "finance"},
	{"accounts receivable", "finance"},
	{"bookkeeper", "finance"},
	{"payroll", "finance"},
	{"treasury", "finance"},
	{"tax accountant", "finance"},
	{"finance manager", "finance"},
	{"financial manager", "finance"},
	{"финансовый директор", "finance"},
	{"главный бухгалтер", "finance"},
	{"главбух", "finance"},
	{"бухгалтер", "finance"},
	{"финансовый аналитик", "finance"},
	{"казначей", "finance"},
	{"general counsel", "legal"},
	{"legal counsel", "legal"},
	{"corporate counsel", "legal"},
	{"legal manager", "legal"},
	{"legal assistant", "legal"},
	{"lawyer", "legal"},
	{"attorney", "legal"},
	{"paralegal", "legal"},
	{"contract manager", "legal"},
	{"contracts manager", "legal"},
	{"compliance officer", "legal"},
	{"compliance manager", "legal"},
	{"compliance analyst", "legal"},
	{"regulatory affairs", "legal"},
	{"data protection officer", "legal"},
	{"privacy officer", "legal"},
	{"data privacy officer", "legal"},
	{"data privacy manager", "legal"},
	{"contracts administrator", "legal"},
	{"contract administrator", "legal"},
	{"юрист", "legal"},
	{"юрисконсульт", "legal"},
	{"корпоративный юрист", "legal"},
	{"комплаенс", "legal"},
	{"chief operating officer", "operations"},
	{"coo", "operations"},
	{"chief of staff", "operations"},
	{"business operations", "operations"},
	{"biz ops", "operations"},
	{"operations manager", "operations"},
	{"operations analyst", "operations"},
	{"operations coordinator", "operations"},
	{"operations specialist", "operations"},
	{"ops manager", "operations"},
	{"head of operations", "operations"},
	{"office manager", "operations"},
	{"executive assistant", "operations"},
	{"administrative assistant", "operations"},
	{"procurement", "operations"},
	{"procurement manager", "operations"},
	{"purchasing manager", "operations"},
	{"facilities manager", "operations"},
	{"facilities coordinator", "operations"},
	{"операционный директор", "operations"},
	{"операционный менеджер", "operations"},
	{"офис-менеджер", "operations"},
	{"ассистент руководителя", "operations"},
	{"помощник руководителя", "operations"},
	{"специалист по закупкам", "operations"},
	{"закупщик", "operations"},
	{"business analyst", "business_analysis"},
	{"business systems analyst", "business_analysis"},
	{"business system analyst", "business_analysis"},
	{"systems analyst", "business_analysis"},
	{"system analyst", "business_analysis"},
	{"business process analyst", "business_analysis"},
	{"process analyst", "business_analysis"},
	{"requirements analyst", "business_analysis"},
	{"functional analyst", "business_analysis"},
	{"it business analyst", "business_analysis"},
	{"business analysis", "business_analysis"},
	{"бизнес-аналитик", "business_analysis"},
	{"бизнес аналитик", "business_analysis"},
	{"системный аналитик", "business_analysis"},
	{"аналитик требований", "business_analysis"},
	{"аналитик бизнес-процессов", "business_analysis"},
	{"developer advocate", "developer_relations"},
	{"developer relations", "developer_relations"},
	{"devrel", "developer_relations"},
	{"developer evangelist", "developer_relations"},
	{"technical evangelist", "developer_relations"},
	{"developer experience engineer", "developer_relations"},
	{"developer community manager", "developer_relations"},
	{"деврел", "developer_relations"},
	{"технический евангелист", "developer_relations"},
	// Community management is marketing's, but only once developer relations has had
	// its say: a "Community Manager, Developer Relations" runs a developer community,
	// not a brand's social presence. Sits below the DevRel block for that reason and
	// above the bare "manager" so the unqualified title still resolves.
	{"community manager", "marketing"},
	{"комьюнити-менеджер", "marketing"},
	// Bare "manager" resolves last so a functional prefix wins ("Sales Manager"
	// → sales, "Operations Manager" → operations, "Finance Manager" → finance); a
	// manager title with no recognized function falls through to management.
	{"manager", "management"},
	{"менеджер", "management"},
	{"analyst", "data_analytics"},
	{"аналитик", "data_analytics"},
	// software_engineering: the generic catch-all for a title classify.IsTech's
	// techTitleTerms already confirms as software/IT work but that names no
	// sub-discipline — "Software Engineer" and "Java Developer" do not say
	// backend vs frontend vs fullstack, and this package never guesses. Every
	// entry below mirrors a techTitleTerms member that has no more specific
	// categoryTable entry above it (cross-checked against tech.go so the two
	// lists cannot silently drift). Kept at the very bottom, second-to-last
	// before the 1C fallback, so any more specific alias anywhere above always
	// wins first — these only fire once nothing else has.
	//
	// Deliberately EXCLUDED: bare "programmer" (techTitleTerms has it as an
	// "unambiguous" single word for is_tech, but prod titles include "CNC
	// Programmer" — a machining role, not software — so a category entry here
	// would mislabel it; is_tech's false positive on it is a separate, smaller
	// bug left alone).
	//
	// The base phrases ("software engineer", "software developer", "software
	// development engineer") deliberately carry every qualified variant with
	// them by substring, the same convention the "design engineer" bare form
	// uses above: "Senior Software Engineer, Platform" and "AI Software
	// Engineer, Internal Tools" resolve without their own entry.
	{"software engineer", "software_engineering"},
	{"software development engineer", "software_engineering"},
	{"software developer", "software_engineering"},
	{"web developer", "software_engineering"},
	{"web engineer", "software_engineering"},
	{"app developer", "software_engineering"},
	{"application developer", "software_engineering"},
	{"game developer", "software_engineering"},
	{"go engineer", "software_engineering"},
	{"golang engineer", "software_engineering"},
	{"go developer", "software_engineering"},
	{"golang developer", "software_engineering"},
	{"python developer", "software_engineering"},
	{"java developer", "software_engineering"},
	{"javascript developer", "software_engineering"},
	{"typescript developer", "software_engineering"},
	{".net developer", "software_engineering"},
	{"dotnet developer", "software_engineering"},
	{"php developer", "software_engineering"},
	{"ruby developer", "software_engineering"},
	{"rails developer", "software_engineering"},
	{"c# developer", "software_engineering"},
	{"c++ developer", "software_engineering"},
	{"node developer", "software_engineering"},
	{"nodejs developer", "software_engineering"},
	{"node.js developer", "software_engineering"},
	{"salesforce developer", "software_engineering"},
	{"sharepoint developer", "software_engineering"},
	{"database developer", "software_engineering"},
	{"rpa developer", "software_engineering"},
	{"erp developer", "software_engineering"},
	{"sap developer", "software_engineering"},
	{"oracle developer", "software_engineering"},
	{"abap developer", "software_engineering"},
	{"wordpress developer", "software_engineering"},
	{"drupal developer", "software_engineering"},
	{"magento developer", "software_engineering"},
	{"shopify developer", "software_engineering"},
	// "Member of Technical Staff" reads as software on the same evidence tech.go
	// cites (294/300 sampled prod postings software or AI). "Founding Engineer"
	// is the early-startup twin of the same generalist population.
	{"member of technical staff", "software_engineering"},
	{"member of the technical staff", "software_engineering"},
	{"founding engineer", "software_engineering"},
	// "AI-native"/"AI-enabled" describe the toolchain, not the discipline (same
	// reasoning as tech.go's techTitleTerms entry for these).
	{"ai-native engineer", "software_engineering"},
	{"ai native engineer", "software_engineering"},
	// software_engineering, non-English forms. Same doctrine as the English block
	// above: only software- or language-ANCHORED phrases, never a bare
	// developer/engineer/programmer noun. Prod titles show the bare noun is
	// genuinely dangerous in every language sampled, not just English — "SPS-
	// Programmierer"/"Roboterprogrammierer" (DE) and "Programador CNC" (ES) are
	// industrial/machine programming, not software, and Spanish/Portuguese
	// "desarrollador/desenvolvedor" alone also names a real-estate developer.
	// Feminine/inclusive forms are listed where prod data showed them in use
	// ("Engenheira de Software", "Pessoa Desenvolvedora"); an exhaustive sweep of
	// every language's gendered and inclusive spellings is future work, not done
	// here.
	//
	// Spanish.
	{"desarrollador de software", "software_engineering"},
	{"desarrolladora de software", "software_engineering"},
	{"ingeniero de software", "software_engineering"},
	{"ingeniera de software", "software_engineering"},
	{"desarrollador java", "software_engineering"},
	{"desarrolladora java", "software_engineering"},
	// Portuguese.
	{"desenvolvedor de software", "software_engineering"},
	{"desenvolvedora de software", "software_engineering"},
	{"engenheiro de software", "software_engineering"},
	{"engenheira de software", "software_engineering"},
	{"desenvolvedor java", "software_engineering"},
	{"desenvolvedora java", "software_engineering"},
	// French. No bare "développeur": it also names a real-estate/regional-economic
	// developer in French job titles, so only the language- and software-anchored
	// forms below resolve — a bare "Développeur" (or the inclusive "développeur.euse"
	// spelling) with no qualifier stays unresolved rather than guessed.
	{"ingénieur logiciel", "software_engineering"},
	{"développeur java", "software_engineering"},
	{"développeur salesforce", "software_engineering"},
	// German. Compound, hyphenated, and spaced forms are three different strings
	// to this matcher — a hyphen and a space both break a compound differently,
	// so each spelling needs its own alias (same trap "middle-east" documents).
	{"softwareentwickler", "software_engineering"},
	{"software-entwickler", "software_engineering"},
	{"software entwickler", "software_engineering"},
	{"entwickler software", "software_engineering"},
	{"softwareingenieur", "software_engineering"},
	{"java entwickler", "software_engineering"},
	{"java-entwickler", "software_engineering"},
	{"abap entwickler", "software_engineering"},
	// Polish.
	{"inżynier oprogramowania", "software_engineering"},
	{"deweloper oprogramowania", "software_engineering"},
	{"programista .net", "software_engineering"},
	{"programista python", "software_engineering"},
	// Italian.
	{"sviluppatore software", "software_engineering"},
	{"ingegnere del software", "software_engineering"},
	{"ingegnere software", "software_engineering"},
	// The IT tail: titles the catalogue carries in volume that this dictionary had no
	// word for at all. Measured on prod 2026-09-02, 47.5% of open postings reached the
	// search index with no role, and EVERY one of them had an empty category — so the
	// gap is here, not in roletag.
	//
	// The whole block is declared late on purpose. Nothing in it resolves to anything
	// today, so a late declaration cannot take a row from a category that already
	// works; the ordering that matters is only WITHIN the block.
	//
	// The four industrial namesakes come first and are BLIND. "Systems Engineer" is the
	// single largest unresolved IT title here (1440 open for the exact spelling), so the
	// bare alias has to exist — and without these four above it, every control, power,
	// electrical and quality engineer in the catalogue would be swept into software.
	// The sentinel keeps them resolving to nothing, which is what they do today; they
	// belong to an industrial taxonomy this change does not introduce.
	{"control systems engineer", categoryNone},
	{"power systems engineer", categoryNone},
	{"electrical systems engineer", categoryNone},
	{"quality systems engineer", categoryNone},
	// Then the qualified IT spellings, each naming its own discipline.
	{"linux systems engineer", "devops"},
	{"cyber systems engineer", "security"},
	{"software systems engineer", "software_engineering"},
	{"it systems engineer", "software_engineering"},
	// The bare form closes the family. `software_engineering` and not `devops`: the
	// population left after the four blind spellings is mixed between infrastructure
	// and generalist software work, and the generic bucket is the honest answer where
	// devops would be a guess.
	{"systems engineer", "software_engineering"},
	{"system engineer", "software_engineering"},

	// Vendor platforms. Naming an enterprise product states the discipline as surely
	// as naming a language does. "Salesforce Developer" and "SAP Developer" already
	// resolved through the bare "developer" alias; the administrator and consultant
	// spellings did not.
	{"servicenow developer", "software_engineering"},
	{"servicenow engineer", "software_engineering"},
	{"servicenow administrator", "devops"},
	{"salesforce administrator", "software_engineering"},
	{"salesforce engineer", "software_engineering"},
	{"salesforce consultant", "software_engineering"},
	{"mainframe developer", "software_engineering"},
	{"oracle dba", "devops"},
	{"sharepoint administrator", "devops"},
	{"tableau developer", "data_analytics"},

	// Infrastructure and end-user IT. "IT Specialist"/"IT Technician" go to `support`
	// rather than `devops`: "IT Support Specialist" already resolves there, and they
	// name the same desk — splitting one job across two facets on a dropped word is
	// the defect the design split existed to fix.
	{"data center technician", "devops"},
	{"data center engineer", "devops"},
	{"release engineer", "devops"},
	{"cloud operations engineer", "devops"},
	{"cloud migration engineer", "devops"},
	{"network operations engineer", "devops"},
	{"network specialist", "network_engineering"},
	{"network technician", "network_engineering"},
	{"it specialist", "support"},
	{"it technician", "support"},
	// The integration family. None of these contains "systems engineer" as consecutive
	// words, so their order against that alias does not matter.
	{"integration engineer", "software_engineering"},

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
	{"administrative assistant", "administration"},
	{"executive assistant", "administration"},
	{"office manager", "administration"},
	{"office assistant", "administration"},
	{"receptionist", "administration"},
	{"legal secretary", "administration"},
	{"medical secretary", "administration"},
	{"secretary", "administration"},
	{"data entry", "administration"},

	// PERSONAL AND FACILITY SERVICES.
	{"master stylist", "personal_services"},
	{"stylist", "personal_services"},
	{"barber", "personal_services"},
	{"esthetician", "personal_services"},
	{"aesthetician", "personal_services"},
	{"lifeguard", "personal_services"},
	// The guard entries live UP in the security block, where they have to sit above the
	// bare "security" alias to take their own titles; declaring them again here would
	// be dead for `Parse` and, worse, `Categories` would still see the duplicate.
	{"janitor", "personal_services"},
	{"custodian", "personal_services"},
	{"housekeeper", "personal_services"},
	{"housekeeping", "personal_services"},

	// Industrial engineering: the seats a factory, plant, utility or field-service
	// organisation staffs. 51 994 open postings measured on prod after the IT wave, and
	// the whole residue was this one shape — there was nowhere to file it, since
	// `engineering_design` means draughting and a Quality Engineer is not a draughtsman.
	//
	// The IT lookalikes come first: each names a discipline of its own that no alias
	// above would catch, and the bare "engineer" at the bottom of this block would
	// otherwise take them. `field application engineer` is the semiconductor pre-sales
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
	// Both were left unresolved by the IT wave for want of a home. Here they read
	// industrial: "Automation Engineer" sits beside "Controls Engineer" in this
	// catalogue, and a QA automation engineer's title already carries "QA", which
	// resolves far above.
	{"automation engineer", "industrial_engineering"},
	{"application engineer", "industrial_engineering"},
	{"applications engineer", "industrial_engineering"},
	// No "reliability engineer": the suite already pins it to no category because
	// mechanical reliability and site reliability share the phrase, and only "site"
	// tells them apart.
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
	{"safety engineer", "industrial_engineering"},
	{"environmental engineer", "industrial_engineering"},
	{"geotechnical engineer", "industrial_engineering"},
	{"planning engineer", "industrial_engineering"},
	{"resident engineer", "industrial_engineering"},

	// 1С (the RU enterprise/ERP dev platform) resolves last so a more specific role word in the
	// title wins first ("Аналитик 1С" → data_analytics, "Тестировщик 1С" → qa); a title whose only
	// signal is 1С ("Программист 1С", "1С-разработчик") reads as backend — server-side enterprise
	// development. Bare tokens so any separator ("1С-разработчик", "разработчик 1С") matches.
	{"1c", "backend"},
	{"1с", "backend"},

	// Russian software and administration, declared AFTER 1С for exactly the reason
	// stated above it: "Программист 1С" must stay backend, and a bare "программист"
	// declared any earlier would take it.
	//
	// Bare tokens, unlike the English entries. Russian puts the technology FIRST
	// ("Java-разработчик", "Python-разработчик", "Инженер-программист") and a hyphen is
	// a word boundary, so no qualified alias can stand in for the bare one — while the
	// bare one reaches every spelling. The same reasoning the 1С entry records.
	// The more specific role words (backend, аналитик, тестировщик) are all declared
	// far above, so they still win.
	{"сетевой администратор", "network_engineering"},
	{"системный администратор", "devops"},
	{"администратор баз данных", "devops"},
	{"программист", "software_engineering"},
	{"разработчик", "software_engineering"},

	// The Russian engineering family. Roughly half the industrial residue, and none of
	// it carried an English alias. The two qualified forms that name ANOTHER discipline
	// are declared first: "Инженер-проектировщик" is a draughtsman and
	// "Инженер по защите информации" an information-security engineer, and the bare
	// token below would otherwise claim both.
	//
	// Bare tokens for the same reason the software ones above are: the qualified forms
	// either hyphenate ("Инженер-технолог") or postfix a prepositional phrase
	// ("Инженер по подготовке производства"), and a hyphen is a word boundary — only
	// the bare token reaches every spelling.
	{"инженер-проектировщик", "engineering_design"},
	{"инженер по защите информации", "security"},
	{"инженер", "industrial_engineering"},
	{"технолог", "industrial_engineering"},

	// The Russian consumer vocabularies. `врач` is a bare token for the same reason
	// every Russian entry here is: the qualified forms hyphenate (`Врач-терапевт`,
	// `Врач-акушер-гинеколог`) or postfix a phrase (`Врач ультразвуковой
	// диагностики`), and a hyphen is a word boundary.
	//
	// The hazard runs the other way here — a short alias hiding INSIDE a longer word.
	// `Делопроизводитель` (an office clerk) ends in `водитель`, and `Электромеханик`
	// contains `механик`. wordmatch matches on boundaries and cannot make that
	// mistake, but nothing in this list shows the hazard, so both pairs carry a
	// regression test.
	{"ветеринарный врач", "healthcare"},
	{"врач", "healthcare"},
	{"медсестра", "healthcare"},
	{"медбрат", "healthcare"},
	{"фельдшер", "healthcare"},
	{"санитар", "healthcare"},
	{"электромеханик", "skilled_trades"},
	{"электросварщик", "skilled_trades"},
	{"электромонтёр", "skilled_trades"},
	{"электромонтер", "skilled_trades"},
	{"электрик", "skilled_trades"},
	{"сварщик", "skilled_trades"},
	{"слесарь", "skilled_trades"},
	{"плотник", "skilled_trades"},
	{"маляр", "skilled_trades"},
	{"механик", "skilled_trades"},
	// General building maintenance — 5 256 open across these two spellings, and the
	// same work the trades above already cover.
	{"рабочий по комплексному обслуживанию", "skilled_trades"},
	{"рабочий по благоустройству", "skilled_trades"},

	// The Russian service vocabularies.
	//
	// `делопроизводитель` is declared FIRST and on purpose: it is an office clerk, and
	// it ENDS in `водитель`. wordmatch matches on word boundaries so the bare driver
	// alias could not take it anyway — but the two now exist together in production,
	// in different categories, and a reader scanning this list would have no way to
	// see the hazard. The regression test asserts the clerk resolves to
	// `administration` specifically, not merely to something.
	{"делопроизводитель", "administration"},
	{"секретарь", "administration"},
	{"администратор", "administration"},
	{"офис-менеджер", "administration"},

	{"сборщик заказов", "logistics"},
	{"заведующий складом", "logistics"},
	{"кладовщик", "logistics"},
	{"экспедитор", "logistics"},
	{"грузчик", "logistics"},
	{"курьер", "logistics"},
	{"водитель", "logistics"},

	// Russian inflects, and `wordmatch` has no more morphology for it than it has for
	// English plurals: "Помощник воспитателя" (2 097 open across two spellings) is the
	// genitive and the nominative alias cannot reach it. The same trap as the
	// automotive plurals fixed in this change, in another language.
	{"воспитателя", "education"},
	{"воспитатель", "education"},
	{"преподаватель", "education"},
	{"педагог", "education"},
	{"методист", "education"},

	{"парикмахер", "personal_services"},
	{"охранник", "personal_services"},
	{"уборщик", "personal_services"},
	{"уборщица", "personal_services"},
	{"сиделка", "personal_services"},

	// NO bare English "engineer", though 689 open postings spell it exactly. It was
	// tried and the existing suite rejected it, which is the answer: "Product
	// Engineer", "Growth Engineer", "Staff Engineer" and "Developer Onboarding
	// Engineer" are all pinned to NO category on purpose, because the word before
	// "engineer" is what decides and those words are ambiguous. A bare alias overrides
	// every one of those decisions at once.
	//
	// It also breaks `Categories()`, which returns EVERY matching alias rather than the
	// strongest: a bare "engineer" appends this category to "Senior Backend Engineer"
	// and to every other engineering title in the catalogue, polluting the multi-
	// category CV path that reads it.
	//
	// The Russian bare token above does not have the second problem to the same degree
	// — "инженер" heads far fewer resolved titles here — and its qualified forms are
	// declared above it, so `Parse` stays correct.

	// Media production, declared LAST on purpose. Every craft here is also a tool or a
	// second hat named inside someone else's title — "Marketing Specialist (Photoshop,
	// Illustrator)", "Graphic Designer & Photographer", "Junior Motion Designer /
	// Animator". This table resolves in declaration order, so declaring these anywhere
	// above `design` or `marketing` does not merely add a category: it TAKES those
	// rows, which is the one thing this change promised not to do. Declared last, a
	// title resolves to the craft only when it names no other discipline at all.
	//
	// The cost is stated rather than hidden: a "Social Media Video Editor" resolves to
	// `marketing`, not to the craft. That is the right side to err on — the posting is
	// still findable, on a facet that was already correct for it.
	//
	// The bare craft words ("video", "audio", "art", "sound", "photo") are not aliases
	// in either block: each occurs in titles across every discipline ("Audio DSP
	// Engineer", "Art Director", "State of the Art").
	{"video editor", "creative"},
	{"video producer", "creative"},
	{"videographer", "creative"},
	{"photographer", "creative"},
	{"photo editor", "creative"},
	{"animator", "creative"},
	// "motion graphics artist" is the artist spelling of the craft; the DESIGNER
	// spelling ("Motion Graphics Designer") stays in `design`, where its named role
	// already lives — and would win here regardless, being declared above.
	{"motion graphics artist", "creative"},
	{"concept artist", "creative"},
	{"character artist", "creative"},
	{"environment artist", "creative"},
	{"technical artist", "creative"},
	{"storyboard artist", "creative"},
	{"vfx artist", "creative"},
	{"3d artist", "creative"},
	{"2d artist", "creative"},
	// Also the Adobe product, which is why it is here rather than beside the crafts it
	// belongs with: a design or marketing title that names the tool keeps its own row.
	{"illustrator", "creative"},
}

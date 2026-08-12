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
	{"frontend", "frontend"},
	{"front-end", "frontend"},
	{"front end", "frontend"},
	{"фронтенд", "frontend"},
	{"фронт", "frontend"},
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
	// DevSecOps stays security, not devops: the security responsibility (SAST/DAST,
	// container/IaC scanning, policy-as-code) is why the title exists, not incidental.
	{"devsecops", "security"},
	{"embedded", "embedded"},
	{"firmware", "embedded"},
	{"встраиваемые", "embedded"},
	{"встраиваемых", "embedded"},
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
	{"технический писатель", "technical_writing"},
	{"техписатель", "technical_writing"},
	{"технический редактор", "technical_writing"},
	{"разработчик документации", "technical_writing"},
	{"специалист по документации", "technical_writing"},
	{"ux-редактор", "technical_writing"},
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
	{"sound design engineer", "design"},
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
	// 1С (the RU enterprise/ERP dev platform) resolves last so a more specific role word in the
	// title wins first ("Аналитик 1С" → data_analytics, "Тестировщик 1С" → qa); a title whose only
	// signal is 1С ("Программист 1С", "1С-разработчик") reads as backend — server-side enterprise
	// development. Bare tokens so any separator ("1С-разработчик", "разработчик 1С") matches.
	{"1c", "backend"},
	{"1с", "backend"},
}

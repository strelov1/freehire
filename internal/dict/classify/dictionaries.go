package classify

import (
	"regexp"
	"slices"
)

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
	// Corporate titles a rung below Vice President — an AVP leads a desk at a bank or an
	// account at a services firm — not executives. They name no grade of their own.
	"assistant vice president",
	"associate vice president",
	// "Second new graduate" — someone a year or three out of university — is written as a
	// welcome ("第二新卒歓迎") on postings of every grade, so it states none. Cut, it also stops
	// the 新卒 inside it from reading as a new-graduate hire.
	"第二新卒",
}

// earlyCareersDeskPatterns are the grade-blind phrases of the early-careers vocabulary,
// stated as a SHAPE rather than a list: the staff who run graduate programmes and
// placements — university offices, graduate recruiting, placement officers — carry
// "graduate"/"placement" right next to the desk they name, and the catalogue (broader
// than IT) holds real postings for all of them: "Assistant Director of Graduate
// Admissions", "Graduate Recruiting Manager", "Graduate Medical Education Coordinator",
// "Work Placement Officer", "Placement Year Manager". Masked, they state no grade,
// exactly as the "graduate"/"placement" aliases would never have matched them.
//
// The first shape is the word FOLLOWED by the desk (optionally through "program(me)"):
// "Graduate Program Manager", "Industrial Placement Coordinator". The second is a desk
// in charge OF "graduate programs": "Director of Graduate Programs", "Manager,
// Graduate Programs". A hire's own title never has that shape — "Graduate Project
// Manager" puts its craft between the two, and "Software Engineering Graduate
// Programme" names no desk — and a real grade elsewhere in the title survives the cut
// ("Senior Graduate Recruiter" is senior).
var earlyCareersDeskPatterns = []*regexp.Regexp{
	regexp.MustCompile(`\b(?:graduate|(?:(?:industrial|industry|work|summer|student|sandwich) )?placement(?: year| student)?)(?: program(?:me)?s?)? (?:co-?ordinator|manager|director|officer|advis[eo]r|administrator|recruiter|recruiting|recruitment|admissions|school|studies|office|medical education|student cent(?:er|re))\b`),
	regexp.MustCompile(`\b(?:director|manager|co-?ordinator|officer|head|dean|advis[eo]r|administrator)(?: of| for|,)? graduate program(?:me)?s\b`),
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
	// A trainee HIRED as one is an employee on an entry grade, not an intern: India's
	// Graduate Engineer Trainee and the Management Trainee are full-time campus hires, as
	// a UK graduate trainee is. Listed before the bare word, which keeps its reading.
	{"graduate engineer trainee", "junior"},
	{"graduate trainee", "junior"},
	{"management trainee", "junior"},
	{"engineer trainee", "junior"},
	{"trainee engineer", "junior"},
	{"trainee", "intern"},
	{"стажёр", "intern"},
	{"стажер", "intern"},
	{"стажировка", "intern"},
	// UK early-careers vocabulary (gradcracker, and the UK employers' own boards). A
	// placement there is a student's paid year or summer inside the employer, which is
	// what an internship is. Every alias is a PHRASE: a bare "placement" names the
	// recruiter's job ("Placement Coordinator") and the marketing one ("Product
	// Placement"), and a bare "undergraduate" names most of a university's admissions
	// and faculty titles — measured on prod, the majority of the unresolved titles
	// carrying it were "The Undergraduate School - Adjunct Faculty" and its siblings.
	{"industrial placement", "intern"},
	{"industry placement", "intern"},
	{"placement year", "intern"},
	{"year placement", "intern"},
	{"year in industry", "intern"},
	{"placement student", "intern"},
	{"student placement", "intern"},
	{"summer placement", "intern"},
	{"undergraduate placement", "intern"},
	{"work placement", "intern"},
	// "12 Month Placement" / "12-month placement": the duration is what makes it a
	// student's year out rather than a recruiter placing a candidate.
	{"month placement", "intern"},
	{"vacation scheme", "intern"},
	{"summer analyst", "intern"},
	// "Graduate" is the entry-level hire (a graduate scheme, "Graduate Software
	// Engineer", a hospital's "New Graduate RN") — the same reading the description
	// matcher gives "graduate position". It is LAST, below every intern alias, because
	// it is the weakest evidence in the table: "Graduate Summer Internship" is an
	// internship, and any explicit grade a title also states outranks it. The
	// staff desks that merely contain the word are masked by
	// earlyCareersDeskPatterns.
	{"graduate", "junior"},
}

// categoryTable lists category aliases in precedence order — multi-word and more
// specific terms first, so "data analyst" wins over a bare "data" and "fullstack"
// is not shadowed by "backend"/"frontend" — each paired with its
// vocab.CategoryValues canonical.
//
// The Japanese block (japanese.go) leads it: a Japanese title routinely carries Latin
// fragments ("AWS", "SE", "PM") that the aliases below would otherwise claim first.
var categoryTable = slices.Concat(japaneseCategoryTable, coreCategoryTable, japaneseTrailCategoryTable)

var coreCategoryTable = []aliasEntry{
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
	{"mobile engineer", "mobile"},
	{"mobile architect", "mobile"},
	{"mobile app", "mobile"},
	{"mobile apps", "mobile"},
	{"mobile application", "mobile"},
	{"mobile applications", "mobile"},
	{"mobile software", "mobile"},
	{"mobile platform", "mobile"},
	{"mobile automation", "mobile"},
	{"mobile qa", "mobile"},
	{"android", "mobile"},
	{"ios", "mobile"},
	// React Native is mobile-only, unlike bare "react" above. Xamarin is too, and it
	// needs its own alias because the titles that carry it write "Mobile/Xamarin
	// Developer" — a slash where the qualified phrases above want a space.
	{"react native developer", "mobile"},
	{"xamarin", "mobile"},
	// Hungarian: "mobilalkalmazás" is the mobile app. Both spellings are listed
	// because a hyphen is a word boundary here, so neither form contains the other.
	{"mobilalkalmazás-fejlesztő", "mobile"},
	{"mobilalkalmazás fejlesztő", "mobile"},
	// Russian qualifies the same way: the app, the craft, or the developer — never the
	// bare adjective, which is what "Мобильный банкир" is built from.
	{"мобильный разработчик", "mobile"},
	{"мобильных приложений", "mobile"},
	{"мобильное приложение", "mobile"},
	{"мобильная разработка", "mobile"},
	{"мобильной разработки", "mobile"},
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
	// 524 live postings. Reached only through the bare "analyst" fall-through before it
	// was removed, which called them data analysts.
	{"test analyst", "qa"},
	// Hungarian, qualified only. The bare noun was admitted here for one day on the
	// footing English's bare "tester" has, and a general-population board took it back:
	// the first sample came from the platform's two IT categories, where every
	// "tesztelő" is a testing seat, and the other 21 hold "Szivattyú tesztelő" —
	// whoever tests pumps. English keeps its bare entry because the boards it is read
	// on are not general-population ones; this language is read on one that is.
	//
	// The adjective "tesztelési" is out for the same reason it always was: the sample
	// spends it on calibration and manufacturing process work ("Kalibrálási- és
	// tesztelési folyamatfejlesztő mérnök").
	{"szoftvertesztelő", "qa"},
	{"manuális tesztelő", "qa"},
	{"automata tesztelő", "qa"},
	{"tesztmenedzser", "qa"},
	// Physical security is not information security: a guard is the protective-service
	// craft (SOC 33-9032), filed under `public_safety` with police and firefighters, and
	// an infosec filter must not return him. "Security Officer" — the commonest guard
	// title — is declared at the END of this block, behind the infosec officer spellings
	// that contain it; see the note there.
	{"security guard", "public_safety"},
	{"security guards", "public_safety"},
	{"armed guard", "public_safety"},
	{"unarmed guard", "public_safety"},
	// Bare "security" is NOT an entry, for the reason the Hungarian block below already
	// gives about "biztonsági": the word alone is the guard at least as often as the
	// discipline. Measured over the live catalogue, the single commonest title carrying
	// it is "Security Officer" (1 102 open postings), followed by "Security Specialist"
	// (602) and "Night shift security front desk - receptionist" (483) — none of them
	// technical, and "Security Supervisor" (149) and bare "Security" (91) behind them.
	// The bare alias sent every one of those to the security category, and because
	// `security` is in vocab.TechCategories that category is enough for
	// jobderive to set is_tech TRUE on its own, ahead of the non-tech
	// dictionary. So a mall guard was filed as an IT security role — in search, and
	// (since 2026-09-14) in what cmd/search-ping spends Google's Indexing API quota on.
	//
	// Only the qualified forms below, each in the spelling the live sample carries.
	// "information security" covers the officer/analyst/engineer/manager/specialist
	// family in one entry, which is how "Chief Information Security Officer" keeps its
	// category while "Security Officer" loses it. Deliberately ABSENT because they are
	// genuinely ambiguous rather than merely rare: "security specialist", "security
	// manager", "security supervisor", "security consultant" — these resolve to no
	// category and their is_tech falls to unknown, which is the never-guess contract
	// every dictionary here follows. Same call the bare "analyst" fall-through got.
	{"security engineer", "security"},
	{"security architect", "security"},
	{"security analyst", "security"},
	{"security operations", "security"},
	{"security researcher", "security"},
	{"information security", "security"},
	{"it security", "security"},
	{"application security", "security"},
	{"network security", "security"},
	{"cloud security", "security"},
	{"infosec", "security"},
	{"appsec", "security"},
	{"cybersecurity", "security"},
	{"cyber security", "security"},
	{"безопасность", "security"},
	{"безопасности", "security"},
	{"кибербезопасность", "security"},
	// Hungarian. The bare adjective "biztonsági" is the guard and the fire warden as
	// often as the discipline — the live sample holds "Biztonsági operátor és
	// fegyveres biztonsági őr" and "Tűz- és munkabiztonsági szakértő" — so it is not
	// an entry and only the qualified forms are. Each is listed in the spellings prod
	// actually carries: the closed compound, the spaced pair, and the hyphenated form,
	// which are three different strings to a word-boundary match.
	{"kiberbiztonsági", "security"},
	{"információbiztonsági", "security"},
	{"információ biztonsági", "security"},
	{"it biztonsági", "security"},
	{"it-biztonsági", "security"},
	{"informatikai biztonsági", "security"},
	// Narrower technical niches within security. Deliberately no bare "compliance" —
	// sampled live titles are dominated by non-IT banking/legal/customs compliance
	// (that population already routes to `legal` via "compliance officer/manager/
	// analyst" above); a bare entry here would be a GTM-style word-trap.
	{"iam", "security"},
	{"identity and access management", "security"},
	{"grc", "security"},
	{"vulnerability management", "security"},
	{"vulnerability analyst", "security"},
	// The analyst forms the bare fall-through used to claim: 263 and 80 live postings.
	{"threat analyst", "security"},
	{"cyber analyst", "security"},
	// The security operations centre. Without this the title falls through to the
	// bare "analyst" alias far below and lands in data_analytics.
	{"soc analyst", "security"},
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
	// "Security Officer" is a guard about nine times in ten — measured on a 2% sample of
	// the open catalogue, 175 of 192 titles carrying it named no information-security
	// word ("Unarmed Security Officer", "Mall Security Officers", "Security Officer -
	// Marriott ..."). The rest are infosec officers, and every spelling of those that
	// CONTAINS the phrase is declared first, as a whole phrase ending in "officer".
	//
	// Whole phrases on purpose: `Categories()` returns every matching alias, and an
	// occurrence of a shorter alias lying inside the span a longer, earlier alias already
	// matched is not counted (see matchAllOrdered). "information security" alone would
	// not cover the word "officer", so "Chief Information Security Officer" would come
	// back tagged with the guard category on the multi-category CV path.
	//
	// The defence-industry "Program/Facility Security Officer" administers clearances and
	// classified material: neither a guard nor an infosec engineer, so it names nothing.
	{"information security officer", "security"},
	{"information systems security officer", "security"},
	{"information system security officer", "security"},
	{"it security officer", "security"},
	{"ict security officer", "security"},
	{"cyber security officer", "security"},
	{"data security officer", "security"},
	{"network security officer", "security"},
	{"cloud security officer", "security"},
	{"application security officer", "security"},
	{"program security officer", categoryNone},
	{"facility security officer", categoryNone},
	{"security officer", "public_safety"},
	{"security officers", "public_safety"},
	{"embedded", "embedded"},
	{"firmware", "embedded"},
	// Hungarian: "beágyazott" is embedded. It precedes the plain
	// "szoftverfejlesztő" entry further down, which would otherwise claim it.
	{"beágyazott szoftverfejlesztő", "embedded"},
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
	// Chip-design titles that also carry "architect": declared above the bare
	// "architect" alias, which would otherwise claim them for software architecture.
	// "ASIC Digital Design, <grade>" alone is ~100 open postings from one employer.
	{"asic digital design", "hardware"},
	{"ic digital design", "hardware"},
	{"digital design architect", "hardware"},
	{"digital design flow", "hardware"},
	{"digital design capability architect", "hardware"},
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
	// Disability and residential care is social service, not a help desk. "Direct
	// Support Professional" is the US title for the worker who supports a person with an
	// intellectual or developmental disability at home or in a group home, and "Support
	// Worker" (with "Personal/Family/Home/Bank Support Worker") is the UK and Canadian
	// title for the same care work. The bare "support" below filed both as customer
	// support. The IT and customer-service spellings of "support worker" are pinned
	// first, so the desk keeps them.
	//
	// The abbreviation "DSP" is deliberately never an alias: in this catalogue it is as
	// often Amazon's Delivery Service Partner ("Amazon DSP Delivery Driver"), a
	// demand-side platform, a downstream-process operator or digital signal processing.
	{"administrative support worker", "administration"},
	{"it support worker", "support"},
	{"customer support worker", "support"},
	{"technical support worker", "support"},
	{"direct support", "social_services"},
	{"support worker", "social_services"},
	{"support workers", "social_services"},
	{"support", "support"},
	{"customer service", "support"},
	{"help desk", "support"},
	// The unspaced compound: "help desk" above cannot reach it, the same trap the
	// "back-end"/"back end"/"backend" trio guards against.
	{"helpdesk", "support"},
	// The ITIL name for the same desk. Without it the far more common "Service Desk
	// Analyst" fell through to the bare "analyst" alias and read as data_analytics.
	{"service desk", "support"},
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
	// IT-company back-office roles. All are multi-word/anchored, so a functional title
	// ("Financial Analyst", "Operations Manager") names its function outright. hr
	// precedes operations so "People Operations Manager" → hr, not operations.
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
	// "brand manager" inside it), so it needs its own entry.
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
	// Immigration practice: must precede the administration block ("Immigration
	// Assistant") and the case-manager spellings so neither steals this family. "immigration paralegal" is
	// listed for completeness — the bare "paralegal" entry above already
	// resolves it to `legal` — but every other entry here is load-bearing.
	{"immigration paralegal", "legal"},
	{"immigration specialist", "legal"},
	{"immigration assistant", "legal"},
	{"immigration consultant", "legal"},
	{"immigration case manager", "legal"},
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
	// The assistant family is `administration` (see that block below), but these
	// spellings must resolve HERE, at the precedence they always had: "Executive
	// Assistant to the General Manager" names a manager after the role, and the
	// manager aliases declared between this line and the administration block would
	// take it.
	{"executive assistant", "administration"},
	{"administrative assistant", "administration"},
	{"personal assistant", "administration"},
	{"ассистент руководителя", "administration"},
	{"помощник руководителя", "administration"},
	{"procurement", "operations"},
	{"procurement manager", "operations"},
	{"purchasing manager", "operations"},
	{"facilities manager", "operations"},
	{"facilities coordinator", "operations"},
	{"операционный директор", "operations"},
	{"операционный менеджер", "operations"},
	{"офис-менеджер", "operations"},
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
	// not a brand's social presence. Sits below the DevRel block for that reason.
	{"community manager", "marketing"},
	{"комьюнити-менеджер", "marketing"},
	// Health, safety and environment. The block sat above the bare "manager"
	// fall-through (since retired), because that one line is what the profession was
	// losing to: measured on prod 2026-09-24, 1,454 live postings carrying an HSE acronym
	// resolved `management` on the strength of the word "Manager" alone, filing an EHS
	// Manager beside a Sales Manager. Another 4,515 resolved nothing at all.
	//
	// SOC codes EHS Managers separately from its 19-5011 specialists. This table
	// deliberately does not: `seniority` is its own facet here, so splitting one
	// profession across two category values would make a subscriber tick two boxes to
	// see one job market.
	//
	// First, the qualifiers that put the word "safety" in a title naming a DIFFERENT
	// profession. Without them the qualified "safety <role>" aliases below claim roughly
	// 2,300 live postings that are not this craft — food 616, public 843, patient 481,
	// product 163, campus 157 on 2026-09-24 — plus the tech-native families further down.
	// "fire safety" is deliberately absent: it is a genuine part of the HSE remit on an
	// industrial site, and 164 postings is too few to be worth splitting the
	// building-warden reading out of.
	//
	// Each ROUTES to the category that is true rather than carrying the blind sentinel,
	// because the rule at the top of this file says so and a review found the first
	// draft breaking it: a blanket `patient safety` sentinel blanked "Patient Safety
	// Registered Nurse" from `healthcare`, and `product safety` blanked "Product Safety
	// Engineer" from `industrial_engineering` — making a facet unreachable, which is the
	// harm this whole change exists to undo. The sentinel is left only where no category
	// is true.
	{"patient safety", "healthcare"},
	{"drug safety", "healthcare"}, // pharmacovigilance, the sibling of patient safety
	{"product safety", "industrial_engineering"},
	{"food safety", "industrial_engineering"},       // plant quality, where `quality engineer` already sits
	{"functional safety", "industrial_engineering"}, // ISO 26262, automotive and semiconductor
	{"life safety", "skilled_trades"},               // fire alarm and sprinkler trades
	{"ai safety", "ml_ai"},                          // alignment research, not a safety department
	// Protective services (SOC 33). They carried the blind sentinel until `public_safety`
	// existed, and now route to it. They stay paired with the role noun rather than bare,
	// because the bare qualifier would claim titles that answer for themselves: "Public
	// Safety Dispatcher" is filed by the emergency-dispatch entries in the public_safety
	// block, and "Public Safety Communications" the same way.
	//
	// The pairs are measured, not enumerated from imagination — every combination below
	// exists in the live catalogue, and the ones that do not are deliberately absent. A
	// first pass listed only the `officer` form, which left 27 postings of these same
	// professions filed under HSE.
	{"public safety officer", "public_safety"},     // 383
	{"campus safety officer", "public_safety"},     // 83
	{"school safety officer", "public_safety"},     // 8
	{"public safety manager", "public_safety"},     // 5
	{"public safety specialist", "public_safety"},  // 4
	{"public safety coordinator", "public_safety"}, // 3
	{"public safety director", "public_safety"},    // 3
	{"public safety technician", "public_safety"},  // 1
	{"public safety trainer", "public_safety"},
	{"public safety representative", "public_safety"},
	{"campus safety specialist", "public_safety"}, // 5
	{"campus safety supervisor", "public_safety"},
	{"campus safety coordinator", "public_safety"},
	{"campus safety manager", "public_safety"},
	{"campus safety representative", "public_safety"},
	{"campus safety intern", "public_safety"},
	{"pool safety officer", "public_safety"},
	// Trust & Safety is platform integrity at a consumer-tech employer — the most likely
	// "safety" collision on an IT job board.
	//
	// It is paired with the role noun for a sharper reason than the protective services
	// above. A bare `trust & safety` sentinel does not merely fail to improve things, it
	// BLANKS correctly-resolved technology roles: "Trust & Safety Product Manager" (7
	// live) resolves `product`, "Software Engineer, Trust & Safety" (4) resolves
	// `software_engineering`, and neither ever collided with this block, because the
	// matcher takes contiguous phrases and "safety product manager" does not contain
	// "safety manager". The first pass shipped that sentinel and blanked both.
	//
	// What the phrase names is a DOMAIN, not a craft — the live titles span product
	// managers, software engineers, analytics engineers, compliance, operations,
	// associates and directors — which is exactly why no single category is true for it
	// and why only the colliding pairs are listed. None of them names a category here.
	// Manager and director resolved `management` until that category was retired; a
	// director still reaches search through the `role_type` facet (internal/dict/roletype).
	{"trust and safety manager", categoryNone},
	{"trust & safety manager", categoryNone}, // 2 + 2 across the two spellings
	{"trust and safety director", categoryNone},
	{"trust & safety director", categoryNone}, // 2
	{"trust and safety specialist", categoryNone},
	{"trust & safety specialist", categoryNone}, // 14, the most common of these
	{"trust and safety lead", categoryNone},
	{"trust & safety lead", categoryNone}, // 4
	{"trust and safety engineer", categoryNone},
	{"trust & safety engineer", categoryNone}, // 3 + 1
	{"trust and safety coordinator", categoryNone},
	{"trust & safety coordinator", categoryNone},
	{"trust and safety representative", categoryNone},
	{"trust & safety representative", categoryNone},
	{"trust and safety intern", categoryNone},
	{"trust & safety intern", categoryNone},
	// Now the profession itself. The acronyms resolve bare: each is a coined initialism
	// with no English-word collision, unlike the two words below them.
	{"hse", "occupational_safety"},
	{"ehs", "occupational_safety"},
	{"hsse", "occupational_safety"},
	{"qhse", "occupational_safety"},
	{"hseq", "occupational_safety"},
	{"sheq", "occupational_safety"},
	{"shes", "occupational_safety"},
	// "SHE" is NOT here bare, and that is the one exclusion in this block with a
	// live-title reason rather than a dictionary one: it is an ordinary English word and
	// a pronoun that postings carry ("Software Engineer (she/her)"). Only the qualified
	// spellings, the same treatment bare "security" and bare "mobile" already get.
	{"she manager", "occupational_safety"},
	{"she officer", "occupational_safety"},
	{"she advisor", "occupational_safety"},
	{"she coordinator", "occupational_safety"},
	{"she specialist", "occupational_safety"},
	// The spelled-out forms. "environmental health and safety" and "occupational health
	// and safety" need no entry of their own — both contain "health and safety", and the
	// matcher is word-boundary based. The ampersand spelling does need one: it is a
	// different string, not a different boundary.
	{"health and safety", "occupational_safety"},
	{"health & safety", "occupational_safety"},
	// Russian. `internal/dict/classify`'s non-tech term list already carries "охрана
	// труда"/"охране труда", which is why these titles were being turned away at ingest
	// and hard-deleted rather than merely going uncategorised — see ConfirmedNonTech.
	// All three cases, because Russian titles inflect the phrase and the matcher does
	// not: "Специалист в области охраны труда" (112 live) is genitive, "Инженер по
	// охране труда" is dative, "Охрана труда" is nominative. The corpus probe found the
	// genitive; the hand-written list had only the other two.
	//
	// Note how narrow these are. The same corpus carries hundreds of postings for bare
	// "охрана" — "младший инспектор отдела охраны", "государственный инспектор по охране
	// леса" — which are security guards and forest rangers, not this profession. Only
	// the two-word phrase is admitted.
	{"охрана труда", "occupational_safety"},
	{"охране труда", "occupational_safety"},
	{"охраны труда", "occupational_safety"},
	// The titles SOC lists as reported for 19-5011, in the spellings prod carries, with
	// live counts. Bare "safety" is deliberately NOT an alias: it names patient safety
	// in healthcare, campus and public safety in protective services and food safety in
	// manufacturing quality, and these qualified forms cover the population without it.
	{"safety coordinator", "occupational_safety"}, // 283
	{"safety specialist", "occupational_safety"},  // 278
	{"safety officer", "occupational_safety"},     // 130
	{"safety technician", "occupational_safety"},  // 67
	{"safety supervisor", "occupational_safety"},  // 55
	{"safety advisor", "occupational_safety"},     // 42
	{"safety director", "occupational_safety"},    // 34
	{"safety manager", "occupational_safety"},     // 1,649 once the qualifiers above are excluded
	{"safety lead", "occupational_safety"},
	// Added after the corpus probe, which is the only thing that could have found them:
	// a test written from the list above can only confirm the list. Counts are live.
	{"safety administrator", "occupational_safety"}, // 24
	{"safety professional", "occupational_safety"},  // 23
	{"director of safety", "occupational_safety"},   // 20; the inverted form of "safety director"
	{"head of safety", "occupational_safety"},
	{"safety trainer", "occupational_safety"},        // 20
	{"safety intern", "occupational_safety"},         // 18
	{"safety representative", "occupational_safety"}, // 18
	{"safety inspector", "occupational_safety"},      // 14
	// The Singapore and site-officer spellings, which name the whole discipline between
	// the two words the qualified aliases above expect to be adjacent.
	{"workplace safety and health", "occupational_safety"}, // 25 across its spellings
	{"workplace safety & health", "occupational_safety"},
	{"safety and health officer", "occupational_safety"}, // 10, "Site Safety and Health Officer"
	{"safety & health officer", "occupational_safety"},
	{"industrial hygienist", "occupational_safety"}, // named in SOC's reported-title list
	{"industrial hygiene", "occupational_safety"},
	{"risk control consultant", "occupational_safety"},
	// Bare "risk" is NOT an alias. It matched 41,468 live postings and most of them are
	// finance — "Risk Analyst", "Credit Risk Manager" — which is a different profession
	// that happens to share a word, and which already resolves correctly elsewhere.
	{"hse risk", "occupational_safety"},
	{"safety risk", "occupational_safety"},
	// There is deliberately NO bare "manager"/"менеджер" and NO bare "analyst"/"аналитик"
	// here. Both used to be fall-throughs for a title with no recognised function, and
	// both filed it on the wrong axis.
	//
	// "manager" fell through to a `management` category, which held about 6% of the open
	// catalogue and told a jobseeker nothing about the work: an Assistant Manager at a
	// burger chain, a Case Manager, a Tax Manager and a Plant Manager in one facet. The
	// category is retired. A manager title now resolves the CRAFT it names — the
	// "restaurant/store/plant/tax/... manager" spellings are filed in the managers block
	// at the end of this table — and whether it manages PEOPLE is the `role_type` facet's
	// question (internal/dict/roletype), which reaches the generic General Manager and
	// Assistant Manager titles across every craft. Search keeps a posting either facet
	// reaches (search.CategoryUnresolved).
	//
	// "analyst" fell through to `data_analytics`, which is technical: an unrecognised
	// analyst was thereby DECLARED a technical data role.
	//
	// Measured on the open catalogue on 2026-09-06, that facet held 102,222 postings and
	// 77,086 of them (75%) carried no data, analytics or BI word at all. 13,748 were
	// Board Certified Behavior Analysts — school ABA therapists — and the rest
	// investment, purchasing, risk, pricing, research, policy and tax analysts. All
	// 102,222 had is_tech = true, so all of them were eligible for the enrichment budget,
	// and that technical category shielded every one from the non-technical dictionary.
	//
	// The technical families it was hiding are named individually instead, each in its
	// own block above and each resolving MORE precisely than this ever did: test,
	// threat/cyber, network, technical, application(s), it, technology, software,
	// infrastructure and cloud analysts, ~3.4k live postings between them. An analyst
	// the dictionary cannot place now resolves to nothing, which is what the rest of
	// this package does with a title it cannot read.
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
	// MES is the manufacturing execution system; the role writes and integrates that
	// software, so it is software engineering with a plant for a customer.
	{"mes developer", "software_engineering"},
	{"oracle developer", "software_engineering"},
	{"abap developer", "software_engineering"},
	{"wordpress developer", "software_engineering"},
	{"drupal developer", "software_engineering"},
	{"magento developer", "software_engineering"},
	{"shopify developer", "software_engineering"},
	// The `engineer` spelling of everything above, and the `developer` spelling of the
	// software titles that only had an `engineer` one. Which noun an employer reaches for
	// is a habit — "Python Developer" and "Python Engineer" are the same job — and the
	// list carried only one side of 38 of them.
	//
	// The cost of the omission is not a missing facet. A title this dictionary cannot
	// read gets no category AND no is_tech; EnqueuePendingJobs gates enrichment on
	// `is_tech IS TRUE`, so the LLM never sees the posting and never supplies the
	// category the dictionary missed; and search.CategoryUnresolved then hides it
	// FOREVER rather than until the next enrichment cycle. Measured on prod 2026-09-16:
	// "senior java engineer" alone was 233 open postings inside that loop.
	//
	// TestBothSpellingsOfACraftResolveTheSame derives the pairs from this table, so a
	// technology added under one noun from now on fails the build rather than quietly
	// losing its postings.
	{"python engineer", "software_engineering"},
	{"java engineer", "software_engineering"},
	{"javascript engineer", "software_engineering"},
	{"typescript engineer", "software_engineering"},
	{".net engineer", "software_engineering"},
	{"dotnet engineer", "software_engineering"},
	{"php engineer", "software_engineering"},
	{"ruby engineer", "software_engineering"},
	{"rails engineer", "software_engineering"},
	{"c# engineer", "software_engineering"},
	{"c++ engineer", "software_engineering"},
	{"node engineer", "software_engineering"},
	{"nodejs engineer", "software_engineering"},
	{"node.js engineer", "software_engineering"},
	{"abap engineer", "software_engineering"},
	{"app engineer", "software_engineering"},
	{"game engineer", "software_engineering"},
	{"mainframe engineer", "software_engineering"},
	{"sharepoint engineer", "software_engineering"},
	{"rpa engineer", "software_engineering"},
	{"erp engineer", "software_engineering"},
	{"sap engineer", "software_engineering"},
	{"mes engineer", "software_engineering"},
	{"oracle engineer", "software_engineering"},
	{"wordpress engineer", "software_engineering"},
	{"drupal engineer", "software_engineering"},
	{"magento engineer", "software_engineering"},
	{"shopify engineer", "software_engineering"},
	{"integration developer", "software_engineering"},
	{"it developer", "software_engineering"},
	{"founding developer", "software_engineering"},
	{"ai-native developer", "software_engineering"},
	{"ai native developer", "software_engineering"},
	// "Software Engineering <anything>" was the same omission in one letter: matching is
	// whole-word, so `software engineer` never occurs inside "Software Engineering
	// Intern" and that title resolved to nothing while "Software Engineer Intern"
	// resolved fine — 192 open postings apart on a gerund. It sits AFTER
	// `engineering manager` far above, which resolves "Software Engineering Manager"
	// first — to this same category.
	{"software engineering", "software_engineering"},
	// The `developer` spelling of the two AI entries below, which had only `engineer`.
	// "AI Developer" alone was 152 open postings resolving to nothing.
	{"ai developer", "ai_engineering"},
	{"ml developer", "ml_ai"},
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
	// Hungarian. The bare noun is "fejlesztő", and on the open catalogue it names
	// materials, supplier, process, product, business, operations and training
	// development ("Anyagfejlesztő", "Beszállítófejlesztő mérnök",
	// "Folyamatfejlesztő", "Termékfejlesztő szakértő", "Üzletfejlesztési menedzser",
	// "Működésfejlesztési szakértő", "Képzés-fejlesztési gyakornok") alongside the
	// software roles. Hungarian also writes closed compounds where English writes two
	// words, so a compound never contains its spaced twin on a word boundary. Where
	// prod showed both spellings in use, both are listed; the language is
	// agglutinative and an exhaustive sweep of its plurals, hyphenations and case
	// endings is future work, not done here — the same stance this file takes on
	// gendered forms above. That is why "java fejlesztők" and "odoo-fejlesztő" have
	// entries and the other twelve entries' plurals do not: those are the surface
	// forms the catalogue actually carries.
	{"szoftverfejlesztő", "software_engineering"},
	{"szoftver fejlesztő", "software_engineering"},
	{"szoftvermérnök", "software_engineering"},
	{"software mérnök", "software_engineering"},
	{"alkalmazásfejlesztő", "software_engineering"},
	{"alkalmazás fejlesztő", "software_engineering"},
	{"webfejlesztő", "software_engineering"},
	{"java fejlesztő", "software_engineering"},
	// The plural surface form does not contain the singular as a whole word.
	{"java fejlesztők", "software_engineering"},
	{"php fejlesztő", "software_engineering"},
	{"python fejlesztő", "software_engineering"},
	{"node.js fejlesztő", "software_engineering"},
	{".net fejlesztő", "software_engineering"},
	{"c# fejlesztő", "software_engineering"},
	{"adatbázis fejlesztő", "software_engineering"},
	{"abap fejlesztő", "software_engineering"},
	{"pega fejlesztő", "software_engineering"},
	{"odoo fejlesztő", "software_engineering"},
	{"odoo-fejlesztő", "software_engineering"},
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
	// The same four under `developer`, and for the same reason: the bare
	// "systems developer" below would otherwise sweep every one of them into software,
	// exactly as the bare "systems engineer" would have. They are listed even though no
	// such title is common, because the blindness has to be declared where the sweep
	// happens — the twin block above is the only thing that makes this one obvious.
	{"control systems developer", categoryNone},
	{"power systems developer", categoryNone},
	{"electrical systems developer", categoryNone},
	{"quality systems developer", categoryNone},
	// Then the qualified IT spellings, each naming its own discipline.
	{"linux systems engineer", "devops"},
	{"cyber systems engineer", "security"},
	{"software systems engineer", "software_engineering"},
	{"it systems engineer", "software_engineering"},
	{"software systems developer", "software_engineering"},
	{"it systems developer", "software_engineering"},
	// The generic technical analyst titles, ~2.2k live postings between them. They were
	// reached only through the bare "analyst" fall-through, which called them data
	// analysts; the software_engineering bucket is the same answer this file gives every
	// title it confirms is IT work without naming a sub-discipline.
	//
	// "it analyst" is why the boundary match matters rather than a substring one: 982
	// live "Credit Analyst" postings CONTAIN it, and a substring match would file every
	// one of them as IT staff.
	{"it analyst", "software_engineering"},
	{"technology analyst", "software_engineering"},
	{"technical analyst", "software_engineering"},
	{"software analyst", "software_engineering"},
	// Both surface forms: the plural does not contain the singular on a word boundary,
	// and prod carries far more of the plural (healthcare's Epic/Cerner analysts).
	{"application analyst", "software_engineering"},
	{"applications analyst", "software_engineering"},
	// The bare form closes the family. `software_engineering` and not `devops`: the
	// population left after the four blind spellings is mixed between infrastructure
	// and generalist software work, and the generic bucket is the honest answer where
	// devops would be a guess.
	{"systems engineer", "software_engineering"},
	{"system engineer", "software_engineering"},
	{"systems developer", "software_engineering"},
	{"system developer", "software_engineering"},

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
	// 256 live postings, previously read as data analysts.
	{"network analyst", "network_engineering"},
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
	// The lifeguard and the security guard are protective service (SOC 33-9092 and
	// 33-9032) and resolve `public_safety`: the lifeguard in the public_safety block
	// above, the guard up in the security block, where its entries sit beside the
	// information-security spellings they must not be confused with.
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
	// "Safety Engineer" is the HSE profession, not the plant seat around it — SOC's own
	// reported-title list for 19-5011 names it.
	//
	// It stays at this position because that is where it already was, not because the
	// position does anything: an earlier draft of this comment claimed moving it up would
	// take "Software Safety Engineer" away from `software_engineering`, and that was
	// simply false. The matcher takes contiguous phrases, so "software safety engineer"
	// never contained "software engineer" and resolved here either way. Verify a claim
	// like that against the matcher before writing it down — a reader trusts the comment
	// over the code.
	{"safety engineer", "occupational_safety"},
	{"environmental engineer", "industrial_engineering"},
	{"geotechnical engineer", "industrial_engineering"},
	{"planning engineer", "industrial_engineering"},
	{"resident engineer", "industrial_engineering"},
	// The physical engineering disciplines. nontech.go named them as non-technical long
	// before this block existed, and with no category to file them under the ingest
	// filter turned every one of them away. A software role in the same domain does not
	// contain the phrase ("Electrical Systems Engineer" is pinned to no category far
	// above), and a hardware one resolves through "hardware" first.
	{"electrical engineer", "industrial_engineering"},
	{"mechanical engineer", "industrial_engineering"},
	{"civil engineer", "industrial_engineering"},
	{"structural engineer", "industrial_engineering"},
	{"quantity surveyor", "industrial_engineering"},

	// The non-technical title dictionary's own vocabulary (nontech.go), filed. Those
	// terms said a posting was not technical but not what it WAS, and a posting the
	// dictionary placed nowhere is one ingest refuses and search excludes. Each term
	// below names the job outright, so it earns the category it names.
	//
	// Declared HERE — after every technical block and after the industrial seats —
	// so that a title any alias above already resolves keeps exactly the category it
	// had. That is what makes this block safe to add in bulk: "HVAC Controls Engineer"
	// stays industrial and "DevOps Engineer (HVAC IoT Platform)" stays devops, and only
	// a title that named nothing else reaches these lines.
	//
	// Healthcare.
	{"behavior technician", "healthcare"},
	{"rbt", "healthcare"},
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
	// The service advisor is the customer-facing desk of a car dealership, a bank
	// branch or a member service — `support` in every case. "Customer Service Advisor"
	// resolves there through "customer service" far above.
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
	// The production floor is `manufacturing` (SOC 51), not a trade. These four keep
	// the precedence they were declared with; the rest of that family is filed in the
	// manufacturing block at the end of this table.
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

	// The Brazilian-Portuguese half of the same vocabulary — the gupy tail.
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

	// 1С (the RU enterprise/ERP dev platform) resolves last so a more specific role word in the
	// title wins first ("Аналитик 1С" → data_analytics, "Тестировщик 1С" → qa); a title whose only
	// signal is 1С ("Программист 1С", "1С-разработчик") reads as backend — server-side enterprise
	// development. Bare tokens so any separator ("1С-разработчик", "разработчик 1С") matches.
	{"1c", "backend"},
	// "Аналитик 1С" is a systems/business analyst on the 1C platform, not a data one:
	// of the 71 live postings the catalogue carries, the fuller spellings say so
	// outright ("Системный аналитик 1С", "Senior Системный аналитик_1С", "Аналитик
	// 1С:ERP"). It needs its own entry now that the bare "аналитик" fall-through is
	// gone — without it the title drops through to the 1C rule below and reads as
	// backend, which is the one thing it certainly is not.
	{"аналитик 1с", "business_analysis"},
	{"1с аналитик", "business_analysis"},
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

	// German administration/technician/tester/developer fused compounds. German
	// joins a title's role words into one unbroken word with no separator, so
	// none of these can be reached by the spaced English alias they otherwise
	// match — same doctrine as the "разработчик" bare tokens above, except a
	// German compound has no internal separator at all, so the alias must be
	// the fused form itself rather than relying on a hyphen/space boundary
	// inside it. A hyphen or space BEFORE the compound is still a boundary,
	// so each bare alias below already reaches an "IT-"/"IT "-prefixed title
	// without a separate entry.
	{"systemadministrator", "devops"},
	{"netzwerkadministrator", "network_engineering"},
	{"datenbankadministrator", "devops"},
	{"netzwerktechniker", "network_engineering"},
	{"softwaretester", "qa"},
	{"anwendungsentwickler", "software_engineering"},

	// Systemtechniker/Systemelektroniker also name non-IT disciplines in prod
	// titles ("Systemtechniker Elektrotechnik", "Systemtechniker
	// Sicherheitstechnik") — the same cross-domain trap the Systems Engineer
	// family below documents. Only the IT-qualified spellings resolve; the
	// bare word is deliberately absent, and hyphenated/spaced forms are two
	// different strings to this matcher so both need their own entry.
	{"it systemtechniker", "devops"},
	{"it-systemtechniker", "devops"},
	{"it systemelektroniker", "devops"},
	{"it-systemelektroniker", "devops"},

	// Fachinformatiker: the German formal IT-specialist title and
	// apprenticeship. Unlike Systemtechniker above, it never names a non-IT
	// role, so the bare word resolves too — but declared LAST, after its two
	// dominant qualifiers, so a title where the qualifier sits directly next
	// to the word (no intervening "für"/"/in"/"m/w/d") gets the more precise
	// category. SPS-Programmierer (PLC/industrial-controller programming) is
	// deliberately NOT given an entry here — already excluded from a software
	// category, same reasoning as "CNC Programmer" above.
	{"fachinformatiker systemintegration", "devops"},
	{"fachinformatiker für systemintegration", "devops"},
	{"fachinformatiker anwendungsentwicklung", "software_engineering"},
	{"fachinformatiker für anwendungsentwicklung", "software_engineering"},
	{"fachinformatiker", "devops"},

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
	{"личный помощник", "administration"},
	{"личный ассистент", "administration"},
	{"виртуальный ассистент", "administration"},
	{"виртуальный помощник", "administration"},

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
	// A school's social worker, not a teacher: declared above the bare "педагог" that
	// would otherwise take it.
	{"социальный педагог", "social_services"},
	{"педагог", "education"},
	{"методист", "education"},

	{"парикмахер", "personal_services"},
	{"уборщик", "personal_services"},
	{"уборщица", "personal_services"},
	{"сиделка", "personal_services"},

	// The Russian half of the non-technical title dictionary, filed (see the English
	// block above the 1С entries for why). Bare tokens, as everywhere in Russian here:
	// "Продавец-кассир" and "Продавец-консультант" are reached by "продавец", since a
	// hyphen is a word boundary.
	{"медицинская сестра", "healthcare"},
	{"санитарка", "healthcare"},
	{"логопед", "healthcare"},
	{"электрогазосварщик", "skilled_trades"},
	{"сантехник", "skilled_trades"},
	{"токарь", "skilled_trades"},
	{"фрезеровщик", "skilled_trades"},
	{"штукатур", "skilled_trades"},
	{"каменщик", "skilled_trades"},
	{"бетонщик", "skilled_trades"},
	{"арматурщик", "skilled_trades"},
	{"стропальщик", "skilled_trades"},
	{"монтажник", "skilled_trades"},
	{"машинист", "skilled_trades"},
	{"тракторист", "skilled_trades"},
	{"разнорабочий", "skilled_trades"},
	{"подсобный рабочий", "skilled_trades"},
	{"повар", "hospitality"},
	{"пекарь", "hospitality"},
	{"кондитер", "hospitality"},
	{"официант", "hospitality"},
	{"официантка", "hospitality"},
	{"бармен", "hospitality"},
	{"посудомойщик", "hospitality"},
	{"кухонный рабочий", "hospitality"},
	{"продавец", "retail"},
	{"кассир", "retail"},
	{"товаровед", "retail"},
	{"мерчендайзер", "retail"},
	{"директор магазина", "retail"},
	{"комплектовщик", "logistics"},
	{"упаковщик", "logistics"},
	{"фасовщик", "logistics"},
	{"приёмщик", "logistics"},
	{"приемщик", "logistics"},
	{"дворник", "personal_services"},
	{"вахтер", "personal_services"},
	{"вахтёр", "personal_services"},
	{"консьерж", "personal_services"},
	{"няня", "personal_services"},
	{"маникюрша", "personal_services"},
	{"косметолог", "personal_services"},
	{"учитель", "education"},

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

	// Media production, declared after every discipline block on purpose (only the
	// three SOC families below follow it, and they reach no title anything above
	// resolves). Every craft here is also a tool or a
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

	// SOCIAL SERVICES, SCIENCE AND MANUFACTURING — the three families the catalogue
	// filed nowhere once it grew past IT, so ingest refused them and search excluded
	// them. Declared after every other block, the media crafts included, so a title any
	// alias above resolves keeps exactly the category it had: these lines only reach a
	// title that named nothing else. The one deliberate move is the four production-
	// floor spellings above, re-filed from `skilled_trades` in place.
	//
	// Community and social service (SOC 21). Counsellors and therapists who treat
	// mental health stay in `healthcare`, where they already resolve. A bare "case
	// manager" is a nurse as often as a social worker and resolves nothing; only the
	// qualified forms are here.
	{"social worker", "social_services"},
	{"social workers", "social_services"},
	{"social work", "social_services"},
	{"social services", "social_services"},
	{"social service", "social_services"},
	// Qualified: a bare "human services" is also a consulting practice ("Health and
	// Human Services Strategy").
	{"human services worker", "social_services"},
	{"human services specialist", "social_services"},
	{"human services assistant", "social_services"},
	{"human services professional", "social_services"},
	{"lcsw", "social_services"},
	{"lmsw", "social_services"},
	{"licsw", "social_services"},
	{"case worker", "social_services"},
	{"caseworker", "social_services"},
	{"child welfare", "social_services"},
	{"child protective", "social_services"},
	{"foster care", "social_services"},
	{"community liaison", "social_services"},
	{"outreach worker", "social_services"},
	{"community health worker", "social_services"},
	{"youth worker", "social_services"},
	{"youth care worker", "social_services"},
	{"youth counselor", "social_services"},
	{"family advocate", "social_services"},
	{"victim advocate", "social_services"},
	{"victim services", "social_services"},
	{"substance abuse counselor", "social_services"},
	{"substance use counselor", "social_services"},
	{"addiction counselor", "social_services"},
	{"drug and alcohol counselor", "social_services"},
	{"recovery coach", "social_services"},
	{"rehabilitation counselor", "social_services"},
	// An after-school counselor runs the afternoon club, not the guidance office; it
	// would otherwise reach "school counselor" across the hyphen.
	{"after school counselor", "education"},
	{"after-school counselor", "education"},
	{"school counselor", "social_services"},
	{"guidance counselor", "social_services"},
	{"probation officer", "social_services"},
	{"parole officer", "social_services"},
	{"chaplain", "social_services"},
	{"pastor", "social_services"},
	{"социальный работник", "social_services"},
	{"соцработник", "social_services"},
	{"социальной работе", "social_services"}, // "специалист по социальной работе"
	{"социальной работы", "social_services"}, // "... ведения социальной работы"
	// German has no morphology here either: the -in and -en forms are their own
	// words, while the gender star and colon are word boundaries ("Sozialpädagog*in").
	{"sozialarbeiter", "social_services"},
	{"sozialarbeiterin", "social_services"},
	{"schulsozialarbeiter", "social_services"},
	{"schulsozialarbeiterin", "social_services"},
	{"sozialpädagoge", "social_services"},
	{"sozialpädagogin", "social_services"},
	{"sozialpädagogen", "social_services"},
	{"sozialpädagog", "social_services"},
	{"travailleur social", "social_services"},
	{"travailleuse sociale", "social_services"},
	{"assistant social", "social_services"},
	{"assistante sociale", "social_services"},
	{"éducateur spécialisé", "social_services"},
	{"éducatrice spécialisée", "social_services"},
	{"assistente social", "social_services"},
	{"trabajador social", "social_services"},
	{"trabajadora social", "social_services"},

	// Science (SOC 19): the laboratory and the named sciences.
	//
	// The clinical laboratory comes first and goes to `healthcare`: SOC files medical
	// and clinical laboratory technologists and technicians under 29-2010, the
	// healthcare practitioners, not with the science technicians of 19-4000. The
	// hospital lab is staffed and hired as part of patient care. A cath lab is a
	// cardiac procedure room, not a laboratory at all.
	{"medical lab technician", "healthcare"},
	{"medical laboratory technician", "healthcare"},
	{"medical lab tech", "healthcare"},
	{"medical laboratory technologist", "healthcare"},
	{"medical laboratory scientist", "healthcare"},
	{"medical lab scientist", "healthcare"},
	{"clinical lab technician", "healthcare"},
	{"clinical laboratory technician", "healthcare"},
	{"clinical laboratory technologist", "healthcare"},
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
	// The school or campus computer lab is IT support, filed beside "it technician".
	{"computer lab technician", "support"},
	{"computer laboratory technician", "support"},
	{"computer lab assistant", "support"},
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
	{"инспектор по кадрам", "hr"},
	{"hr-менеджер", "hr"},
	{"office coordinator", "administration"},
	{"заведующий хозяйством", "administration"},
	{"заведующая хозяйством", "administration"},
	{"завхоз", "administration"},
	{"buyer", "operations"},
	{"buyers", "operations"},
	{"закупкам", "operations"},
	{"госзакупкам", "operations"},
	{"менеджер по мто", "operations"},
	// The rest of the Russian "менеджер ..." compounds the bare "менеджер" used to file.
	{"менеджер проектов", "project_management"},
	{"менеджер проекта", "project_management"},
	{"pr-менеджер", "marketing"},
	{"продаже", "sales"},
	{"ключевыми клиентами", "sales"},
	{"территориальный менеджер", "sales"},
	// Sales: the territory manager is a field-sales seat ("Territory Business Manager"
	// in pharma). The area and district manager is NOT — in this catalogue it is a
	// retail or restaurant chain's multi-unit operator — so it has no category and
	// reaches search through role_type.
	{"territory manager", "sales"},
	{"territory business manager", "sales"},
	// IT: the manager of a company's IT estate, beside "it administrator".
	{"it manager", "devops"},
	// Personal services. A groundskeeper is personal and facility services here, so the
	// Russian green-space worker is filed with it, and the culture organiser is a
	// recreation worker (SOC 39-9032).
	{"рабочий зеленого хозяйства", "personal_services"},
	{"рабочий зелёного хозяйства", "personal_services"},
	{"культорганизатор", "personal_services"},

	// The functional managers: a manager title whose craft is a business function the
	// table already files. Each was the residue of the retired fall-through in volume,
	// and each names its function as surely as "Finance Manager" always did.
	{"portfolio manager", "finance"},
	{"risk manager", "finance"},
	{"credit manager", "finance"},
	{"it audit manager", "security"},
	{"audit manager", "finance"},
	{"internal audit", "finance"},
	{"investor relations", "finance"},
	{"underwriting", "finance"},
	{"f&i manager", "finance"},
	{"finance and insurance manager", "finance"},
	{"finance & insurance manager", "finance"},
	{"people & culture", "hr"},
	{"people and culture", "hr"},
	{"commercial manager", "sales"},
	{"partnerships manager", "sales"},
	{"partnership manager", "sales"},
	{"partner manager", "sales"},
	{"bid manager", "sales"},
	{"proposal manager", "sales"},
	{"partner success manager", "customer_success"},
	{"success manager", "customer_success"},
	{"implementation manager", "customer_success"},
	{"growth manager", "marketing"},
	{"affiliate manager", "marketing"},
	{"communications manager", "marketing"},
	{"category manager", "operations"},
	{"sourcing manager", "operations"},
	{"strategic sourcing", "operations"},
	{"commodity manager", "operations"},
	{"operation manager", "operations"},
	{"facility manager", "operations"},
	{"administrative manager", "administration"},
	{"administration manager", "administration"},
	{"training manager", "hr"},
	{"compensation manager", "hr"},
	{"employee relations", "hr"},
	{"labor relations", "hr"},
	{"labour relations", "hr"},
	{"industrial relations", "hr"},
	{"human resource", "hr"},
	{"analytics manager", "data_analytics"},
	{"release manager", "devops"},
	{"bim manager", "engineering_design"},
	{"continuous improvement", "industrial_engineering"},
	{"commissioning manager", "industrial_engineering"},
	{"cost manager", "industrial_engineering"},
	{"environmental manager", "occupational_safety"},
	{"eh&s", "occupational_safety"},
	{"quality control manager", "manufacturing"},
	{"project controls", "project_management"},
	{"preconstruction", "skilled_trades"},
	{"distribution manager", "logistics"},
	{"inventory manager", "logistics"},
	{"materials manager", "logistics"},
	{"material manager", "logistics"},
	{"care manager", "healthcare"},
	{"clinical manager", "healthcare"},
	{"clinic manager", "healthcare"},
	{"pharmacy manager", "healthcare"},
	{"laboratory manager", "science"},
	{"lab manager", "science"},
	{"deli manager", "retail"},
	{"bakery manager", "retail"},
	{"meat manager", "retail"},
	{"produce manager", "retail"},
	{"perishable manager", "retail"},
	{"perishables manager", "retail"},
	{"beverage manager", "hospitality"},
	{"cafe manager", "hospitality"},
	{"café manager", "hospitality"},
	{"reservations manager", "hospitality"},
	{"spa manager", "personal_services"},
	{"fitness manager", "personal_services"},
	// Film and television: the post-production manager is the media craft, not the plant.
	{"post production", "creative"},
	{"post-production", "creative"},
	{"менеджер (в торговле)", "retail"},
	{"менеджер в торговле", "retail"},
	{"в коммерческой деятельности", "sales"},
	{"категорийный менеджер", "operations"},

	// PUBLIC SAFETY, the rest of the family (see its first block, above logistics). These
	// words name the employer as often as the job, so they reach only a title that
	// named no craft at all.
	{"police", "public_safety"},
	{"sheriff", "public_safety"},
	{"correctional officer", "public_safety"},
	{"corrections officer", "public_safety"},
	{"correctional sergeant", "public_safety"},
	{"correctional lieutenant", "public_safety"},
	{"correctional captain", "public_safety"},
	{"corrections sergeant", "public_safety"},
	{"corrections lieutenant", "public_safety"},
	{"corrections captain", "public_safety"},
	{"corrections deputy", "public_safety"},
	{"detention officer", "public_safety"},
	{"detention deputy", "public_safety"},
	{"jailer", "public_safety"},
	{"bailiff", "public_safety"},
	{"deputy marshal", "public_safety"},
	{"fire marshal", "public_safety"},
	{"patrol officer", "public_safety"},
	// Retail loss prevention is store security (SOC 33-9021/33-9032). The data-loss
	// prevention discipline is information security and is pinned first.
	{"data loss prevention", "security"},
	{"loss prevention", "public_safety"},
	{"asset protection", "public_safety"},
	{"охранник", "public_safety"},
	{"охранница", "public_safety"},
	{"сторож", "public_safety"},
	{"вохр", "public_safety"},
	{"полицейский", "public_safety"},
	{"сотрудник полиции", "public_safety"},
	{"участковый уполномоченный", "public_safety"},
	{"инспектор дпс", "public_safety"},
	{"следователь", "public_safety"},
	{"дознаватель", "public_safety"},
	{"пожарный", "public_safety"},
	{"пожарной части", "public_safety"},
	{"пожарной команды", "public_safety"},
	{"спасатель", "public_safety"},
	{"sicherheitsmitarbeiter", "public_safety"},
	{"sicherheitsmitarbeiterin", "public_safety"},
	{"sicherheitsdienst", "public_safety"},
	{"schutz und sicherheit", "public_safety"},
	{"objektschutz", "public_safety"},
	{"werkschutz", "public_safety"},
	{"polizist", "public_safety"},
	{"polizistin", "public_safety"},
	{"polizeibeamter", "public_safety"},
	{"polizeibeamtin", "public_safety"},
	{"polizeivollzugsbeamter", "public_safety"},
	{"polizeivollzugsbeamtin", "public_safety"},
	{"feuerwehrmann", "public_safety"},
	{"feuerwehrfrau", "public_safety"},
	{"berufsfeuerwehr", "public_safety"},
	{"werkfeuerwehr", "public_safety"},
	{"brandmeister", "public_safety"},
	// Brazilian Portuguese: "vigilante" is the security guard; "porteiro", the doorman,
	// stays with the personal services.
	{"vigilante", "public_safety"},

	// THE REMOTE TAIL. Measured 2026-10-19: of 462k open remote postings, 183k resolved
	// no category, and a posting without one never reaches search. Every spelling below
	// was read against that corpus and then against a 3% sample of the whole open
	// catalogue; together they place ~26k of the remote postings and ~107k overall. What
	// is left is long and flat, or names no craft at all ("General Application",
	// "Talent Pool", the "work from home opportunity" funnels), and stays unresolved.
	//
	// Left out on purpose: "AI Trainer" is the domain it trains in ("CAD Engineer - AI
	// Trainer"), not a craft of its own; "Product Engineer", "Tech Lead" and "Engineering
	// Lead" split between software and the plant floor, the same reason "Product Engineer"
	// is pinned elsewhere; bare "counselor" is financial, camp and admissions as often as
	// clinical; bare "journeyman" is also a US federal pay grade.
	//
	// Selling: real estate, insurance and the benefits funnels, which sell policies.
	{"real estate agent", "sales"},
	{"real estate showing agent", "sales"},
	{"real estate acquisition", "sales"},
	{"estate agent", "sales"},
	{"agent immobilier", "sales"},
	{"conseiller immobilier", "sales"},
	{"chasseur immobilier", "sales"},
	{"insurance agents", "sales"},
	{"insurance representative", "sales"},
	{"life insurance", "sales"},
	{"final expense", "sales"},
	{"insurance broker", "sales"},
	{"benefits representative", "sales"},
	{"client benefits", "sales"},
	{"appointment setter", "sales"},
	// Lending and billing.
	{"financial services professional", "finance"},
	{"financial adviser", "finance"},
	{"loan originator", "finance"},
	{"mortgage", "finance"},
	{"billing specialist", "finance"},
	{"premium auditor", "finance"},
	{"finance director", "finance"},
	// Travel booking is the hospitality trade, home-based or not.
	{"travel agent", "hospitality"},
	{"travel advisor", "hospitality"},
	{"travel consultant", "hospitality"},
	{"travel specialist", "hospitality"},
	{"travel planner", "hospitality"},
	{"travel coordinator", "hospitality"},
	{"reservation specialist", "hospitality"},
	// Clinical. The spa therapist is the treatment room, pinned ahead of the bare word.
	// No "case manager": it is a nurse as often as a social worker (see the suite).
	{"spa therapist", "personal_services"},
	{"beauty therapist", "personal_services"},
	{"behavior analyst", "healthcare"},
	{"bcba", "healthcare"},
	{"therapist", "healthcare"},
	{"mental health", "healthcare"},
	{"speech language", "healthcare"},
	{"medical science liaison", "healthcare"},
	{"clinical research", "healthcare"},
	{"clinical trial", "healthcare"},
	{"psychologist", "healthcare"},
	{"psychotherapist", "healthcare"},
	{"professional counselor", "healthcare"},
	{"clinical counselor", "healthcare"},
	{"dietitian", "healthcare"},
	{"optometric assistant", "healthcare"},
	{"attorneys", "legal"},
	// Interpreting sits with translation (see "translator"). A parts interpreter reads a
	// parts catalogue at a dealership counter.
	{"parts interpreter", "retail"},
	{"interpreter", "technical_writing"},
	{"interpreters", "technical_writing"},
	// Marketing and creative.
	{"creative director", "creative"},
	{"creative strategist", "marketing"},
	{"digital marketer", "marketing"},
	{"ppc specialist", "marketing"},
	{"ppc manager", "marketing"},
	{"content editor", "marketing"},
	// IT spellings the families above never wrote down.
	{"react engineer", "frontend"},
	{"laravel developer", "backend"},
	{"chief technology officer", "software_engineering"},
	{"product analyst", "data_analytics"},
	// The vehicle trades, including the Spanish mechanic.
	{"mecánico", "skilled_trades"},
	{"técnico de motores", "skilled_trades"},
	{"tire technician", "skilled_trades"},
	{"rv technician", "skilled_trades"},
	{"small engine technician", "skilled_trades"},
	// The rest. "Profesor" is the Spanish tutor and teacher, the bulk of the thin
	// Latin American WhatJobs accounts.
	{"supply chain", "logistics"},
	{"transaction coordinator", "administration"},
	{"intake specialist", "administration"},
	{"intake coordinator", "administration"},
	{"english trainer", "education"},
	{"profesor", "education"},

	// CENTRAL AND EASTERN EUROPE (freehire#3513). Measured 2026-10-24 over the open,
	// canonical postings located in si/pl/ua/bg/hu/cz/hr/rs/sk/ba: 43% resolved no
	// category, and almost every Polish, Czech, Slovak, Slovenian, Croatian, Serbian,
	// Ukrainian and Bulgarian title among them was one this table had no word for —
	// a pharmaceutical employer's company page showed 12 of its 82 open postings.
	//
	// Declared at the END, behind every block above, so a title any earlier alias
	// resolves keeps exactly the category it had: these lines only reach a title that
	// named nothing else. Each alias files the role the way its English twin is filed
	// above ("Medical Representative" with the pharmaceutical sales rep, the
	// storekeeper with the warehouse, the controller with finance), and where the
	// English twin is deliberately unresolved — the bare quality, compliance, tax and
	// audit specialists, the generic "specialist"/"associate" nouns (specjalista,
	// фахівець, sodelavec, suradnik, munkatárs), interns and student work, the talent
	// pools — the native one is left unresolved too.
	//
	// The matcher has no morphology, and these languages inflect for case and gender,
	// so every alias is a surface form the catalogue carries: the masculine and the
	// feminine of a role noun, and the CASE a domain noun takes in a title. A Polish
	// title names its desk in the genitive ("Specjalista ds. sprzedaży", "Kierownik
	// Działu Logistyki"), a Slovenian or Croatian one in the locative ("v Marketingu",
	// "u proizvodnji"); the nominative of the same word would match neither.
	//
	// Within the block the role nouns come first and the domain nouns last, because
	// the table resolves in declaration order, not title order: "Magazynier - obsługa
	// produkcji" is a warehouse worker, not production, and "Lekarz medycyny pracy" a
	// doctor whatever else the title says.

	// English: the pharmaceutical field-sales rep, as "Pharmaceutical Sales
	// Representative" already resolves. The plural is its own word to this matcher.
	{"medical representative", "sales"},
	{"medical representatives", "sales"},
	{"pharmaceutical representative", "sales"},
	{"pharmaceutical representatives", "sales"},
	{"pharmacy representative", "sales"},
	{"pharmacy representatives", "sales"},
	// The same seat in each language. Hungarian calls it the doctor's and the
	// pharmacy's "visitor".
	{"przedstawiciel medyczny", "sales"},
	{"przedstawicielka medyczna", "sales"},
	{"przedstawiciel farmaceutyczny", "sales"},
	{"przedstawicielka farmaceutyczna", "sales"},
	{"przedstawiciel naukowy", "sales"},
	{"przedstawicielka naukowa", "sales"},
	{"przedstawiciel handlowy", "sales"},
	{"przedstawicielka handlowa", "sales"},
	{"medicínský reprezentant", "sales"},
	{"medicínská reprezentantka", "sales"},
	{"obchodní zástupce", "sales"},
	{"regionální zástupce", "sales"},
	{"obchodný zástupca", "sales"},
	{"medicinski predstavnik", "sales"},
	{"prodajni predstavnik", "sales"},
	{"prodajni savjetnik", "sales"},
	{"prodajna savjetnica", "sales"},
	{"broker ubezpieczeń", "sales"},
	{"broker ubezpieczeniowy", "sales"},
	{"brokerka ubezpieczeniowa", "sales"},
	{"orvoslátogató", "sales"},
	{"patikalátogató", "sales"},
	{"területi képviselő", "sales"},
	{"медичний представник", "sales"},
	{"медична представниця", "sales"},
	{"медицински представител", "sales"},
	{"медицинский представитель", "sales"},
	{"търговски представител", "sales"},
	{"представител медицински", "sales"},

	// Software. The machine programmers are pinned first: CNC, PLC, CAM, CAD and robot
	// programming is not software. The CNC programmer is a production occupation (SOC
	// 51-9162) and is filed on the floor; the rest name no category. The bare programmer
	// nouns themselves are declared at the very END of this block (see there).
	{"programista cnc", "manufacturing"},
	{"programistka cnc", "manufacturing"},
	{"programista plc", categoryNone},
	{"cnc programer", "manufacturing"},
	{"programer cnc", "manufacturing"},
	{"cnc programátor", "manufacturing"},
	{"programátor cnc", "manufacturing"},
	{"programátor plc", categoryNone},
	{"cnc programozó", "manufacturing"},
	{"plc programozó", categoryNone},
	{"plc programer", categoryNone},
	{"programista cam", categoryNone},
	{"cam programozó", categoryNone},
	{"programer plc", categoryNone},
	{"plc programátor", categoryNone},
	{"plc programista", categoryNone},
	{"programer cad", categoryNone},
	{"elektro programer", categoryNone},
	{"programista robotów", categoryNone},
	{"programátor robotů", categoryNone},
	{"programátor svařovacího robota", categoryNone},
	{"programer strojev", "manufacturing"},
	{"programer procesorjev", "hardware"},
	// The software engineer and developer, by the phrase rather than the bare noun.
	{"programski inženir", "software_engineering"},
	{"vgrajene programske opreme", "embedded"},
	{"programske opreme", "software_engineering"},
	{"razvoj softvera", "software_engineering"},
	{"sistemski inženir", "software_engineering"},
	{"sistemski inženjer", "software_engineering"},
	{"sistem inženjer", "software_engineering"},
	{"softverski inženjer", "software_engineering"},
	{"softverska inženjerka", "software_engineering"},
	{"машинного навчання", "ml_ai"},
	{"systémový inžinier", "software_engineering"},
	{"testni inženir", "qa"},
	{"preskusni inženir", "qa"},
	{"kibernetsko varnost", "security"},
	{"kibernetske varnosti", "security"},

	// SLOVENIAN JOB-BOARD TITLES (freehire#3518, contributed by @josepowera from 3,288
	// distinct Slovenian titles, curated here). The multi-word phrases come first, so a
	// longer title that names its craft is read before a shorter noun below would shadow
	// it ("vzdrževalec perila" is the laundry, not maintenance). Curation dropped the
	// catch-all nouns (vodja, specialist, sodelavec, svetovalec, delavec, operater,
	// referent, asistent, tehnik, ...), the civil-service ranks and the recruiting copy,
	// and re-filed what English files elsewhere: the sales engineer is
	// solutions_engineering, the systems engineer software_engineering, the quality
	// manager industrial_engineering, the receptionist administration.
	{"strokovnjak s področja informacijsko-komunikacijskih tehnologij", "devops"},
	{"strokovni svetovalec področja v raziskovalni dejavnosti", "science"},
	{"strokovnjak za mobilne komunikacije vgrajenih sistemov", "embedded"},
	{"študentsko delo: projektna in administrativna podpora", "administration"},
	{"it svetovalec na področjih digitalne transformacije", "solutions_engineering"},
	{"strokovni sodelavec za spremljavo kreditnih tveganj", "finance"},
	{"specialist za digitalizacijo in ai avtomatizacijo", "ai_engineering"},
	{"vodja informacijskih tehnologij in digitalizacije", "devops"},
	{"strokovni sodelavec za telekomunikacijska omrežja", "network_engineering"},
	{"vodja strateškega upravljanja prodajnega prostora", "marketing"},
	{"specialist za kontroling in informacijski sistem", "finance"},
	{"specialist za informacijsko in fizično varnost", "security"},
	{"pomočnik vodje računovodsko-finančnega oddelka", "finance"},
	{"strokovni sodelavec za hortikulturo in prodajo", "sales"},
	{"sodelavec za komunikacijo z nemškimi strankami", "support"},
	{"vodja službe za vzdrževanje strojne energetike", "skilled_trades"},
	{"specialist za administracijo in računovodstvo", "finance"},
	{"produktni specialist za tehnično in digitalno", "sales"},
	{"strokovni vodja izvajalca socialno varstvenih", "social_services"},
	{"vodja medpodjetniškega izobraževalnega centra", "education"},
	{"strokovni sodelavec za upravno pravne zadeve", "legal"},
	{"strokovni sodelavec za operativni kontroling", "finance"},
	{"strokovni sodelovec za operativni kontroling", "finance"},
	{"strokovni sodelavec vii/2 - kadrovske zadeve", "hr"},
	{"regionalni predstavnik logističnih projektov", "sales"},
	{"višji svetovalec v pravosodju - računovodja", "finance"},
	{"vodja majhne in srednje velike pravne osebe", "finance"},
	{"strokovni delavec v raziskovalni dejavnosti", "science"},
	{"strokovni delavec v visokošolski dejavnosti", "education"},
	{"sprejemnik v kleparsko - ličarski delavnici", "support"},
	{"strokovni delavec-za zaposlovanje invalidov", "social_services"},
	{"specialist za davčno skladnost in e-račune", "finance"},
	{"organizator zdravstveno higienskega režima", "occupational_safety"},
	{"strokovni sodelavec vii/2 - javna naročila", "operations"},
	{"asistent za podporo partnerjem in strankam", "support"},
	{"skupinovodja v enoti asfalterske operative", "skilled_trades"},
	{"svetovalec za implementacijo itsm rešitev", "solutions_engineering"},
	{"strokovni sodelavec za odnose z javnostmi", "marketing"},
	{"inženir varnosti informacijskih sistemov", "security"},
	{"svetovalec za poslovno programsko opremo", "solutions_engineering"},
	{"strokovni sodelavec v asistenčnem centru", "support"},
	{"sodelavec za dostavo in razdelitev hrane", "hospitality"},
	{"študentsko delo: administrativna podpora", "administration"},
	{"vodja proizvodnih znanosti in tehnologij", "science"},
	{"sodelavec za delo na razvojnih projektih", "project_management"},
	{"direktor za ai in podatkovno strategijo", "ai_engineering"},
	{"strokovni delavec za varnost in zdravje", "occupational_safety"},
	{"sodelavec za iskanje in selekcijo kadra", "recruiting"},
	{"specialist za razvoj trga in poslovanja", "sales"},
	{"administrator za logistiko in skladišče", "logistics"},
	{"poslovodja poslovno-zabaviščnega centra", "hospitality"},
	{"delovodja v enoti asfalterske operative", "skilled_trades"},
	{"elektrikar v proizvodnji elektro omaric", "manufacturing"},
	{"sodelavec v nadzorno-operativnem centru", "public_safety"},
	{"vodja razvoja nepremičninskih projektov", "project_management"},
	{"razvojni inženir za umetno inteligenco", "ml_ai"},
	{"specialist za finance in računovodstvo", "finance"},
	{"višji specialist za finančno poročanje", "finance"},
	{"vodja finančno-računovodskega sektorja", "finance"},
	{"vodja upravljanja kreditnega portfelja", "finance"},
	{"vodja oddelka transakcijsko bančništvo", "finance"},
	{"področni prodajno tehnični strokovnjak", "sales"},
	{"referent za sprejem in podporo servisu", "support"},
	{"pomočnik v transportnem delu skladišča", "logistics"},
	{"analitik za varnost elektronske pošte", "security"},
	{"diplomirani inženir tehniške varnosti", "occupational_safety"},
	{"računovodske in knjigovodske storitve", "finance"},
	{"koordinator za prihode tujih delavcev", "hr"},
	{"vodja razvoja poslovanja za nove trge", "sales"},
	{"grafični in mulzimedijski oblikovalec", "design"},
	{"vodja otroškega centra z restavracijo", "hospitality"},
	{"cnc programer in nastavljalec strojev", "manufacturing"},
	{"tehnični svetovalec za servisno mrežo", "support"},
	{"strokovni sodelavec za pravne zadeve", "legal"},
	{"strokovni sodelavec za računovodstvo", "finance"},
	{"referent za finance in računovodstvo", "finance"},
	{"referent specialist za računovodstvo", "finance"},
	{"specialist za sklepanje pozavarovanj", "finance"},
	{"vodja enote v visokošolski dejavnost", "education"},
	{"referent za kadrovsko administracijo", "hr"},
	{"vodja za montažo strojnih instalacij", "skilled_trades"},
	{"spremljevalec gibalno oviranih otrok", "social_services"},
	{"koordinator za preprečevanje nasilja", "social_services"},
	{"operater varnostno nadzornega centra", "public_safety"},
	{"operativni vodja fizičnega varovanja", "public_safety"},
	{"sodelavec v računovodskem/finančnem", "finance"},
	{"specialist vizualne podobe trgovine", "retail"},
	{"vodja tima v poslovalnici halfprice", "retail"},
	{"vodja programa vodovod in ogrevanje", "skilled_trades"},
	{"predstojnik oddelka za radiologijo", "healthcare"},
	{"analitik v laboratorijski medicini", "healthcare"},
	{"inženir vgrajene programske opreme", "embedded"},
	{"oblikovale kovin/orodjar/programer", "manufacturing"},
	{"vodja finančno računovodske službe", "finance"},
	{"svetovalec za poslovna zavarovanja", "sales"},
	{"organizator izobraževanja odraslih", "education"},
	{"specialist za kreativno strategijo", "marketing"},
	{"administrator v skladiščni pisarni", "logistics"},
	{"administrator v servisni delavnici", "administration"},
	{"vodja enostavnejšega gospodinjstva", "personal_services"},
	{"namestnik upravitelja restavracije", "hospitality"},
	{"vodja poslovno-zabaviščnega centra", "hospitality"},
	{"izvajalec specialnih gradbenih del", "skilled_trades"},
	{"vodja razvoja prehranskih dodatkov", "manufacturing"},
	{"vodja oddelka elektro konstrukcije", "engineering_design"},
	{"operativni vodja oddelka projektov", "project_management"},
	{"programer cnc obdelovalnih strojev", "manufacturing"},
	{"it inženir za uporabniško podporo", "support"},
	{"it skrbnik za podporo uporabnikom", "support"},
	{"svetovalci za podporo uporabnikom", "support"},
	{"svetovalec za podporo uporabnikom", "support"},
	{"it svetovalec za poslovne rešitve", "solutions_engineering"},
	{"koordinator za varnost in zdravje", "occupational_safety"},
	{"strokovnjak za regulativne zadeve", "legal"},
	{"sodelavec za provizijske obračune", "finance"},
	{"prodajno administrativni referent", "sales"},
	{"predstavnik za odnose z javnostmi", "marketing"},
	{"specialist za klinične aplikacije", "healthcare"},
	{"administrator in koordinator dela", "administration"},
	{"operater nakladalnika v betonarni", "skilled_trades"},
	{"vodja izvedbe gradbenih projektov", "skilled_trades"},
	{"delavec na prometni signalizaciji", "skilled_trades"},
	{"kontrolor čiščenja in vzdrževanja", "personal_services"},
	{"koordinator za dolgotrajno oskrbo", "social_services"},
	{"sodelavec za mladinske dejavnosti", "social_services"},
	{"vodja tehnične podpore in servisa", "support"},
	{"predstojnik kliničnega inštituta", "healthcare"},
	{"skrbnika informacijskih sistemov", "devops"},
	{"strokovni sodelavec za ekologijo", "science"},
	{"finančno - komercialni sodelavec", "finance"},
	{"višji naravovarstveni svetovalec", "science"},
	{"direktor centra za socialno delo", "social_services"},
	{"svetovalec za iskanje& selekcijo", "recruiting"},
	{"strokovni sodelavec za kadrovsko", "hr"},
	{"strokovnjak za kadrovske procese", "hr"},
	{"administrator v prodajni pisarni", "sales"},
	{"specialist za amazon oglaševanje", "marketing"},
	{"vodja izmene v grafični dodelavi", "manufacturing"},
	{"administrator/turistični delavec", "hospitality"},
	{"sodelavec za promet in operativo", "logistics"},
	{"referent strojnega in avto parka", "logistics"},
	{"upravljalec skladiščnih procesov", "logistics"},
	{"svetovalec v turistični agenciji", "hospitality"},
	{"svetovalec za dolgotrajno oskrbo", "social_services"},
	{"vodja polnitev olja v steklenice", "manufacturing"},
	{"vodja oddelka splošne skladnosti", "legal"},
	{"koordinator produkta in sistemov", "product"},
	{"vodja področja tržnih dejavnosti", "sales"},
	{"skrbnik informacijskega sistema", "devops"},
	{"urejanje pravno kadrovskh zadev", "legal"},
	{"strokovni sodelavec kontrolinga", "finance"},
	{"produktni specialist za kredite", "finance"},
	{"vodja poslovnega razvoja za trg", "sales"},
	{"oskrbovalec prodajnih avtomatov", "sales"},
	{"specialist za kreativne vsebine", "marketing"},
	{"administrator v zaledni pisarni", "administration"},
	{"prometnik v transportni pisarni", "logistics"},
	{"specialist za logistiko naročil", "logistics"},
	{"specialist za načrtovanje zalog", "logistics"},
	{"skrbnik za skladišče in dostavo", "logistics"},
	{"sodelavec za higieno in čistočo", "personal_services"},
	{"izvajalec srednje zahtevnih del", "skilled_trades"},
	{"vodja enote oskrba s pitno vodo", "skilled_trades"},
	{"kontrolor v tehnološkem procesu", "manufacturing"},
	{"študentsko delo v vrtnem centru", "retail"},
	{"razvojno raziskovalni sodelavec", "science"},
	{"pomočnik pravosodnega policista", "public_safety"},
	{"vodja drevesnic in rastlinjakov", "skilled_trades"},
	{"specialist za projektno vodenje", "project_management"},
	{"specialist klinične logopedije", "healthcare"},
	{"specialist klinićne logopedije", "healthcare"},
	{"skrbnik računalniških sistemov", "devops"},
	{"strokovni sodelavec za omrežje", "network_engineering"},
	{"vpisničar v pravosodnem organu", "legal"},
	{"vodja splošno-kadrovske službe", "hr"},
	{"specialist za prodajno podporo", "sales"},
	{"referent za prodajno logistiko", "logistics"},
	{"produktni specialist na terenu", "sales"},
	{"nameščevalec oglasnih sporočil", "marketing"},
	{"svetovalec za delo s strankami", "support"},
	{"referent za odnose s strankami", "support"},
	{"sodelavec v svetovalnem centru", "support"},
	{"načrtovalec oskrbovalnih verig", "logistics"},
	{"delovodja - gradbena operativa", "skilled_trades"},
	{"pomožni delavec v gradbeništvu", "skilled_trades"},
	{"odgovorni operater na kontroli", "manufacturing"},
	{"kalibrirec na končnem prevzemu", "manufacturing"},
	{"študentsko laboratorijsko delo", "science"},
	{"specialist družinske medicine", "healthcare"},
	{"analitik v kontroli kakovosti", "science"},
	{"strokovnjak za javna naročila", "operations"},
	{"asistent v pravno - razvojnem", "legal"},
	{"ocenjevalec kreditnih tveganj", "finance"},
	{"upravljalec kreditnih tveganj", "finance"},
	{"referent višje strokovne šole", "education"},
	{"svetovalec v stalni arbitraži", "legal"},
	{"priložnost za prodajne zvezde", "sales"},
	{"predstavnik za podporo kupcem", "support"},
	{"referent za stike s strankami", "support"},
	{"skladiščno proizvodni delavec", "logistics"},
	{"poslovodja čistilnega servisa", "personal_services"},
	{"sodelavec v lokalu z burgerji", "hospitality"},
	{"pomožni delavec v grabeništvu", "skilled_trades"},
	{"vodja enote vodovodni sistemi", "skilled_trades"},
	{"operater na vizualni kontroli", "manufacturing"},
	{"kontrolor - popravljec vozila", "manufacturing"},
	{"usklajevalec osebne asistence", "social_services"},
	{"uskaljevalec osebne asistence", "social_services"},
	{"skrbnik laboratorijske opreme", "science"},
	{"svetovalec za javna naročila", "operations"},
	{"podsekretar za pravne zadeve", "legal"},
	{"svetovalec na bančnem okencu", "finance"},
	{"specialist za obračun davkov", "finance"},
	{"strokovnjak za računovodstvo", "finance"},
	{"višji svetovalec za podjetja", "finance"},
	{"strokovni sodelavec za kadre", "hr"},
	{"talent management specialist", "hr"},
	{"pridruži se novi ekipi rexel", "sales"},
	{"prodajni in produktni trener", "sales"},
	{"strokovni prodajni sodelavec", "sales"},
	{"specialist za amazon prodajo", "sales"},
	{"ambasador korejske kozmetike", "marketing"},
	{"strokovnjak za komuniciranje", "marketing"},
	{"content production assistant", "marketing"},
	{"vodja digitalnega marketinga", "marketing"},
	{"vodja zamrznjenega skladišča", "logistics"},
	{"namestnik/-ca vodje trgovine", "retail"},
	{"pomožni delavec v gostinstvu", "hospitality"},
	{"operater kontrolnih operacij", "manufacturing"},
	{"kontrola in pregled izdelkov", "manufacturing"},
	{"vodja raziskovalnega oddelka", "science"},
	{"spremljevalec v osnovni šoli", "education"},
	{"vodja razvoja in inženiringa", "industrial_engineering"},
	{"cnc operater | cnc programer", "manufacturing"},
	{"višji strokovnjak za prodajo", "sales"},
	{"pripravnik zdravstvene nege", "healthcare"},
	{"c++ in python ai razvijalci", "ml_ai"},
	{"analitik prodajnih operacij", "sales"},
	{"višji pravosodni svetovalec", "legal"},
	{"vodja kartičnega poslovanja", "finance"},
	{"vodja privatnega bančništva", "finance"},
	{"meta performance specialist", "marketing"},
	{"strateg za družbena omrežja", "marketing"},
	{"svetovalec v klicnem centru", "support"},
	{"administrator na info pultu", "administration"},
	{"vodja maloprodajnega salona", "retail"},
	{"vodja izmene v poslovalnici", "retail"},
	{"strokovni delavec v strežbi", "hospitality"},
	{"organizator šolske prehrane", "hospitality"},
	{"hostesa za delo na blagajni", "retail"},
	{"delovodja asfalterskih ekip", "skilled_trades"},
	{"operater kontrole kakovosti", "manufacturing"},
	{"vodja upravljanja kakovosti", "industrial_engineering"},
	{"rehabilitacijski svetovalec", "social_services"},
	{"asistent vodje poslovalnice", "administration"},
	{"zobozdravstvena asistentka", "healthcare"},
	{"predstojnik anesteziološko", "healthcare"},
	{"skrbnik spletne e-trgovine", "sales"},
	{"analitik energetskih trgov", "finance"},
	{"tehnik varnostnih sistemov", "skilled_trades"},
	{"referent za javna naročila", "operations"},
	{"specialist za obračun plač", "finance"},
	{"računovodski administrator", "finance"},
	{"administrator v komerciali", "sales"},
	{"koordinator prodajne mreže", "sales"},
	{"koordinator merchandisinga", "retail"},
	{"vodja izmene bavarski dvor", "retail"},
	{"negovalec delovnih oblačil", "personal_services"},
	{"delovodja -gradbeni tehnik", "skilled_trades"},
	{"vodja izvedbenih projektov", "skilled_trades"},
	{"izvajalec suhomontažni del", "skilled_trades"},
	{"vodja sektorja kontrolinga", "finance"},
	{"laboratorijski znanstvenik", "science"},
	{"vodja mladinskih programov", "social_services"},
	{"vodja skupine varnostnikov", "public_safety"},
	{"vodja proizvodnega oddelka", "manufacturing"},
	{"vodja operacije / projekta", "project_management"},
	{"asistent produktnega vodje", "product"},
	{"koordinator izvedbe poslov", "project_management"},
	{"planer razvojnih projektov", "project_management"},
	{"ai platform administrator", "devops"},
	{"prodajno razvojni inženir", "solutions_engineering"},
	{"sodelavec za sodne zadeve", "legal"},
	{"svetovalec pravnim osebam", "finance"},
	{"sodelavec za obračun plač", "finance"},
	{"referent prodajne podpore", "sales"},
	{"skrbnik strateških kupcev", "sales"},
	{"specialist podpore kupcem", "support"},
	{"vodja gradbenih projektov", "project_management"},
	{"vodja gradbenega programa", "project_management"},
	{"skupinovodja na gradbišču", "skilled_trades"},
	{"delavec suhomontažnih del", "skilled_trades"},
	{"vezalec električne opreme", "manufacturing"},
	{"operater kontrolne linije", "manufacturing"},
	{"kontrolor varilnih spojev", "manufacturing"},
	{"vodja računovodske službe", "finance"},
	{"kriminalistični inšpektor", "public_safety"},
	{"vodja zlatarske delavnice", "skilled_trades"},
	{"vodja prodajnega področja", "sales"},
	{"vodja sanitarnega oddelka", "healthcare"},
	{"diplomirani zdravstvenik", "healthcare"},
	{"doktor dentalne medicine", "healthcare"},
	{"zobozdravstveni asistent", "healthcare"},
	{"it logistični svetovalec", "solutions_engineering"},
	{"skrbnik erp-sistema odoo", "solutions_engineering"},
	{"agile bussnes specialist", "business_analysis"},
	{"sosvetovalec za podjetja", "finance"},
	{"admin v prodajni pisarni", "sales"},
	{"nepremičninski posrednik", "sales"},
	{"v mednarodnem transportu", "logistics"},
	{"referent za registracijo", "administration"},
	{"referent za registracije", "administration"},
	{"delavec v zbirnem centru", "logistics"},
	{"vodja gostinskega lokala", "hospitality"},
	{"vodja razdelilne kuhinje", "hospitality"},
	{"namestnik vodje trgovine", "retail"},
	{"vodja asfaltne operative", "skilled_trades"},
	{"pomočnik vodje gradbišča", "skilled_trades"},
	{"gradbeni pomožni delavec", "skilled_trades"},
	{"vodja kontrole kvalitete", "industrial_engineering"},
	{"vodja sektorja kakovosti", "industrial_engineering"},
	{"pomočnik v.d. ravnatelja", "education"},
	{"psihosocialni svetovalec", "social_services"},
	{"vodja servisne delavnice", "skilled_trades"},
	{"vodja kalanderske linije", "manufacturing"},
	{"vodja izmene v mešalnici", "manufacturing"},
	{"cnc programer - rezkalec", "manufacturing"},
	{"pomočnik vodje projektov", "project_management"},
	{"skrbnik hotelske okolice", "personal_services"},
	{"skrbnik procesnih naprav", "manufacturing"},
	{"tehnik zdravstvene nege", "healthcare"},
	{"zdravstveni hidrotehnik", "healthcare"},
	{"laboratorijski analitik", "science"},
	{"svetovalec v pravosodju", "legal"},
	{"direktor financ skupine", "finance"},
	{"svetovalec za poprodajo", "sales"},
	{"skrbnik ključnih kupcev", "sales"},
	{"skrbnik ključnih strank", "sales"},
	{"skrbnik prodajnih polic", "sales"},
	{"referent za najem vozil", "logistics"},
	{"referent za registracij", "administration"},
	{"vodja nadomestnih delov", "logistics"},
	{"upravitelj restavracije", "hospitality"},
	{"sodelavec v maloprodaji", "retail"},
	{"pomožni gradben delavec", "skilled_trades"},
	{"pomočnik v gradbeništvu", "skilled_trades"},
	{"višji ekspert validacij", "science"},
	{"vodja mehanske obdelave", "manufacturing"},
	{"administrativni delavec", "administration"},
	{"pomočnik vodje projekta", "project_management"},
	{"referent za rezervacije", "hospitality"},
	{"vodja zdravstvene nege", "healthcare"},
	{"konfigurator e-računov", "solutions_engineering"},
	{"poklicni delavec - cnc", "manufacturing"},
	{"inženir avtomatizacije", "industrial_engineering"},
	{"vodja ekonomske službe", "finance"},
	{"specialist za podjetja", "finance"},
	{"prodajni administrator", "sales"},
	{"marketplace specialist", "sales"},
	{"agent v klicnem centru", "support"},
	{"administrator podatkov", "administration"},
	{"prometni administrator", "logistics"},
	{"vodja igralnega salona", "hospitality"},
	{"pomočnik vodje strežbe", "hospitality"},
	{"delavec v avtopralnici", "personal_services"},
	{"delovodja na gradbišču", "skilled_trades"},
	{"delovodja gradbeništva", "skilled_trades"},
	{"scensko odrski mojster", "skilled_trades"},
	{"vodja proizvodne enote", "manufacturing"},
	{"vodja službe ekologije", "science"},
	{"vodja produktne linije", "product"},
	{"cnc programer/operater", "manufacturing"},
	{"referent za namestitve", "hospitality"},
	{"zdravstveni sodelavec", "healthcare"},
	{"referent obračun plač", "finance"},
	{"vodja finančne službe", "finance"},
	{"svetovalec za leasing", "finance"},
	{"specialist za prodajo", "sales"},
	{"predstavnik na terenu", "sales"},
	{"komercialni sodelavec", "sales"},
	{"referent za poprodajo", "sales"},
	{"sodelavec v poprodaji", "sales"},
	{"vodja ključnih kupcev", "sales"},
	{"zavarovalni zastopnik", "sales"},
	{"delo v klicnem centru", "support"},
	{"svetovalec za stranke", "support"},
	{"referent za garancije", "support"},
	{"logistični specialist", "logistics"},
	{"skrbnik voznega parka", "logistics"},
	{"referent za logistiko", "logistics"},
	{"vodja polnilcev plina", "logistics"},
	{"skladiščni manipulant", "logistics"},
	{"kontrola in pakiranje", "manufacturing"},
	{"group revenue manager", "hospitality"},
	{"pralec v avtopralnici", "personal_services"},
	{"spremljevalec skupine", "hospitality"},
	{"hidrotehnični delavec", "skilled_trades"},
	{"asistent z doktoratom", "education"},
	{"začasni spremljevalec", "social_services"},
	{"nadzornik na smučišču", "public_safety"},
	{"pristaniški nadzornik", "public_safety"},
	{"vodja cnc operaterjev", "manufacturing"},
	{"vodja rafiniranih olj", "manufacturing"},
	{"projektni koordinator", "project_management"},
	{"specialist za izvedbo", "project_management"},
	{"računovodski referent", "finance"},
	{"ai & automation lead", "ai_engineering"},
	{"sistemski it inženir", "software_engineering"},
	{"sodelavec za varnost", "occupational_safety"},
	{"svetnik v pravosodju", "legal"},
	{"sodelavec v obračunu", "finance"},
	{"koordinator za kadre", "hr"},
	{"prodajni strokovnjak", "sales"},
	{"sodelavec za prodajo", "sales"},
	{"terenski predstavnik", "sales"},
	{"komercialni asistent", "sales"},
	{"komercialni referent", "sales"},
	{"nepremičninski agent", "sales"},
	{"programski zastopnik", "sales"},
	{"grafični oblikovalec", "design"},
	{"sprejemnik na servis", "support"},
	{"mednarodni transport", "logistics"},
	{"referent registracij", "administration"},
	{"referent v skladišču", "logistics"},
	{"usmerjevalec pošiljk", "logistics"},
	{"organizator prehrane", "hospitality"},
	{"monter v proizvodnji", "manufacturing"},
	{"operater za kakovost", "manufacturing"},
	{"kontrolor ekspeditor", "logistics"},
	{"kontrolor v financah", "finance"},
	{"proizvodni kontrolor", "manufacturing"},
	{"znanstveni sodelavec", "science"},
	{"spremljevalec učenca", "education"},
	{"spremljevalec učencu", "education"},
	{"spremljevalec otroka", "education"},
	{"policijski inšpektor", "public_safety"},
	{"vodja servisne ekipe", "skilled_trades"},
	{"vodja izmene montaže", "manufacturing"},
	{"produktni specialist", "product"},
	{"asistent na projektu", "project_management"},
	{"spremljava projektov", "project_management"},
	{"vodja administracije", "administration"},
	{"vodja prodajne ekipe", "sales"},
	{"vodja nabavne službe", "operations"},
	{"pripravljalec orodij", "manufacturing"},
	{"medicinski biokemik", "healthcare"},
	{"farmacevtski tehnik", "healthcare"},
	{"inženir radiologije", "healthcare"},
	{"vodja pravne službe", "legal"},
	{"odvetniški kandidat", "legal"},
	{"finančni svetovalec", "finance"},
	{"obračunski referent", "finance"},
	{"referent za obračun", "finance"},
	{"vodja skupnih služb", "administration"},
	{"prodajni svetovalec", "sales"},
	{"prodajni specialist", "sales"},
	{"področni svetovalec", "sales"},
	{"referent za prodajo", "sales"},
	{"meta ads advertiser", "marketing"},
	{"spletni oglaševalec", "marketing"},
	{"servisni svetovalec", "support"},
	{"koordinator prevoza", "logistics"},
	{"fulfillment manager", "logistics"},
	{"vodja voznega parka", "logistics"},
	{"blagovni manipulant", "logistics"},
	{"transportni delavec", "logistics"},
	{"delavec v skladišču", "logistics"},
	{"vodja gospodinjstva", "personal_services"},
	{"vodja - šef kuhinje", "hospitality"},
	{"kuhinjska pomočnica", "hospitality"},
	{"pripravljalec hrane", "hospitality"},
	{"delovodja gradbišča", "skilled_trades"},
	{"kontrolor kakovosti", "manufacturing"},
	{"vodja knjigovodstva", "finance"},
	{"kandidat za gasilca", "public_safety"},
	{"projektni sodelavec", "project_management"},
	{"asistent direktorja", "administration"},
	{"asistent direktorju", "administration"},
	{"natakarski pomočnik", "hospitality"},
	{"zdravstveni tehnik", "healthcare"},
	{"peka zob in diskov", "healthcare"},
	{"radiološki inženir", "healthcare"},
	{"vodja it projektov", "project_management"},
	{"analitik - modelar", "data_analytics"},
	{"pravosodni svetnik", "legal"},
	{"sodelavec za škode", "finance"},
	{"finančni kontrolor", "finance"},
	{"prodajni zastopnik", "sales"},
	{"skrbnik komitentov", "sales"},
	{"business developer", "sales"},
	{"influencer manager", "marketing"},
	{"skladiščni delavec", "logistics"},
	{"partner za dostavo", "logistics"},
	{"sodelavec na pošti", "logistics"},
	{"vodja restavracije", "hospitality"},
	{"kuhinjski pomočnik", "hospitality"},
	{"pomočnik v kuhinji", "hospitality"},
	{"pomočnik v strežbi", "hospitality"},
	{"pomočnik natakarja", "hospitality"},
	{"gostinski pomočnik", "hospitality"},
	{"vzdrževalec perila", "personal_services"},
	{"delavec v pralnici", "personal_services"},
	{"operater betonarne", "skilled_trades"},
	{"gradbeni delovodja", "skilled_trades"},
	{"pomočnik vodje del", "skilled_trades"},
	{"rekviziter mojster", "skilled_trades"},
	{"garderober mojster", "skilled_trades"},
	{"operater kakovosti", "manufacturing"},
	{"vizualni kontrolor", "manufacturing"},
	{"kontrolor analitik", "science"},
	{"procesni kontrolor", "manufacturing"},
	{"bioprocesni tehnik", "manufacturing"},
	{"koordinator pomoči", "social_services"},
	{"obdelovalcev kovin", "manufacturing"},
	{"referent za nabavo", "operations"},
	{"varnostni inženir", "occupational_safety"},
	{"sanitarni inženir", "occupational_safety"},
	{"sanitarni inžinir", "occupational_safety"},
	{"prodajni tehnolog", "solutions_engineering"},
	{"pravni svetovalec", "legal"},
	{"davčni svetovalec", "finance"},
	{"obračunski tehnik", "finance"},
	{"finančni referent", "finance"},
	{"referent za kadre", "hr"},
	{"vodja kadrovskega", "hr"},
	{"prodajni referent", "sales"},
	{"prodajni asistent", "sales"},
	{"regionalni tržnik", "sales"},
	{"skrbnik/ca strank", "sales"},
	{"kreativni strateg", "marketing"},
	{"grafični operater", "manufacturing"},
	{"sprejemnik servis", "support"},
	{"servisni asistent", "support"},
	{"koordinator zalog", "logistics"},
	{"transport manager", "logistics"},
	{"skrbnik skladišča", "logistics"},
	{"vodja pretakališč", "logistics"},
	{"dvoriščni delavec", "logistics"},
	{"blagovni referent", "logistics"},
	{"prevzemalec blaga", "logistics"},
	{"delavec v odpremi", "logistics"},
	{"pomoč v skladišču", "logistics"},
	{"prevoznik denarja", "logistics"},
	{"vodja hotelirstva", "hospitality"},
	{"vodja maloprodaje", "retail"},
	{"kuhinjski pomočnk", "hospitality"},
	{"delavec v kuhinji", "hospitality"},
	{"gostinski delavec", "hospitality"},
	{"gradbeni pomočnik", "skilled_trades"},
	{"cestni preglednik", "skilled_trades"},
	{"komunalni delavec", "skilled_trades"},
	{"vodja kontrolinga", "finance"},
	{"osebna asistentka", "social_services"},
	{"mladinski delavec", "social_services"},
	{"socialnem varstvu", "social_services"},
	{"medobčinski redar", "public_safety"},
	{"kmetijski delavec", "skilled_trades"},
	{"pomočnik gozdarja", "skilled_trades"},
	{"delavec v prireji", "skilled_trades"},
	{"ladijski operater", "skilled_trades"},
	{"vodenje projektov", "project_management"},
	{"vodja informatike", "devops"},
	{"delovni terapevt", "healthcare"},
	{"sistemski tehnik", "support"},
	{"prodajni inženir", "solutions_engineering"},
	{"cnc posluževalec", "manufacturing"},
	{"borzni posrednik", "finance"},
	{"skrbnik koncesij", "sales"},
	{"poslovni partner", "sales"},
	{"vodja marketinga", "marketing"},
	{"tehnični urednik", "technical_writing"},
	{"grafični delavec", "manufacturing"},
	{"podpora strankam", "support"},
	{"delo v skladišču", "logistics"},
	{"rušenje objektov", "skilled_trades"},
	{"gradbeni delavec", "skilled_trades"},
	{"elektro montažer", "manufacturing"},
	{"sestava pohištva", "manufacturing"},
	{"kontrolor opreme", "manufacturing"},
	{"socialni delavec", "social_services"},
	{"socialno varstvo", "social_services"},
	{"socialno vartsvo", "social_services"},
	{"vodja energetike", "skilled_trades"},
	{"vodja investicij", "project_management"},
	{"hr administrator", "hr"},
	{"gradbeni mojster", "skilled_trades"},
	{"vodja reševalcev", "public_safety"},
	{"skrbnik sistemov", "devops"},
	{"tehnik zdr.nege", "healthcare"},
	{"nabavni inženir", "operations"},
	{"upravljalec cnc", "manufacturing"},
	{"v pravni službi", "legal"},
	{"prodajni trener", "sales"},
	{"vodja poprodaje", "sales"},
	{"trgovski potnik", "sales"},
	{"carrier manager", "logistics"},
	{"vodja skladišča", "logistics"},
	{"vodja recepcije", "hospitality"},
	{"vodja ala carda", "hospitality"},
	{"pomoč v kuhinji", "hospitality"},
	{"pomoč v strežbi", "hospitality"},
	{"kabinsko osebje", "hospitality"},
	{"vodja betonarne", "skilled_trades"},
	{"vodja gradbišča", "skilled_trades"},
	{"monterska ekipa", "skilled_trades"},
	{"monter izdelkov", "manufacturing"},
	{"vodja kakovosti", "industrial_engineering"},
	{"as&t specialist", "science"},
	{"kemijski tehnik", "science"},
	{"osebni asistent", "social_services"},
	{"romski pomočnik", "social_services"},
	{"šolski psiholog", "education"},
	{"vodja lakirnice", "manufacturing"},
	{"vodja delavnice", "skilled_trades"},
	{"vodja pakiranja", "manufacturing"},
	{"vodja pakirnice", "manufacturing"},
	{"vodja projektov", "project_management"},
	{"produktni vodja", "product"},
	{"projektni vodja", "project_management"},
	{"skrbnik objekta", "personal_services"},
	{"zobni asistent", "healthcare"},
	{"prodajni agent", "sales"},
	{"področni vodja", "sales"},
	{"skrbnik strank", "sales"},
	{"vodja trgovine", "retail"},
	{"delo v kuhinji", "hospitality"},
	{"delo v strežbi", "hospitality"},
	{"vodja gradbišč", "skilled_trades"},
	{"monter baterij", "manufacturing"},
	{"občinski redar", "public_safety"},
	{"gozdni delavec", "skilled_trades"},
	{"vodja projekta", "project_management"},
	{"vodja skladišč", "logistics"},
	{"tehnični risar", "engineering_design"},
	{"kalilca orodij", "manufacturing"},
	{"erp specialist", "solutions_engineering"},
	{"erp svetovalec", "solutions_engineering"},
	{"sap svetovalec", "solutions_engineering"},
	{"vodja lekarne", "healthcare"},
	{"vodja pravnih", "legal"},
	{"klicni center", "support"},
	{"vodja strežbe", "hospitality"},
	{"vodja kuhinje", "hospitality"},
	{"vodja trafike", "retail"},
	{"vodja gradnje", "skilled_trades"},
	{"vodja montaže", "skilled_trades"},
	{"monter orodij", "manufacturing"},
	{"vodja pisarne", "operations"},
	{"hr specialist", "hr"},
	{"utopne kovače", "manufacturing"},
	{"skrbnik zalog", "logistics"},
	{"operater cnc", "manufacturing"},
	{"vodja lokala", "hospitality"},
	{"pralec vozil", "personal_services"},
	{"vodja financ", "finance"},
	{"vodja linije", "manufacturing"},
	{"utopni kovač", "manufacturing"},
	{"vodja pošte", "logistics"},
	{"vodja farme", "skilled_trades"},
	{"hr direktor", "hr"},
	{"hr partner", "hr"},
	{"vodja del", "skilled_trades"},
	{"vodja hr", "hr"},
	{"nabavni referent", "operations"},
	{"nabavni specialist", "operations"},
	{"pobiralec odpadkov", "personal_services"},
	{"pogrebni delavec", "personal_services"},
	{"sodelavec na razrezu", "manufacturing"},
	{"delavec za razrez", "manufacturing"},
	{"izvajalec razreza", "manufacturing"},
	{"upravljalec avtodvigal", "skilled_trades"},
	{"upravljalec avtodvigala", "skilled_trades"},
	{"upravljalec dvigal", "skilled_trades"},
	{"upravljalec dvižnih naprav", "skilled_trades"},
	{"upravljalec žerjava", "skilled_trades"},
	{"upravljalec gradbenih strojev", "skilled_trades"},
	{"upravljalec kolesnega nakladalnika", "skilled_trades"},
	{"upravljalec gm", "skilled_trades"},
	{"upravljalec asfaltne baze", "skilled_trades"},
	{"upravljalec kotlovskih naprav", "skilled_trades"},
	{"upravljalec energetskih", "skilled_trades"},
	{"upravljalca energetskih", "skilled_trades"},
	{"upravljalca sistemov centralnega ogrevanja", "skilled_trades"},
	{"upravljalec spte", "skilled_trades"},
	{"upravljalec sistema", "skilled_trades"},
	{"elektroenergetskih postrojev", "skilled_trades"},
	{"av tehnik", "skilled_trades"},
	{"servisni tehnik", "skilled_trades"},
	{"izdelovalec stavbnega pohištva", "manufacturing"},
	{"izdelava vzmeti", "manufacturing"},
	{"izdelava šablon", "manufacturing"},
	{"obdelava kovin", "manufacturing"},
	{"procesni delavec", "manufacturing"},
	{"procesni operater", "manufacturing"},
	{"procesni tehnik", "manufacturing"},
	{"proizvodni delavec", "manufacturing"},
	{"proizvodni delavci", "manufacturing"},
	{"proizvodni tehnik", "manufacturing"},
	{"proizvodni operater", "manufacturing"},
	{"proizvodna dela", "manufacturing"},
	{"živilski delavec", "manufacturing"},
	{"kartonažni delavec", "manufacturing"},
	{"kemijski delavec", "manufacturing"},
	{"ročna dela", "manufacturing"},
	{"ročni dodelovalec", "manufacturing"},
	{"lažja dela", "manufacturing"},
	{"lažje fizično delo", "manufacturing"},
	{"enostavna dela", "manufacturing"},
	{"zahtevna dela", "manufacturing"},
	{"tehnološka dela", "manufacturing"},
	{"preprosta dela", "manufacturing"},
	{"pomožna dela", "manufacturing"},
	{"pomožni delavec", "manufacturing"},
	{"pomočnik cevarja", "skilled_trades"},
	{"pomočnik elektroinštalaterja", "skilled_trades"},
	{"pomočnik izolaterja", "skilled_trades"},
	{"pomočnik krovca", "skilled_trades"},
	{"pomočnik monterja", "skilled_trades"},
	{"pomočnik tesarja", "skilled_trades"},
	{"pomočnik mizarja", "skilled_trades"},
	{"pomočnik vzdrževalca", "skilled_trades"},
	{"pomočnik strojnika", "skilled_trades"},
	{"pomočnik v elektro delavnici", "skilled_trades"},
	{"pomočnik v mizarski delavnici", "skilled_trades"},
	{"pomočnik v mesnici", "retail"},
	{"pomočnik na lepilni liniji", "manufacturing"},
	{"pomočnik rezalca", "manufacturing"},
	{"pomočnik operaterja", "manufacturing"},
	{"pomočnik v pisarni in delavnici", "administration"},
	{"delavec na bencinski črpalki", "retail"},
	{"delavec v trgovini", "retail"},
	{"delavec v sirarni", "manufacturing"},
	{"delavec v vzdrževanju", "skilled_trades"},
	{"delavec v delavnici", "skilled_trades"},
	{"delavec v mizarski delavnici", "skilled_trades"},
	{"delavec v tesarski delavnici", "skilled_trades"},
	{"delavec v tapetništvu", "manufacturing"},
	{"delavec v reciklaži", "manufacturing"},
	{"delavec v predelavi odpadkov", "manufacturing"},
	{"delavec za ravnanje z odpadki", "personal_services"},
	{"delavec ali delavka za mikroskopom", "manufacturing"},
	{"delavec finomehanik", "manufacturing"},
	{"delavec - montažer", "manufacturing"},
	{"delavec na montaži", "manufacturing"},
	{"delavec na pripravi vložka", "manufacturing"},
	{"delavec na izdelavi sendvičev", "manufacturing"},
	{"delavec na ekstruziji", "manufacturing"},
	{"delavec na konfekciji", "manufacturing"},
	{"delavec na stroju", "manufacturing"},
	{"delavec na žagarskem obratu", "manufacturing"},
	{"delavec v adjustaži", "manufacturing"},
	{"delavec v kovinarski industriji", "manufacturing"},
	{"delavec v kovinski industriji", "manufacturing"},
	{"delavec v lesni", "manufacturing"},
	{"delavec v lakirnici", "manufacturing"},
	{"delavec v pakirnici", "manufacturing"},
	{"delavec v prašni lakirnici", "manufacturing"},
	{"delavec v tovarni zdravil", "manufacturing"},
	{"delavec v živilski", "manufacturing"},
	{"delavec v dvoizmenski", "manufacturing"},
	{"delavec začetnik", "manufacturing"},
	{"delavec za lažja", "manufacturing"},
	{"delavec za preprosta", "manufacturing"},
	{"delavec za pomožna", "manufacturing"},
	{"delo s stroji", "manufacturing"},
	{"mojster obrata", "manufacturing"},
	{"tehnični operater", "manufacturing"},
	{"tehnični upravljalec linije", "manufacturing"},
	{"tehnik v mešalnici", "manufacturing"},
	{"tehnik zaključnih operacij", "manufacturing"},
	{"tehnik za sestavo in vzdrževanje orodij", "manufacturing"},
	{"tehnik za priklop storitev operaterja", "skilled_trades"},
	{"tehnik elektro sistemov", "skilled_trades"},
	{"tehnik energetike", "skilled_trades"},
	{"tehnik infrastrukture", "skilled_trades"},
	{"tehnik inženiringa lokacije", "skilled_trades"},
	{"tehnik specialist", "skilled_trades"},
	{"tehnik v vzdrževanju", "skilled_trades"},
	{"tehnik kontrolor", "manufacturing"},
	{"tehnik v farmacevtskem podjetju", "manufacturing"},
	{"tehnik v kongresnem", "hospitality"},
	{"tehnični delavec", "skilled_trades"},
	{"tehnični sodelavec", "skilled_trades"},
	{"izvajalec ddd", "personal_services"},
	{"telesni stražar", "public_safety"},
	{"asistent za proizvodnjo", "manufacturing"},
	{"developer php", "software_engineering"},
	{"medicinska sestrav", "healthcare"},
	{"uvajalec informacijskih tehnologij", "solutions_engineering"},
	{"pomoč v vzdrževanju", "skilled_trades"},
	{"procesni sistematizer", "industrial_engineering"},
	{"skupinski habilitator", "social_services"},
	{"poslovna asistentka", "administration"},
	{"poslovni asistent", "administration"},
	{"poslovni administrator", "administration"},
	{"poslovni analitik", "business_analysis"},
	{"tržni analitik", "business_analysis"},
	{"podatkovni analitik", "data_analytics"},
	{"sodelavec v glavni pisarni", "administration"},
	{"sodelavec za administrativno podporo", "administration"},
	{"koordinator administracije", "administration"},
	{"koordinator operacijskih dvoran", "healthcare"},
	{"koordinator ambulante", "healthcare"},
	{"koordinator sistema ravnanja z okoljem", "science"},
	{"koordinator naklada", "manufacturing"},
	{"operativni gradbeni inženir", "industrial_engineering"},
	{"specialist energetike", "skilled_trades"},
	{"specialist za inženiring in avtomatizacijo", "industrial_engineering"},
	{"specialist za podporo", "support"},
	{"ekspert tehnologije izdelkov", "industrial_engineering"},
	{"strokovnjak za robotiko itot", "industrial_engineering"},
	{"strokovnjak za tehnično dokumentacijo", "technical_writing"},
	{"strokovni sodelavec za meritve", "industrial_engineering"},
	{"strokovni sodelavec za vzdrževanje", "skilled_trades"},
	{"strokovni sodelavec za spremljanje", "finance"},
	{"sodelavec razvoja", "industrial_engineering"},
	{"sodelavec v naročanju", "operations"},
	{"sodelavec v poslovalnici", "retail"},
	{"svetovalec za mobilno telefonijo", "sales"},
	{"svetovalec za prodajo", "sales"},
	{"svetovalni delavec", "social_services"},
	{"svetovalec v trgovini", "retail"},
	{"višji kreditni", "finance"},
	// The test, network and database engineers and administrators, ahead of the bare
	// "інженер" and "адміністратор" below.
	{"тест інженер", "qa"},
	{"тест-інженер", "qa"},
	{"мережевий інженер", "network_engineering"},
	{"адміністратор баз даних", "devops"},
	{"адміністратор системи", "devops"},
	{"тестувальник", "qa"},
	{"tesztmérnök", "qa"},
	{"тестувальниця", "qa"},
	{"testowania oprogramowania", "qa"},
	{"inżynier danych", "data_engineering"},
	{"analityk biznesowy", "business_analysis"},
	{"analityczka biznesowa", "business_analysis"},
	{"analityk systemowy", "business_analysis"},
	{"analityczka systemowa", "business_analysis"},
	{"analityk biznesowo-systemowy", "business_analysis"},
	{"rendszerszervező", "business_analysis"},
	{"cyberbezpieczeństwa", "security"},
	{"бізнес-аналітик", "business_analysis"},
	{"бизнес анализатор", "business_analysis"},
	{"системний адміністратор", "devops"},
	{"administrator systemów", "devops"},
	{"sistemski administrator", "devops"},
	// The IT generalist: the same seat "Fachinformatiker" names in German.
	{"informatyk", "devops"},
	{"informatyczka", "devops"},
	{"informatičar", "devops"},
	{"informatičarka", "devops"},
	{"informatik", "devops"},
	{"informatikus", "devops"},
	{"it підтримки", "support"},
	{"технічної підтримки", "support"},
	{"обслуговування клієнтів", "support"},
	{"obsługi klienta", "support"},
	{"obsługa klienta", "support"},
	{"obsługi klientów", "support"},
	{"korisničkoj podršci", "support"},
	{"računalni tehničar", "support"},
	{"informatički tehničar", "support"},
	{"korisnička podrška", "support"},
	{"kontaktnem centru", "support"},
	{"контактен център", "support"},
	{"ügyfélszolgálati", "support"},
	{"обслужване на клиенти", "support"},
	// The Czech and Slovak public occupation register (CZ-ISCO, SK-ISCO) titles a
	// posting with its plural group name, sometimes twice over ("Vývojáři softwaru,
	// Vývojáři softwaru"). The groups are the ICT ones; each resolves as the English
	// occupation it names. Database and network specialists are declared before the
	// bare network phrase that "Specialisté v oblasti databází a počítačových sítí"
	// also contains, and the administrators before it too.
	{"vývojáři softwaru", "software_engineering"},
	{"vývojári softvéru", "software_engineering"},
	{"vývojáři a analytici softwaru", "software_engineering"},
	{"vývojári a analytici softvéru", "software_engineering"},
	{"vývojáři webu", "software_engineering"},
	{"vývojári webových", "software_engineering"},
	{"weboví vývojári", "software_engineering"},
	{"programátoři aplikací", "software_engineering"},
	{"aplikační programátoři", "software_engineering"},
	{"programátori aplikácií", "software_engineering"},
	{"aplikační programátor", "software_engineering"},
	{"systémoví analytici", "business_analysis"},
	{"systémový analytik", "business_analysis"},
	{"systémová analytička", "business_analysis"},
	{"testování softwaru", "qa"},
	{"testovania softvéru", "qa"},
	{"návrháři a správci databází", "devops"},
	{"dizajnéri a správcovia databáz", "devops"},
	{"systémoví administrátoři", "devops"},
	{"správcovia systémov", "devops"},
	{"systémový inženýr", "software_engineering"},
	{"počítačových sítí", "network_engineering"},
	{"počítačových sietí", "network_engineering"},
	{"technici uživatelské podpory", "support"},
	{"technici užívateľskej podpory", "support"},

	// Healthcare.
	{"lekarz", "healthcare"},
	{"lekarka", "healthcare"},
	{"pielęgniarz", "healthcare"},
	{"pielęgniarka", "healthcare"},
	{"położna", "healthcare"},
	{"ratownik medyczny", "healthcare"},
	{"ratowniczka medyczna", "healthcare"},
	{"fizjoterapeuta", "healthcare"},
	{"fizjoterapeutka", "healthcare"},
	{"farmaceuta", "healthcare"},
	{"farmaceutka", "healthcare"},
	{"stomatolog", "healthcare"},
	{"stomatolożka", "healthcare"},
	{"dentysta", "healthcare"},
	{"dentystka", "healthcare"},
	{"ortodonta", "healthcare"},
	{"ortodontka", "healthcare"},
	{"lékař", "healthcare"},
	{"lékařka", "healthcare"},
	{"všeobecná sestra", "healthcare"},
	{"zdravotní sestra", "healthcare"},
	{"lekár", "healthcare"},
	{"lekárka", "healthcare"},
	{"zdravotná sestra", "healthcare"},
	{"zdravnik", "healthcare"},
	{"zdravnica", "healthcare"},
	{"liječnik", "healthcare"},
	{"liječnica", "healthcare"},
	{"lekar", "healthcare"},
	{"medicinska sestra", "healthcare"},
	// Bare, a Czech, Slovak or Croatian title's "sestra" is the nurse ("Odběrová sestra").
	{"sestra", "healthcare"},
	{"dentalni asistent", "healthcare"},
	{"dentalna asistentica", "healthcare"},
	{"obsługi pacjenta", "healthcare"},
	{"medicinski tehničar", "healthcare"},
	{"farmacevt", "healthcare"},
	{"farmacevtka", "healthcare"},
	{"ljekarnik", "healthcare"},
	{"ljekarnica", "healthcare"},
	{"orvos", "healthcare"},
	{"ápoló", "healthcare"},
	{"gyógyszerész", "healthcare"},
	{"szakorvos", "healthcare"},
	{"klinikai kutatási munkatárs", "healthcare"},
	{"лікар", "healthcare"},
	{"лікарка", "healthcare"},
	{"фармацевт", "healthcare"},
	{"провізор", "healthcare"},
	// The genitive: the doctor's assistant ("Помощник врача"), Russian and Ukrainian.
	{"врача", "healthcare"},
	{"лекар", "healthcare"},
	{"медицинска сестра", "healthcare"},

	// Public safety. A Slovenian water rescuer is the lifeguard; the bare "reševalec"
	// is the ambulance crew, filed with the paramedic.
	{"reševalec iz vode", "public_safety"},
	{"reševalka iz vode", "public_safety"},
	{"reševalec", "healthcare"},
	{"pracownik ochrony", "public_safety"},
	{"pracowniczka ochrony", "public_safety"},
	{"agent ochrony", "public_safety"},
	{"varnostnik", "public_safety"},
	{"varnostnica", "public_safety"},
	{"zaštitar", "public_safety"},
	{"vagyonőr", "public_safety"},
	{"охоронець", "public_safety"},
	{"охоронник", "public_safety"},
	{"vatrogasac", "public_safety"},
	{"охранител", "public_safety"},

	// Retail.
	// A bank's cashier is the teller, filed with finance as "teller" is.
	{"касир відділення", "finance"},
	{"sprzedawca", "retail"},
	{"sprzedawczyni", "retail"},
	{"kasjer", "retail"},
	{"kasjerka", "retail"},
	{"prodavač", "retail"},
	{"prodavačka", "retail"},
	{"prodavačica", "retail"},
	{"prodavac", "retail"},
	{"pokladní", "retail"},
	{"predavač", "retail"},
	{"predavačka", "retail"},
	{"pokladník", "retail"},
	{"pokladníčka", "retail"},
	{"prodajalec", "retail"},
	{"prodajalka", "retail"},
	{"blagajnik", "retail"},
	{"blagajnica", "retail"},
	{"blagajničarka", "retail"},
	{"kasir", "retail"},
	{"kasirka", "retail"},
	{"eladó", "retail"},
	{"pénztáros", "retail"},
	{"продавець", "retail"},
	{"продавчиня", "retail"},
	{"касир", "retail"},
	{"касирка", "retail"},
	{"продавач", "retail"},
	{"касиер", "retail"},
	{"pracownik sklepu", "retail"},
	{"pracowniczka sklepu", "retail"},
	{"kierownik sklepu", "retail"},
	{"kierowniczka sklepu", "retail"},
	{"kierownika sklepu", "retail"},
	{"árufeltöltő", "retail"},
	{"vedoucí prodejny", "retail"},
	{"vedúci predajne", "retail"},
	{"prodejny", "retail"},
	{"prodejně", "retail"},
	{"predajne", "retail"},
	{"predajni", "retail"},
	{"predajňu", "retail"},
	{"магазину", "retail"},

	// Logistics.
	{"magazynier", "logistics"},
	{"magazynierka", "logistics"},
	{"pracownik magazynu", "logistics"},
	{"pracowniczka magazynu", "logistics"},
	{"operator wózka widłowego", "logistics"},
	{"operatorka wózka widłowego", "logistics"},
	{"wózka widłowego", "logistics"},
	{"wózków widłowych", "logistics"},
	{"kierowca", "logistics"},
	{"kurier", "logistics"},
	{"spedytor", "logistics"},
	{"spedytorka", "logistics"},
	{"dyspozytor", "logistics"},
	{"dyspozytorka", "logistics"},
	{"skladník", "logistics"},
	{"skladnice", "logistics"},
	{"skladníčka", "logistics"},
	{"řidič", "logistics"},
	{"řidička", "logistics"},
	{"vodič", "logistics"},
	{"skladištar", "logistics"},
	{"skladištarka", "logistics"},
	{"magacioner", "logistics"},
	{"vozač", "logistics"},
	{"vozačica", "logistics"},
	{"dostavljač", "logistics"},
	{"dostavljačica", "logistics"},
	{"skladiščnik", "logistics"},
	{"skladiščnica", "logistics"},
	{"viličarist", "logistics"},
	{"voznik", "logistics"},
	{"voznica", "logistics"},
	{"raktáros", "logistics"},
	{"anyagmozgató", "logistics"},
	{"targoncavezető", "logistics"},
	{"targoncás", "logistics"},
	{"gépkocsivezető", "logistics"},
	{"sofőr", "logistics"},
	{"futár", "logistics"},
	{"комірник", "logistics"},
	{"комірниця", "logistics"},
	{"водій", "logistics"},
	{"вантажник", "logistics"},
	{"комплектувальник", "logistics"},
	{"шофьор", "logistics"},
	{"rakodó", "logistics"},
	{"raktárkezelő", "logistics"},
	{"склададжия", "logistics"},
	{"складов работник", "logistics"},
	{"мотокарист", "logistics"},
	{"електрокарист", "logistics"},

	// Hospitality. "Asistent kuharja" is the cook's assistant, in the genitive.
	{"kucharz", "hospitality"},
	{"kucharka", "hospitality"},
	{"pomoc kuchenna", "hospitality"},
	{"kelner", "hospitality"},
	{"kelnerka", "hospitality"},
	{"kuchař", "hospitality"},
	{"kuchařka", "hospitality"},
	{"číšník", "hospitality"},
	{"servírka", "hospitality"},
	{"kuchár", "hospitality"},
	{"kuchárka", "hospitality"},
	{"čašník", "hospitality"},
	{"čašníčka", "hospitality"},
	{"kuhar", "hospitality"},
	{"kuharica", "hospitality"},
	{"kuharja", "hospitality"},
	{"kuharski pomočnik", "hospitality"},
	{"kuharska pomočnica", "hospitality"},
	{"kuvar", "hospitality"},
	{"kuvarica", "hospitality"},
	{"konobar", "hospitality"},
	{"konobarica", "hospitality"},
	{"barmen", "hospitality"},
	{"natakar", "hospitality"},
	{"natakarica", "hospitality"},
	{"szakács", "hospitality"},
	{"pincér", "hospitality"},
	{"кухар", "hospitality"},
	{"кухарка", "hospitality"},
	{"офіціант", "hospitality"},
	{"офіціантка", "hospitality"},
	{"готвач", "hospitality"},
	{"сервитьор", "hospitality"},
	{"szobalány", "hospitality"},

	// The production floor. "Farmacevtski delavec" works a drug-production line.
	{"pracownik produkcji", "manufacturing"},
	{"pracowniczka produkcji", "manufacturing"},
	{"operator produkcji", "manufacturing"},
	{"operatorka produkcji", "manufacturing"},
	{"operator maszyn", "manufacturing"},
	{"operatorka maszyn", "manufacturing"},
	{"operator cnc", "manufacturing"},
	{"operatorka cnc", "manufacturing"},
	{"operator linii", "manufacturing"},
	{"kontroler jakości", "manufacturing"},
	{"kontrolerka jakości", "manufacturing"},
	{"operátor výroby", "manufacturing"},
	{"operátorka výroby", "manufacturing"},
	{"seřizovač", "manufacturing"},
	{"seřizovačka", "manufacturing"},
	{"cnc operater", "manufacturing"},
	{"cnc operaterka", "manufacturing"},
	{"cnc operatorica", "manufacturing"},
	{"cnc operátor", "manufacturing"},
	{"tokar", "manufacturing"},
	{"tokarica", "manufacturing"},
	{"farmacevtski delavec", "manufacturing"},
	{"farmacevtska delavka", "manufacturing"},
	{"procesničar", "manufacturing"},
	{"procesničarka", "manufacturing"},
	{"upravljalec strojev", "manufacturing"},
	{"upravljalka strojev", "manufacturing"},
	{"gépkezelő", "manufacturing"},
	{"gépbeállító", "manufacturing"},
	{"összeszerelő", "manufacturing"},
	{"forgácsoló", "manufacturing"},
	{"minőségellenőr", "manufacturing"},
	{"машинен оператор", "manufacturing"},
	{"производствена линия", "manufacturing"},
	{"ustawiacz", "manufacturing"},
	{"ustawiaczka", "manufacturing"},
	{"obsluha strojů", "manufacturing"},
	{"glodač", "manufacturing"},
	{"glodačica", "manufacturing"},

	// The trades.
	{"elektryk", "skilled_trades"},
	{"elektromonter", "skilled_trades"},
	{"elektromechanik", "skilled_trades"},
	{"mechanik", "skilled_trades"},
	{"mechaniczka", "skilled_trades"},
	{"ślusarz", "skilled_trades"},
	{"spawacz", "skilled_trades"},
	{"konserwator", "skilled_trades"},
	{"konserwatorka", "skilled_trades"},
	{"serwisant", "skilled_trades"},
	{"serwisantka", "skilled_trades"},
	{"hydraulik", "skilled_trades"},
	{"utrzymania ruchu", "skilled_trades"},
	{"kierownik budowy", "skilled_trades"},
	{"kierowniczka budowy", "skilled_trades"},
	{"kierownik robót", "skilled_trades"},
	{"kierowniczka robót", "skilled_trades"},
	{"elektrikář", "skilled_trades"},
	{"údržbář", "skilled_trades"},
	{"svářeč", "skilled_trades"},
	{"elektrikár", "skilled_trades"},
	{"údržbár", "skilled_trades"},
	{"zvárač", "skilled_trades"},
	{"električar", "skilled_trades"},
	{"zavarivač", "skilled_trades"},
	{"mehaničar", "skilled_trades"},
	{"serviser", "skilled_trades"},
	{"serviserka", "skilled_trades"},
	{"vzdrževalec", "skilled_trades"},
	{"vzdrževalka", "skilled_trades"},
	{"tehnik vzdrževanja", "skilled_trades"},
	{"tehničar održavanja", "skilled_trades"},
	{"karbantartó", "skilled_trades"},
	{"villanyszerelő", "skilled_trades"},
	{"hegesztő", "skilled_trades"},
	{"lakatos", "skilled_trades"},
	{"слюсар", "skilled_trades"},
	{"електрик", "skilled_trades"},
	{"зварник", "skilled_trades"},
	{"механік", "skilled_trades"},
	{"автомеханік", "skilled_trades"},
	{"автослюсар", "skilled_trades"},
	{"автомеханик", "skilled_trades"},
	{"автослесарь", "skilled_trades"},
	{"кровельщик", "skilled_trades"},
	{"плиточник", "skilled_trades"},
	{"облицовщик", "skilled_trades"},
	{"монтьор", "skilled_trades"},
	{"servisní technik", "skilled_trades"},
	{"servisný technik", "skilled_trades"},
	{"szerviztechnikus", "skilled_trades"},
	{"építésvezető", "skilled_trades"},

	// The plant's engineers, by the discipline the title names. There is NO bare
	// engineer noun, as there is none in English: the word beside it decides, and a bare
	// "inženir"/"inżynier"/"inženjer" filed the software, security, sales, radiology and
	// test engineers with the plant (freehire#3518). The process technologist is filed
	// as the Russian "технолог" is.
	{"inżynier procesu", "industrial_engineering"},
	{"inżynier procesów", "industrial_engineering"},
	{"inżynier jakości", "industrial_engineering"},
	{"inżynier ds. jakości", "industrial_engineering"},
	{"inżynierka ds. jakości", "industrial_engineering"},
	{"inżynier produkcji", "industrial_engineering"},
	{"inżynier utrzymania ruchu", "industrial_engineering"},
	{"inżynier mechanik", "industrial_engineering"},
	{"inżynier automatyk", "industrial_engineering"},
	{"inżynier elektryk", "industrial_engineering"},
	{"inżynier budowy", "skilled_trades"},
	{"inżynierka budowy", "skilled_trades"},
	{"inżynier konstruktor", "engineering_design"},
	{"inżynier projektant", "engineering_design"},
	{"inženir projektant", "engineering_design"},
	{"odgovorni projektant", "engineering_design"},
	{"elektro projektant", "engineering_design"},
	{"procesní inženýr", "industrial_engineering"},
	{"inženýr kvality", "industrial_engineering"},
	{"procesný inžinier", "industrial_engineering"},
	{"inžinier kvality", "industrial_engineering"},
	{"procesni inženir", "industrial_engineering"},
	{"strojni inženir", "industrial_engineering"},
	{"elektro inženir", "industrial_engineering"},
	{"elektroinženir", "industrial_engineering"},
	{"inženir elektrotehnike", "industrial_engineering"},
	{"inženir kakovosti", "industrial_engineering"},
	{"servisni inženir", "industrial_engineering"},
	{"procesni inženjer", "industrial_engineering"},
	{"građevinski inženjer", "industrial_engineering"},
	{"inženjer građevine", "industrial_engineering"},
	{"inženjer elektrotehnike", "industrial_engineering"},
	{"inženjerka elektrotehnike", "industrial_engineering"},
	{"inženjer strojarstva", "industrial_engineering"},
	{"inženjer održavanja", "industrial_engineering"},
	{"inženjer prodaje", "solutions_engineering"},
	{"prodajni inženjer", "solutions_engineering"},
	{"inżynier sprzedaży", "solutions_engineering"},
	{"inżynierka sprzedaży", "solutions_engineering"},
	{"inżynier ds. sprzedaży", "solutions_engineering"},
	{"inżynierka ds. sprzedaży", "solutions_engineering"},
	{"inženjer gradilišta", "skilled_trades"},
	{"інженер-енергетик", "industrial_engineering"},
	{"інженер з якості", "industrial_engineering"},
	{"охорони праці", "occupational_safety"},
	{"пожежної безпеки", "occupational_safety"},
	{"technolog", "industrial_engineering"},
	{"technolożka", "industrial_engineering"},
	{"tehnolog", "industrial_engineering"},
	{"tehnologinja", "industrial_engineering"},
	{"automatyk", "industrial_engineering"},
	{"villamosmérnök", "industrial_engineering"},
	{"folyamatmérnök", "industrial_engineering"},
	{"gépészmérnök", "industrial_engineering"},
	{"strojárski technici", "industrial_engineering"},
	// The design engineer, filed with draughting as "Design Engineer" is.
	{"konstruktér", "engineering_design"},
	{"konštruktér", "engineering_design"},
	// The laboratory. A Polish "technik analityk" is the laboratory analyst.
	{"technik analityk", "science"},
	{"techniczka analityk", "science"},
	{"laboratorijski tehnik", "science"},
	{"laboratorijska tehnica", "science"},
	{"laboratoriju", "science"},
	{"laboratóriumi", "science"},
	// Health and safety at work (BHP) and the environment, filed with HSE.
	{"bhp", "occupational_safety"},
	{"ochrony środowiska", "occupational_safety"},
	// Insurance sales, as "insurance agent" is.
	{"zavarovalni agent", "sales"},
	{"zavarovalna agentka", "sales"},
	// Personal services.
	{"trener personalny", "personal_services"},
	{"trenerka personalna", "personal_services"},
	{"takarító", "personal_services"},
	{"higijeničar", "personal_services"},
	{"higijeničarka", "personal_services"},
	{"садовник", "personal_services"},

	// Finance, HR, administration, education.
	{"księgowy", "finance"},
	{"księgowa", "finance"},
	{"księgowej", "finance"},
	{"księgowego", "finance"},
	{"księgowości", "finance"},
	{"účetní", "finance"},
	{"účtovník", "finance"},
	{"účtovníčka", "finance"},
	{"računovodja", "finance"},
	{"računovodkinja", "finance"},
	{"računovođa", "finance"},
	{"knjigovođa", "finance"},
	{"računovodstvu", "finance"},
	{"računovodstva", "finance"},
	{"kontrolingu", "finance"},
	{"controllingu", "finance"},
	{"könyvelő", "finance"},
	{"számviteli", "finance"},
	{"pénzügyi", "finance"},
	{"kontroller", "finance"},
	{"bérszámfejtő", "finance"},
	{"бухгалтерії", "finance"},
	{"счетоводител", "finance"},
	{"кредитиране", "finance"},
	{"kadr", "hr"},
	{"personalnych", "hr"},
	{"działu hr", "hr"},
	{"zasobów ludzkich", "hr"},
	{"personalista", "hr"},
	{"personalistka", "hr"},
	{"ljudskih potencialov", "hr"},
	{"ljudskih potencijala", "hr"},
	{"ljudskih resursa", "hr"},
	{"munkaügyi", "hr"},
	{"з персоналу", "hr"},
	{"човешки ресурси", "hr"},
	// Not the bare genitive: "Zgłoszenia do przyszłych rekrutacji" is a talent pool.
	{"ds. rekrutacji", "recruiting"},
	{"ds rekrutacji", "recruiting"},
	{"dziale rekrutacji", "recruiting"},
	{"zespole rekrutacji", "recruiting"},
	{"zespołu rekrutacji", "recruiting"},
	{"toborzási", "recruiting"},
	{"ds. administracji", "administration"},
	{"ds administracji", "administration"},
	{"recepcjonista", "administration"},
	{"recepcjonistka", "administration"},
	{"recepciós", "administration"},
	{"адміністратор", "administration"},
	{"nauczyciel", "education"},
	{"nauczycielka", "education"},
	{"učitel", "education"},
	{"učitelka", "education"},
	{"učitelj", "education"},
	{"učiteljica", "education"},
	{"nastavnik", "education"},
	{"nastavnica", "education"},
	{"vzgojitelj", "education"},
	{"vzgojiteljica", "education"},
	{"odgojitelj", "education"},
	{"odgojiteljica", "education"},
	{"tanár", "education"},
	{"tanító", "education"},
	{"korepetytor", "education"},
	{"korepetytorka", "education"},
	{"instruktor fitness", "education"},
	{"instruktorka fitness", "education"},
	{"jogász", "legal"},
	{"ügyvéd", "legal"},
	{"вчитель", "education"},
	{"вчителька", "education"},
	{"викладач", "education"},
	{"вихователь", "education"},
	{"учител", "education"},

	// The single Slovenian role nouns of the same contribution.
	{"komercialista", "sales"},
	{"komercialisti", "sales"},
	{"komercialist", "sales"},
	{"raziskovalec", "science"},
	{"tekstopisec", "marketing"},
	{"predavatelj", "education"},
	{"vzgojitelja", "education"},
	{"bibliotekar", "education"},
	{"oskrbovalec", "social_services"},
	{"oskrbovalka", "social_services"},
	{"varnnostnik", "public_safety"},
	{"vinogradnik", "skilled_trades"},
	{"konstrukter", "engineering_design"},
	{"konstruktor", "engineering_design"},
	{"viličarista", "logistics"},
	{"komisionar", "logistics"},
	{"meteorolog", "science"},
	{"konstruker", "engineering_design"},
	{"disponent", "logistics"},
	{"ravnatelj", "education"},
	{"psihiater", "healthcare"},
	{"veterinar", "healthcare"},
	{"cvetličar", "retail"},
	{"kalkulant", "industrial_engineering"},
	{"promotor", "marketing"},
	{"plakater", "marketing"},
	{"čiščenje", "personal_services"},
	{"čistilec", "personal_services"},
	{"čistilka", "personal_services"},
	{"čistelec", "personal_services"},
	{"oskrbnik", "personal_services"},
	{"psiholog", "healthcare"},
	{"policist", "public_safety"},
	{"kmetijec", "skilled_trades"},
	{"pedagog", "education"},
	{"gasilec", "public_safety"},
	{"tolmač", "technical_writing"},
	{"portir", "personal_services"},
	{"vratar", "personal_services"},
	{"sadjar", "skilled_trades"},
	{"gozdar", "skilled_trades"},
	{"mornar", "skilled_trades"},
	{"vrtnar", "skilled_trades"},
	{"šofer", "logistics"},
	{"kurir", "logistics"},
	{"dostavljalec", "logistics"},
	{"dostavnik", "logistics"},
	{"pismonoša", "logistics"},
	{"špediter", "logistics"},
	{"logist", "logistics"},
	{"zakladalec", "logistics"},
	{"nakladalec", "logistics"},
	{"sortirec", "logistics"},
	{"odpremnik", "logistics"},
	{"transporter", "logistics"},
	{"skladišnik", "logistics"},
	{"nabavnik", "operations"},
	{"deklarant", "logistics"},
	{"blagajničar", "retail"},
	{"prodajalca", "retail"},
	{"prodajalce", "retail"},
	{"prodajalci", "retail"},
	{"prodajale", "retail"},
	{"trgovec", "retail"},
	{"trgovka", "retail"},
	{"aranžer", "retail"},
	{"delilec", "hospitality"},
	{"servirka", "hospitality"},
	{"servir", "hospitality"},
	{"strežnica", "hospitality"},
	{"strežnik", "hospitality"},
	{"strežba", "hospitality"},
	{"šankist", "hospitality"},
	{"barist", "hospitality"},
	{"kuharji", "hospitality"},
	{"kulinarik", "hospitality"},
	{"picopek", "hospitality"},
	{"slaščičar", "hospitality"},
	{"pomivalec", "hospitality"},
	{"sobarica", "hospitality"},
	{"sobar", "hospitality"},
	{"gospodinja", "personal_services"},
	{"gospodinjec", "personal_services"},
	{"hišnik", "personal_services"},
	{"snažilec", "personal_services"},
	{"smetar", "personal_services"},
	{"razkuževalec", "personal_services"},
	{"frizer", "personal_services"},
	{"kozmetičarka", "personal_services"},
	{"maser", "personal_services"},
	{"maserka", "personal_services"},
	{"receptor", "administration"},
	{"kineziolog", "healthcare"},
	{"dietetik", "healthcare"},
	{"fizioterapevt", "healthcare"},
	{"fiuiterapevt", "healthcare"},
	{"logoped", "healthcare"},
	{"bolničar", "healthcare"},
	{"zobozdravnik", "healthcare"},
	{"zobotehnik", "healthcare"},
	{"zdravnika", "healthcare"},
	{"babica", "healthcare"},
	{"protikorozist", "skilled_trades"},
	{"sestavljalec", "manufacturing"},
	{"sestavljavec", "manufacturing"},
	{"mizar", "skilled_trades"},
	{"tesar", "skilled_trades"},
	{"zidar", "skilled_trades"},
	{"pleskar", "skilled_trades"},
	{"slikopleskar", "skilled_trades"},
	{"soboslikar", "skilled_trades"},
	{"fasader", "skilled_trades"},
	{"krovec", "skilled_trades"},
	{"klepar", "skilled_trades"},
	{"izolater", "skilled_trades"},
	{"keramičar", "skilled_trades"},
	{"polagalec", "skilled_trades"},
	{"parketar", "skilled_trades"},
	{"steklar", "skilled_trades"},
	{"železokrivec", "skilled_trades"},
	{"opažer", "skilled_trades"},
	{"asfalter", "skilled_trades"},
	{"finišerist", "skilled_trades"},
	{"bagerist", "skilled_trades"},
	{"žerjavist", "skilled_trades"},
	{"žerjavovodja", "skilled_trades"},
	{"strojnik", "skilled_trades"},
	{"strojevodja", "skilled_trades"},
	{"traktorist", "skilled_trades"},
	{"progar", "skilled_trades"},
	{"dimnikar", "skilled_trades"},
	{"vodoinštalater", "skilled_trades"},
	{"vodovodar", "skilled_trades"},
	{"inštalater", "skilled_trades"},
	{"elektroinstalater", "skilled_trades"},
	{"elektroinštalater", "skilled_trades"},
	{"elektrikar", "skilled_trades"},
	{"elektromehanik", "skilled_trades"},
	{"elektrovzrževalec", "skilled_trades"},
	{"elektrotehnik", "skilled_trades"},
	{"avtoelektričar", "skilled_trades"},
	{"avtoklepar", "skilled_trades"},
	{"avtoličar", "skilled_trades"},
	{"avtokaroserist", "skilled_trades"},
	{"avtomehanik", "skilled_trades"},
	{"avtoserviser", "skilled_trades"},
	{"avtomatik", "industrial_engineering"},
	{"vulkanizer", "skilled_trades"},
	{"mehatronik", "skilled_trades"},
	{"mehanik", "skilled_trades"},
	{"ključavničar", "skilled_trades"},
	{"cevar", "skilled_trades"},
	{"kovač", "skilled_trades"},
	{"kamnosek", "skilled_trades"},
	{"orodjar", "skilled_trades"},
	{"varilec", "skilled_trades"},
	{"varilca", "skilled_trades"},
	{"peskar", "skilled_trades"},
	{"montažer", "skilled_trades"},
	{"monter", "skilled_trades"},
	{"monterji", "skilled_trades"},
	{"servbiser", "skilled_trades"},
	{"vzrževalec", "skilled_trades"},
	{"žičničar", "skilled_trades"},
	{"nastavljalec", "manufacturing"},
	{"nastavljavec", "manufacturing"},
	{"strugar", "manufacturing"},
	{"kovinostrugar", "manufacturing"},
	{"rezkalec", "manufacturing"},
	{"rezkar", "manufacturing"},
	{"brusilec", "manufacturing"},
	{"polirec", "manufacturing"},
	{"lakirec", "manufacturing"},
	{"lakirničar", "manufacturing"},
	{"ličar", "manufacturing"},
	{"lakiranje", "manufacturing"},
	{"kalilec", "manufacturing"},
	{"talilec", "manufacturing"},
	{"topilec", "manufacturing"},
	{"livar", "manufacturing"},
	{"galvanizer", "manufacturing"},
	{"brizgalec", "manufacturing"},
	{"ekstruder", "manufacturing"},
	{"obdelovalec", "manufacturing"},
	{"preoblikovalec", "manufacturing"},
	{"plastilec", "manufacturing"},
	{"kovinar", "manufacturing"},
	{"lepilec", "manufacturing"},
	{"tiskar", "manufacturing"},
	{"tiskanje", "manufacturing"},
	{"knjigovez", "manufacturing"},
	{"krojač", "manufacturing"},
	{"šivilja", "manufacturing"},
	{"šivlja", "manufacturing"},
	{"šiviljec", "manufacturing"},
	{"steklopihalec", "manufacturing"},
	{"svečar", "manufacturing"},
	{"pek", "hospitality"},
	{"mlinar", "manufacturing"},
	{"mesar", "manufacturing"},
	{"tehtalec", "manufacturing"},
	{"tehtanje", "manufacturing"},
	{"polnilec", "manufacturing"},
	{"pakirec", "manufacturing"},
	{"proizvodnja", "manufacturing"},
	{"obratovodja", "manufacturing"},
	{"kolorist", "manufacturing"},
	{"kontroler", "finance"},
	{"vzorčevalec", "science"},
	{"telefonist", "support"},
	{"medicinskasestra", "healthcare"},
	{"dispečer", "logistics"},
	{"finančnik", "finance"},
	{"elektromonterji", "skilled_trades"},
	{"kadrovik", "hr"},
	{"kadrovnik", "hr"},
	{"kadrovski", "hr"},
	{"knjigovodja", "finance"},
	{"krovstvo", "skilled_trades"},
	{"narezovalec", "manufacturing"},
	{"odvetnik", "legal"},
	{"pravnik", "legal"},
	{"peskanje", "manufacturing"},
	{"prodaja", "sales"},
	{"prodajnik", "sales"},
	{"selekcionist", "recruiting"},
	{"skladiščnika", "logistics"},
	{"voznika", "logistics"},
	{"knaufar", "skilled_trades"},
	{"zlagalec", "manufacturing"},
	{"tajnik", "administration"},
	{"sekretar", "administration"},
	{"sekretarka", "administration"},
	{"evidentičar", "administration"},
	{"gradbenik", "skilled_trades"},
	{"gradbinec", "skilled_trades"},
	{"računalnikar", "support"},
	{"plakatiranje", "marketing"},

	// The desk a title names, in the case the title puts it in: last, so a role noun
	// anywhere in the title is read first.
	{"sprzedaży", "sales"},
	{"prodeje", "sales"},
	{"predaja", "sales"},
	{"prodaje", "sales"},
	{"prodaji", "sales"},
	{"értékesítési", "sales"},
	{"értékesítő", "sales"},
	{"продажу", "sales"},
	{"продажів", "sales"},
	{"продажби", "sales"},
	{"marketingu", "marketing"},
	{"promocji", "marketing"},
	{"маркетингу", "marketing"},
	{"produkcji", "manufacturing"},
	{"výroby", "manufacturing"},
	{"výrobě", "manufacturing"},
	{"výrobe", "manufacturing"},
	{"proizvodnji", "manufacturing"},
	{"proizvodnje", "manufacturing"},
	{"proizvodnju", "manufacturing"},
	{"termelési", "manufacturing"},
	{"виробництва", "manufacturing"},
	{"производство", "manufacturing"},
	{"logistyki", "logistics"},
	{"magazynu", "logistics"},
	{"magazynowy", "logistics"},
	{"magazynowa", "logistics"},
	{"magazynowej", "logistics"},
	{"floty", "logistics"},
	{"logistiky", "logistics"},
	{"skladu", "logistics"},
	{"logistike", "logistics"},
	{"logistici", "logistics"},
	{"logistiki", "logistics"},
	{"logisztikai", "logistics"},
	{"raktári", "logistics"},
	{"логістики", "logistics"},
	{"zakupów", "operations"},
	{"nákupu", "operations"},
	{"nabave", "operations"},
	{"nabavi", "operations"},
	{"beszerző", "operations"},
	{"beszerzési", "operations"},
	{"закупівель", "operations"},
	{"vzdrževanja", "skilled_trades"},
	{"održavanja", "skilled_trades"},
	{"budowy", "skilled_trades"},
	{"finansowy", "finance"},
	{"finansowa", "finance"},
	{"finansowej", "finance"},
	{"finansów", "finance"},
	{"płac", "finance"},
	{"rozliczeń", "finance"},
	{"windykacji", "finance"},
	{"bankowości", "finance"},
	{"bankar", "finance"},
	{"łańcucha dostaw", "logistics"},
	{"łańcuchem dostaw", "logistics"},
	{"łańcuchu dostaw", "logistics"},
	{"działu dostaw", "logistics"},
	{"flotą", "logistics"},
	{"bančni", "finance"},
	{"bérszámfejtési", "finance"},
	{"фінансовий", "finance"},
	{"кадрів", "hr"},
	{"виробництво", "manufacturing"},
	{"skladišta", "logistics"},
	{"údržby", "skilled_trades"},
	{"održavanju", "skilled_trades"},

	// The bare programmer and developer nouns are software in each of these languages,
	// the way "программист" is — but only once nothing else in the title has spoken.
	// Declared LAST, so "CNC operater - programer" is the machine operator, "Tehnolog -
	// programer" the technologist, and only a title that names nothing else is software.
	{"programista", "software_engineering"},
	{"programistka", "software_engineering"},
	{"programer", "software_engineering"},
	{"programerka", "software_engineering"},
	{"programátor", "software_engineering"},
	{"programátorka", "software_engineering"},
	{"programozó", "software_engineering"},
	{"програміст", "software_engineering"},
	{"розробник", "software_engineering"},
	{"програмист", "software_engineering"},
	{"razvijalec", "software_engineering"},
	{"razvijalka", "software_engineering"},

	// The ВОХР armed guard (see the public_safety block), declared last behind the pin
	// for the military contract posting that also names a rifleman: SOC 55 has no
	// category here, and the pin only reaches a title nothing above resolved.
	{"военнослужащий", categoryNone},
	{"стрелок", "public_safety"},
}

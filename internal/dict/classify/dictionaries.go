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
	// Japanese. Every alias is a COMPOUND, and that is the design rather than a shortcut:
	// a bare エンジニア ("engineer"), データ ("data"), 開発 ("development") or デザイナー
	// ("designer") runs through Japanese manufacturing, procurement and recruiting titles —
	// 半導体プロセス生産エンジニア is a semiconductor production engineer, 電気電子部品購買担当者
	// buys parts, 採用担当…エンジニアリング事業本部 is a RECRUITER, and トラクタのキャビン…設計開発
	// designs a tractor cab. Matching a single token would have declared all of them
	// technical, which is the trap a bare "analyst" already sprang on 77k postings.
	//
	// Japanese writes no spaces, so these match as substrings; the probe that established
	// the matcher accepts them is classify_ja_test.go, whose negative half is nine real
	// hrmos titles that must stay unclassified.
	//
	// Placed ahead of the Latin aliases for the same reason the table is ordered at all:
	// フロントエンドから設計・AWSまで！…Webアプリケーションエンジニア contains "AWS", and the
	// compound must win over it.
	{"機械学習エンジニア", "ml_ai"},
	{"mlエンジニア", "ml_ai"},
	{"データサイエンティスト", "data_science"},
	{"データエンジニア", "data_engineering"},
	{"データアナリスト", "data_analytics"},
	{"sreエンジニア", "sre"},
	{"インフラエンジニア", "devops"},
	{"サーバーエンジニア", "devops"},
	{"ネットワークエンジニア", "devops"},
	{"ネットワーク設計構築エンジニア", "devops"},
	{"クラウドエンジニア", "devops"},
	{"プラットフォームエンジニア", "devops"},
	{"セキュリティエンジニア", "security"},
	{"バックエンドエンジニア", "backend"},
	{"サーバーサイドエンジニア", "backend"},
	{"フロントエンドエンジニア", "frontend"},
	{"webアプリケーションエンジニア", "backend"},
	{"アプリケーションエンジニア", "backend"},
	{"webエンジニア", "backend"},
	{"ソフトウェアエンジニア", "backend"},
	{"モバイルエンジニア", "mobile"},
	{"iosエンジニア", "mobile"},
	{"androidエンジニア", "mobile"},
	{"qaエンジニア", "qa"},
	// UI/UX is the qualifier that makes this a product-design role; a bare デザイナー is
	// industrial or graphic design far more often (リードデザイナー/デザインセンター).
	{"ui/uxデザイナー", "design"},
	{"uiデザイナー", "design"},
	{"uxデザイナー", "design"},
	{"プロダクトデザイナー", "design"},
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
	// an estate, not a role, so it resolves with the other domain nouns down beside
	// the bare "manager"/"analyst" fall-throughs. Search for it there.
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
	// Bare "security" is NOT an entry, for the reason the Hungarian block below already
	// gives about "biztonsági": the word alone is the guard at least as often as the
	// discipline. Measured over the live catalogue, the single commonest title carrying
	// it is "Security Officer" (1 102 open postings), followed by "Security Specialist"
	// (602) and "Night shift security front desk - receptionist" (483) — none of them
	// technical, and "Security Supervisor" (149) and bare "Security" (91) behind them.
	// The bare alias sent every one of those to the security category, and because
	// `security` is in vocab.TechCategories that category is enough for
	// jobderive.TechEvidence to set is_tech TRUE on its own, ahead of the non-tech
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
	// Immigration practice: must precede the bare "manager" fall-through
	// ("Immigration Case Manager") and the administration block ("Immigration
	// Assistant") so neither steals this family. "immigration paralegal" is
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
	// Health, safety and environment. The block sits here, above the bare "manager"
	// fall-through, because that one line is what the profession was losing to: measured
	// on prod 2026-09-24, 1,454 live postings carrying an HSE acronym resolved
	// `management` on the strength of the word "Manager" alone, filing an EHS Manager
	// beside a Sales Manager. Another 4,515 resolved nothing at all.
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
	// Protective services. This catalogue has no category for them, so these keep the
	// blind sentinel — and they are paired with the role noun rather than left bare,
	// because the bare qualifier blanks titles that answer for themselves: "Public Safety
	// Dispatcher" (66 live) resolves `logistics` on the strength of `dispatcher`, and
	// "Public Safety Telecommunicator" and "Public Safety Communications" the same way.
	//
	// The pairs are measured, not enumerated from imagination — every combination below
	// exists in the live catalogue, and the ones that do not are deliberately absent. A
	// first pass listed only the `officer` form, which left 27 postings of these same
	// professions filed under HSE.
	{"public safety officer", categoryNone},     // 383
	{"campus safety officer", categoryNone},     // 83
	{"school safety officer", categoryNone},     // 8
	{"public safety manager", categoryNone},     // 5
	{"public safety specialist", categoryNone},  // 4
	{"public safety coordinator", categoryNone}, // 3
	{"public safety director", categoryNone},    // 3
	{"public safety technician", categoryNone},  // 1
	{"public safety trainer", categoryNone},
	{"public safety representative", categoryNone},
	{"campus safety specialist", categoryNone}, // 5
	{"campus safety supervisor", categoryNone},
	{"campus safety coordinator", categoryNone},
	{"campus safety manager", categoryNone},
	{"campus safety representative", categoryNone},
	{"campus safety intern", categoryNone},
	{"pool safety officer", categoryNone},
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
	// and why only the colliding pairs are listed. Manager and director resolve
	// `management`, which is what they resolved before this change and is an honest
	// statement about them; the rest name no category here.
	{"trust and safety manager", "management"},
	{"trust & safety manager", "management"}, // 2 + 2 across the two spellings
	{"trust and safety director", "management"},
	{"trust & safety director", "management"}, // 2
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
	// Bare "manager" resolves last so a functional prefix wins ("Sales Manager"
	// → sales, "Operations Manager" → operations, "Finance Manager" → finance); a
	// manager title with no recognized function falls through to management.
	{"manager", "management"},
	{"менеджер", "management"},
	// There is deliberately NO bare "analyst"/"аналитик" here, and the asymmetry with
	// the "manager" pair above is the point. Both were written as the same idea — a
	// title with no recognised function falls through — but "manager" falls through to
	// `management`, which is non-technical, while "analyst" fell through to
	// `data_analytics`, which is not: an unrecognised analyst was thereby DECLARED a
	// technical data role.
	//
	// Measured on the open catalogue on 2026-09-06, that facet held 102,222 postings and
	// 77,086 of them (75%) carried no data, analytics or BI word at all. 13,748 were
	// Board Certified Behavior Analysts — school ABA therapists — and the rest
	// investment, purchasing, risk, pricing, research, policy and tax analysts. All
	// 102,222 had is_tech = true, so all of them were eligible for the enrichment budget,
	// and TechEvidence's veto shielded every one from the non-technical dictionary.
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
	// `engineering manager` far above, so "Software Engineering Manager" stays
	// management, which is the craft that title actually names.
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

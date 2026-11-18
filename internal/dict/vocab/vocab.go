// Package vocab holds the controlled vocabularies that pin down the allowed
// values of every enum-shaped job facet. It is a neutral, dependency-free home
// shared by the enrichment contract (internal/ai/enrich), the deterministic
// dictionaries (internal/dict/classify, internal/dict/location, internal/dict/skilltag's
// consumers), the source parsers (internal/ingest/sources), and the read side — so an
// ingest-layer package never has to import the AI-enrichment layer just to
// reference a canonical value.
//
// One definition prevents value fragmentation ("senior" vs "Senior" vs "sr").
// ISO-standard fields (countries, salary_currency, posting_language) and the open
// skills field have no closed vocabulary here.
package vocab

// Controlled vocabularies. Each is the ordered, canonical list of allowed
// values for one enum field. They are exported so the enrichment prompt, the
// deterministic dictionaries, and the facet config reference the same lists.
var (
	WorkModeValues = []string{"remote", "hybrid", "onsite"}
	// RegionValues is the geographic-area vocabulary: a single, consistent macro
	// level (continents/macro-regions, plus `global` and the distinct `uk` area).
	// Country codes are NOT regions — country-level filtering lives in the separate
	// `countries` facet, so the US collapses into `north_america` and Russia into
	// `cis`. `cis` covers the whole post-Soviet space (Russia, Belarus, Moldova,
	// the Caucasus, and the five Central Asian republics) that dominates the
	// Telegram sources.
	RegionValues = []string{
		"global", "north_america", "latam", "eu", "uk",
		"mena", "africa", "apac", "cis",
	}
	EmploymentTypeValues = []string{"full_time", "part_time", "contract", "internship", "fellowship"}
	RelocationValues     = []string{"not_supported", "supported", "required"}
	SalaryPeriodValues   = []string{"year", "month", "day", "hour"}
	SeniorityValues      = []string{"intern", "junior", "middle", "senior", "lead", "staff", "principal", "c_level"}
	// RoleTypeValues answers "does this role manage people?" — an axis none of the
	// other vocabularies covers. It holds ONE value on purpose. The management side
	// is legible from a title (`head of`, `director`, a craft-qualified manager);
	// the individual-contributor side is not, and treating the absence of a marker
	// as proof of the opposite is exactly the inference the dictionaries in this
	// repo are forbidden to make. An unresolved posting means "no marker found",
	// never "individual contributor" — see internal/dict/roletype.
	RoleTypeValues = []string{"people_manager"}
	// SourceTypeValues classify where a posting was read: a multi-tenant ATS, one
	// employer's own careers site (or a vacancy a verified employer published here),
	// a multi-company aggregator, or a community channel. The classifier is
	// sources.ProviderKind; a source it cannot place (a manual import, a pasted link)
	// carries no value, so it is reachable only through the `source` facet.
	SourceTypeValues = []string{"ats", "company", "aggregator", "social"}
	// PerkValues are the benefits a posting's description states it offers — the
	// `perks` facet, matched by internal/dict/perks from explicit text only. Absence
	// means "not mentioned", never "not offered". Visa sponsorship and relocation are
	// deliberately NOT here: both already have their own enrichment-backed controls
	// (visa_sponsorship, relocation), and a dictionary reading of either is dominated by
	// the denial ("visa sponsorship is not available"), which the model reads and a
	// phrase list does not. Equity is not here either: it is its own compensation facet,
	// and a perk chip beside it would answer the same question twice.
	PerkValues = []string{
		"generous_pto", "four_day_week", "401k_match", "parental_leave",
		"retirement_plan", "tuition_reimbursement",
		"unlimited_pto", "health_insurance", "mental_health",
		"learning_budget", "home_office_stipend", "sabbatical", "sign_on_bonus",
	}
	// EngagementValues are the legal terms a posting states it engages its worker on —
	// the `engagement` facet, matched by internal/dict/engagement from explicit text
	// only. It answers "employee or own business, and on what tax footing", which
	// employment_type (how long and how many hours) does not: a Polish B2B role is
	// full_time, a UK Inside IR35 role is a contract taxed like a salary. Multi-valued,
	// because a posting may offer more than one ("Inside or Outside IR35", "CLT ou PJ").
	// Each value is one market's own term, grouped by market in the order below.
	EngagementValues = []string{
		"ir35_outside", "ir35_inside",
		"w2", "1099", "c2c", "contract_to_hire",
		"b2b", "uop", "umowa_zlecenie", "umowa_o_dzielo",
		"clt", "pj",
		"tk", "gph", "samozanyaty", "ip",
		"freiberuflich", "arbeitnehmerueberlassung",
		"zzp",
		"auto_entrepreneur", "portage_salarial",
		"autonomo", "monotributo",
	}
	EnglishLevelValues = []string{"none", "a1", "a2", "b1", "b2", "c1", "c2", "native"}
	// EquityValues answer "does this job pay in equity?" — derived from the posting's
	// text by internal/job/jobfacts.Equity. There is no "no equity" value: a posting the
	// matcher reads nothing from is NOT STATED, which is the absence of a value, not a
	// member of the vocabulary (the search filter names it EquityNotStated).
	EquityValues         = []string{EquityWithSalary, EquityOnly}
	EducationLevelValues = []string{"none", "bachelor", "master", "phd"}
	// ClearanceLevelValues are the security-clearance levels a posting may state,
	// written by internal/dict/location.ClearanceLevelsFromDescription. The US ladder
	// in ascending order, then interim (a status stated beside a level) and other
	// (a non-US scheme, or a clearance stated with no level). The location package
	// argues the split with the measured numbers.
	ClearanceLevelValues = []string{
		ClearanceConfidential, ClearanceSecret, ClearanceTopSecret, ClearanceTSSCI,
		ClearancePublicTrust, ClearanceInterim, ClearanceOther,
	}
	CategoryValues = []string{
		// "software_engineering" is the generic bucket for a title the dictionary
		// confirms is software/IT work (feeds is_tech via classify.IsTech's
		// techTitleTerms) but that names no sub-discipline to resolve to — a bare
		// "Software Engineer" or "Java Developer" does not say backend vs frontend
		// vs fullstack, and classify never guesses. Before this category existed
		// that population sat at `category = ""` forever (~110k open postings on
		// prod at introduction) despite being fully enriched; this gives it a home
		// instead of leaving it unfilterable.
		"software_engineering",
		"backend", "frontend", "fullstack", "mobile", "devops", "sre",
		"network_engineering", "system_administration", "telecommunications_engineering",
		// "it_support" is the help desk and service desk: desktop, deskside and
		// end-user support, the L1/L2/L3 tiers, the technical support engineer of a
		// software product. It was split out of `support` in 2026-10, which held it
		// beside customer service and the call centre — 143,798 open postings of
		// which only ~15k were technical — so a candidate with years on a service
		// desk found no filter that named their work. It is TECHNICAL (the desk runs
		// an IT estate), while `support` stays the non-technical customer side.
		"it_support",
		"data_engineering", "data_science", "data_analytics", "ml_ai", "ai_engineering",
		"qa", "security", "hardware", "embedded", "blockchain", "architecture",
		// "creative" is media production — video, animation, art, audio and
		// photography. It is a sibling of `design`, not a slice of it: the titles
		// it claims resolved to NO category before it existed, apart from the
		// audio designers who sat in `design` for no reason beyond the word
		// "designer" in the title. Product, motion, graphic, visual and brand
		// design stay where they are.
		// "industrial_engineering" is the engineering seat a factory, plant, utility
		// or field-service organisation staffs — manufacturing, process, quality,
		// maintenance, controls, commissioning, reliability, field service. A sibling
		// of `engineering_design`, not a slice: draughting is drawing the thing, this is
		// making and running it. Many of its titles are Russian (`Инженер`,
		// `Инженер-технолог`, `Инженер ПТО`) with no English alias.
		"design", "creative", "engineering_design", "industrial_engineering",
		// "occupational_safety" is the health, safety and environment seat — the
		// profession that spells itself HSE, EHS, HSSE, QHSE, HSEQ, SHE, SHEQ and
		// SHES. It is a sibling of `industrial_engineering`, not a slice of it: that
		// category is the seat a plant staffs to make and run the thing, this is the
		// compliance and controls function that audits it, and an HSE Manager is not
		// a process engineer. Both national occupational classifications place it
		// this way — SOC minor group 19-5000, under Life/Physical/Social Science, and
		// ISCO-08 unit group 2263, under Other Health Professionals — and neither
		// files it under engineering.
		//
		// Measured on prod 2026-09-24: the acronyms alone matched 6,254 live
		// postings, 4,515 of them with no category at all (unreachable from every
		// facet) and 1,454 sitting in `management` because the bare `manager`
		// fall-through outranked a block that did not exist.
		"occupational_safety",
		// There is no "management": it was retired 2026-10. It held the titles whose
		// only resolved word was "manager" — about 6% of the open catalogue, a Burger
		// King Assistant Manager beside a Tax Manager — and said nothing about the work.
		// A manager title now resolves the craft it manages, and whether it manages
		// PEOPLE is the role_type facet (RoleTypeValues, internal/dict/roletype). An old
		// link or saved search carrying category=management is translated to
		// role_type=people_manager by the search filter (RetiredCategoryManagement).
		"product", "project_management",
		// The consumer industries a broad multi-industry ATS crawl carries in with the
		// boards it wants. They are FILTERABLE — a facet excludes as well as it
		// selects — and, since the catalogue is broader than IT, a posting filed
		// under one is kept at ingest and searchable under it.
		"healthcare", "skilled_trades", "retail", "hospitality",
		// The service sectors: moving goods, teaching, the front desk, and the
		// personal and facility services. Same placement as the four above — non-
		// technical, and deliberately NOT craft-protected.
		"logistics", "education", "personal_services", "administration",
		// Three families the broader-than-IT catalogue keeps meeting and had no seat
		// for, each named after its SOC major group: community and social service
		// (SOC 21), life, physical and social science (SOC 19 — the laboratory, not the
		// research lab of a software company), and production (SOC 51 — the plant
		// floor, distinct from the construction and repair trades). With no category a
		// posting is refused at ingest and excluded from search, so a role nobody can
		// file is a role the catalogue loses.
		"social_services", "science", "manufacturing",
		// The protective services, SOC major group 33: police, sheriffs, corrections,
		// firefighters, security guards, lifeguards, crossing guards. Not information
		// security — that is `security`, and the two must never share a facet.
		"public_safety",
		"marketing", "sales", "support",
		// IT-company roles: 4 technical, 6 business.
		"business_analysis", "solutions_engineering", "developer_relations", "technical_writing",
		"recruiting", "hr", "finance", "legal", "operations", "customer_success",
		"other",
	}
	// NonTechCategories are the CategoryValues for confidently non-technical roles: a
	// member feeds `jobderive.deriveIsTech` to `is_tech = false`, and the enrichment
	// enqueue gate (`EnqueueJobEnrichment` / `EnqueuePendingJobs`) reads
	// `is_tech IS TRUE`, so a confirmed non-tech role never consumes LLM budget. The
	// gate is stricter than this list: it ALSO excludes `is_tech IS NULL` (category left
	// unresolved), which enriched almost nothing useful. The back-office roles
	// (recruiting/hr/finance/legal/operations/customer_success) and `engineering_design`
	// (draughting: engineering, but not the IT work this catalogue serves) join this
	// set: filterable facets, kept out of the LLM and embedding budget.
	NonTechCategories = []string{
		"marketing", "sales", "support",
		"recruiting", "hr", "finance", "legal", "operations", "customer_success",
		"engineering_design", "industrial_engineering", "occupational_safety",
		"healthcare", "skilled_trades", "retail", "hospitality",
		"logistics", "education", "personal_services", "administration",
		"social_services", "science", "manufacturing", "public_safety",
	}
	// TechCategories are the CategoryValues for recognized technical roles: every
	// category that is neither a NonTechCategories member nor the residual "other".
	// It is the single source of truth for "is this a technical category?" that the
	// is_tech derivation reads; a test asserts the three sets partition CategoryValues
	// exactly. Product/design/project_management and the IT-product-adjacent business
	// roles count as technical here, so they are enriched; the back-office roles are not.
	//
	// These are the categories a TECH COMPANY staffs, not "is this a technical craft":
	// `product`, `project_management`, `design`, `creative` and `business_analysis` are
	// wrong anywhere the population is not software, and the bare `analyst` alias under
	// `data_analytics` is VAT and credit control on a finance board. Do not use it to
	// decide whether to STORE a posting from a general job board (see
	// internal/ingest/sources/profession.go). For "is the CRAFT technical", use
	// `classify.IsTech` (strict, software-anchored) and treat this list as the facet
	// vocabulary.
	TechCategories = []string{
		"software_engineering",
		"backend", "frontend", "fullstack", "mobile", "devops", "sre",
		"network_engineering", "system_administration", "telecommunications_engineering", "it_support",
		"data_engineering", "data_science", "data_analytics", "ml_ai", "ai_engineering",
		"qa", "security", "hardware", "embedded", "blockchain", "architecture",
		"design", "creative", "product", "project_management",
		"business_analysis", "solutions_engineering", "developer_relations", "technical_writing",
	}
	// DomainValues is the industry/vertical of the company or product behind a job
	// (what the company does), an LLM-emitted multi-value enum glossed per-value in the
	// enrichment prompt. Each value names a vertical, never a business model — "saas"
	// was dropped for that reason (it overlaps every vertical); its coverage moved to
	// "devtools" and the functional verticals. Synonyms fold into one canonical
	// (web3->crypto, insurtech->fintech, martech->adtech, social/dating->media,
	// biotech->healthcare, retail->ecommerce, greentech->climatetech).
	DomainValues = []string{
		"fintech", "crypto", "ecommerce", "gambling", "gamedev", "media", "travel",
		"healthcare", "edtech", "govtech",
		"devtools", "cybersecurity", "ai", "hrtech", "adtech", "proptech",
		"logistics", "mobility", "climatetech", "other",
	}
	// DomainGloss is the one-line definition of each domain, supplied to the enrichment
	// LLM so it classifies on what the company does rather than guessing from a bare
	// name. Every DomainValues entry has a gloss (asserted by a test). "ai" is scoped to
	// core-product AI so it does not swallow every company that merely uses AI.
	DomainGloss = map[string]string{
		"fintech":       "payments, banking, lending, wealth/trading, insurtech, regtech (traditional financial rails)",
		"crypto":        "blockchain, web3, DeFi, tokens/NFTs, exchanges, on-chain infra",
		"ecommerce":     "online retail, marketplaces, D2C, retail/checkout/fulfillment tech",
		"gambling":      "betting, casino/iGaming, sportsbook, lottery",
		"gamedev":       "video-game development, publishing, game engines/infra",
		"media":         "content, publishing, streaming, entertainment, social networks, dating, creator economy",
		"travel":        "travel, hospitality, tourism, booking",
		"healthcare":    "health-tech, medtech, digital health, biotech, pharma, wellness",
		"edtech":        "education, e-learning, training, LMS",
		"govtech":       "government, public sector, civic tech",
		"devtools":      "developer tools, cloud infra, databases, DevOps, APIs, IT infrastructure",
		"cybersecurity": "security software, identity, threat detection, appsec, privacy, fraud",
		"ai":            "company whose CORE PRODUCT is AI/ML (model providers, AI/ML platforms, applied-AI) — NOT merely \"uses AI\"",
		"hrtech":        "recruiting, HR, payroll, people-ops, staffing software",
		"adtech":        "advertising and marketing technology (ad serving, attribution, CRM, marketing automation)",
		"proptech":      "real-estate and construction technology",
		"logistics":     "supply chain, freight, delivery, fleet, warehousing (goods)",
		"mobility":      "automotive, autonomous vehicles, ride-hailing, transport of people",
		"climatetech":   "climate, clean/renewable energy, sustainability",
		"other":         "none of the above (incl. generic horizontal productivity/CRM SaaS with no vertical)",
	}
	CompanyTypeValues = []string{"product", "startup", "outsource", "outstaff", "agency", "inhouse", "government"}
	// CompanyTypeGloss is the one-line definition of each company_type value, supplied
	// to the enrichment LLM so it distinguishes them by BUSINESS MODEL rather than
	// guessing from a bare label — outsource vs outstaff in particular collapse to the
	// same intuition ("body shop for hire") without an explicit contrast on who manages
	// the day-to-day work. Every CompanyTypeValues entry has a gloss (asserted by a test).
	CompanyTypeGloss = map[string]string{
		"product":    "builds and sells its OWN software product(s) directly to customers — the software is the business itself",
		"startup":    "early-stage company (a funding/maturity stage, not a service model) — small team, still finding product-market fit",
		"outsource":  "builds custom software FOR OTHER companies under contract — the vendor's own team manages and delivers whole projects/products for external clients",
		"outstaff":   "staff augmentation — places its own engineers to work EMBEDDED inside a client's team, day-to-day managed by the CLIENT (contrast with outsource, where the vendor manages delivery)",
		"agency":     "work-for-hire studio delivering marketing, design, branding, or creative services for clients — the non-engineering-delivery counterpart of outsource",
		"inhouse":    "an internal engineering/IT team inside a company whose core business is NOT software — builds tools for the company's own operations, not to sell",
		"government": "a government or public-sector body/agency",
	}
	// CompanyFeedbackTypeValues is the closed category vocabulary for a company
	// feedback/review entry (internal/engage/companyfeedback) — what aspect of the
	// experience the review is about, Glassdoor-style.
	CompanyFeedbackTypeValues = []string{
		"interview", "culture", "compensation", "management",
		"work_life_balance", "career_growth", "other",
	}
	// CompanyFeedbackReportReasonValues is the closed reason vocabulary for
	// flagging a specific company feedback entry (internal/engage/companyfeedback) —
	// deliberately smaller than internal/engage/report's job-reasons, since a review
	// report has no "no longer relevant" or "no response" equivalent.
	CompanyFeedbackReportReasonValues = []string{"spam", "offensive", "false_information", "other"}
	// CompanyProcessReportKindValues is the closed vocabulary of candidate-reported
	// facts about how a company hires (internal/engage/processreport). Unlike a
	// feedback entry it carries no rating and no text: each value is a statement that
	// is either true of an employer or not, which is why one reporter is enough to
	// show it and why there is nothing here for a moderator to decide.
	//
	// It is mirrored by a CHECK constraint in migration 0156, and it decides what the
	// badge renders and what the search facet declares — so a new value is a code
	// change and a migration, never configuration. Each value additionally needs its
	// own counter column, which is what keeps the cost of another one visible.
	CompanyProcessReportKindValues = []string{"ai_interview"}
	CompanySizeValues              = []string{"1-10", "11-50", "51-200", "201-500", "501-1000", "1000+"}
	// AIArchetypeValues is the six AI skill-signature archetype slugs
	// internal/ai/aiarchetype's rule table can derive, in priority order. Kept here
	// (rather than only inside internal/ai/aiarchetype) so cmd/gen-contracts can emit
	// it into the web contracts without importing the AI-enrichment layer; a test
	// in internal/ai/aiarchetype cross-checks this list against the rule table itself.
	AIArchetypeValues = []string{
		"rag_app_builder", "agent_builder", "cloud_ml_platform_engineer",
		"ml_trainer_researcher", "fullstack_ai_engineer", "devops_infra_engineer",
	}
	// JobSearchStageValues is how far along the candidate says they are, asked once in
	// onboarding (internal/candidate/survey). Ordered by commitment, not urgency:
	// `exploring` is last (least engaged); `not_started` has decided to start and has
	// not yet.
	//
	// About the candidate, not a posting: it reaches no facet, filter or enrichment
	// prompt. It lives here so the Go validator and the wizard's option list share one
	// definition.
	JobSearchStageValues = []string{"not_started", "searching", "employed_looking", "exploring"}
	// JobChallengeValues are the things a candidate says are in their way. Several may be
	// picked, except JobChallengeJustStarting, which stands alone.
	//
	// `english` is worded "Language requirements" since migration 0207; earlier answers
	// mean English only.
	//
	// `other` is the ONLY value a free-text note may accompany, so a note cannot
	// contradict a coded answer. TestJobChallengeHasOther locks it.
	JobChallengeValues = []string{
		"english", "recruiter_contact", "working_abroad", "technical_interviews",
		"job_fit", "remote_flexible", "visa_sponsorship", "first_job",
		JobChallengeJustStarting, JobChallengeOther,
	}
)

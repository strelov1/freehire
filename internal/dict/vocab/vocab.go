// Package vocab holds the controlled vocabularies that pin down the allowed
// values of every enum-shaped job facet. It is a neutral, dependency-free home
// shared by the enrichment contract (internal/ai/enrich), the deterministic
// dictionaries (internal/dict/classify, internal/dict/location, internal/dict/skilltag's
// consumers), the source parsers (internal/ingest/sources), and the read side — so an
// ingest-layer package never has to import the AI-enrichment layer just to
// reference a canonical value.
//
// Keeping the vocabularies here as one definition prevents value fragmentation
// (e.g. "senior" vs "Senior" vs "sr") across those layers. ISO-standard fields
// (countries, salary_currency, posting_language) and the open skills field have
// no bundled closed vocabulary here.
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
	RoleTypeValues       = []string{"people_manager"}
	EnglishLevelValues   = []string{"none", "a1", "a2", "b1", "b2", "c1", "c2", "native"}
	EducationLevelValues = []string{"none", "bachelor", "master", "phd"}
	CategoryValues       = []string{
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
		"network_engineering",
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
		// maintenance, controls, commissioning, reliability, field service. It is a
		// sibling of `engineering_design`, not a slice of it: draughting is drawing
		// the thing, this is making and running it, and a Quality Engineer is not a
		// draughtsman. Half the population it names is Russian (`Инженер`,
		// `Инженер-технолог`, `Инженер ПТО`) and carried no English alias at all.
		"design", "creative", "engineering_design", "industrial_engineering",
		"product", "project_management", "management",
		// The consumer industries a broad multi-industry ATS crawl carries in with the
		// boards it wants. They are here to be FILTERABLE — a facet excludes as well as
		// it selects, and 225k unfilterable postings are worse than four options no IT
		// candidate will choose. Deliberately NOT members of NonTechCraftCategories:
		// unlike the two engineering categories, these are exactly the non-technical
		// business cmd/prune's rule exists to remove, and leaving them out is what
		// keeps categorising them behaviour-neutral.
		"healthcare", "skilled_trades", "retail", "hospitality",
		// The service sectors: moving goods, teaching, the front desk, and the
		// personal and facility services. Same placement as the four above — non-
		// technical, and deliberately NOT craft-protected.
		"logistics", "education", "personal_services", "administration",
		"marketing", "sales", "support",
		// IT-company roles added by expand-role-taxonomy (4 technical, 6 business)
		"business_analysis", "solutions_engineering", "developer_relations", "technical_writing",
		"recruiting", "hr", "finance", "legal", "operations", "customer_success",
		"other",
	}
	// NonTechCategories are the CategoryValues for confidently non-technical roles: a
	// member feeds `jobderive.deriveIsTech` to `is_tech = false`, and the enrichment
	// enqueue gate (`internal/platform/db/queries/jobs.sql`'s EnqueueJobEnrichment /
	// `enrichment.sql`'s EnqueuePendingJobs) reads `is_tech IS TRUE`, so a confirmed
	// non-tech role never consumes LLM budget. That gate is stricter than this list
	// alone: it ALSO excludes `is_tech IS NULL` — a category the title dictionary and
	// description left unresolved in either direction — which used to enqueue by
	// default (the reasoning then: "never silently skip a tech job the dictionary
	// missed"). Measured at catalogue scale that unresolved bucket was ~65% of the
	// open catalogue and enrichment returned nothing useful for ~91% of it (broad
	// multi-industry ATS crawls), so the coverage it bought no longer justified the
	// spend — see the enqueue queries' comments for the full reasoning. The back-office
	// IT-company roles (recruiting/hr/finance/legal/operations/customer_success) join
	// this set: surfaced as facets but kept out of the LLM enrich budget, like
	// marketing/sales.
	// `engineering_design` — mechanical, electrical, civil and architectural draughting
	// — joins them for the same reason: it is engineering, but not the IT work this
	// catalogue serves, so it is filterable without spending LLM or embedding budget on
	// it. Note that two DELETE paths read TechCategories as a veto over the non-tech
	// title dictionary; `classify.ConfirmedNonTech` and `prune`'s business rule spare
	// this category explicitly, so membership here does not make its postings
	// removable.
	NonTechCategories = []string{
		"marketing", "sales", "support", "management",
		"recruiting", "hr", "finance", "legal", "operations", "customer_success",
		"engineering_design", "industrial_engineering",
		"healthcare", "skilled_trades", "retail", "hospitality",
		"logistics", "education", "personal_services", "administration",
	}
	// NonTechCraftCategories are the NonTechCategories members that are non-technical
	// because the CRAFT sits outside IT — not because the posting is back-office or
	// go-to-market work at a software employer. The distinction is not decorative:
	// `cmd/prune`'s business rule deletes non-technical categories at a company with no
	// technical history, and applied to these it would take out an engineering
	// employer's entire catalogue the moment its board was retired. That rule
	// SUBTRACTS this set.
	//
	// It lives here rather than in prune because it states what a category MEANS, and
	// because the alternative has already been tried: the exception was one category
	// named inline at the rule, which meant the second craft category could be added to
	// the vocabulary — by someone with no reason to open cmd/prune — and become
	// deletable in silence. A test asserts every member is also a NonTechCategories
	// member, so the two cannot drift.
	NonTechCraftCategories = []string{"engineering_design", "industrial_engineering"}
	// TechCategories are the CategoryValues for recognized technical roles: every
	// category that is neither a NonTechCategories member nor the residual "other".
	// It is the single source of truth for "is this a technical category?" that the
	// is_tech derivation reads, so the tech/non-tech/other split stays in one place;
	// a test asserts the three sets partition CategoryValues exactly. Product/design/
	// project_management — and the IT-product-adjacent business_analysis/
	// solutions_engineering/developer_relations/technical_writing — count as technical
	// here (IT-industry roles), so they are enriched; the back-office roles are not.
	TechCategories = []string{
		"software_engineering",
		"backend", "frontend", "fullstack", "mobile", "devops", "sre",
		"network_engineering",
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
	CompanySizeValues                 = []string{"1-10", "11-50", "51-200", "201-500", "501-1000", "1000+"}
	// AIArchetypeValues is the six AI skill-signature archetype slugs
	// internal/ai/aiarchetype's rule table can derive, in priority order. Kept here
	// (rather than only inside internal/ai/aiarchetype) so cmd/gen-contracts can emit
	// it into the web contracts without importing the AI-enrichment layer; a test
	// in internal/ai/aiarchetype cross-checks this list against the rule table itself.
	AIArchetypeValues = []string{
		"rag_app_builder", "agent_builder", "cloud_ml_platform_engineer",
		"ml_trainer_researcher", "fullstack_ai_engineer", "devops_infra_engineer",
	}
)

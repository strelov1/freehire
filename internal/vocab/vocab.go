// Package vocab holds the controlled vocabularies that pin down the allowed
// values of every enum-shaped job facet. It is a neutral, dependency-free home
// shared by the enrichment contract (internal/enrich), the deterministic
// dictionaries (internal/classify, internal/location, internal/skilltag's
// consumers), the source parsers (internal/sources), and the read side — so an
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
	EmploymentTypeValues = []string{"full_time", "part_time", "contract", "internship"}
	RelocationValues     = []string{"not_supported", "supported", "required"}
	SalaryPeriodValues   = []string{"year", "month", "day", "hour"}
	SeniorityValues      = []string{"intern", "junior", "middle", "senior", "lead", "staff", "principal", "c_level"}
	EnglishLevelValues   = []string{"none", "a1", "a2", "b1", "b2", "c1", "c2", "native"}
	EducationLevelValues = []string{"none", "bachelor", "master", "phd"}
	CategoryValues       = []string{
		"backend", "frontend", "fullstack", "mobile", "devops", "sre",
		"network_engineering",
		"data_engineering", "data_science", "data_analytics", "ml_ai", "ai_engineering",
		"qa", "security", "hardware", "embedded", "blockchain", "architecture",
		"design", "engineering_design", "product", "project_management", "management",
		"marketing", "sales", "support",
		// IT-company roles added by expand-role-taxonomy (4 technical, 6 business)
		"business_analysis", "solutions_engineering", "developer_relations", "technical_writing",
		"recruiting", "hr", "finance", "legal", "operations", "customer_success",
		"other",
	}
	// NonTechCategories are the CategoryValues for confidently non-technical roles
	// that are excluded from AI enrichment: the derived category is a source fact
	// (internal/classify, from the title), so gating the enrichment enqueue on it
	// keeps LLM budget off jobs no engineer will see. It is a blacklist, not a
	// whitelist — only categories we are sure are non-technical are dropped; an
	// empty/"other"/unrecognized category still enqueues, so a tech job the title
	// dictionary could not place is never silently skipped. The back-office IT-company
	// roles (recruiting/hr/finance/legal/operations/customer_success) join this set:
	// surfaced as facets but kept out of the LLM enrich budget, like marketing/sales.
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
		"engineering_design",
	}
	// TechCategories are the CategoryValues for recognized technical roles: every
	// category that is neither a NonTechCategories member nor the residual "other".
	// It is the single source of truth for "is this a technical category?" that the
	// is_tech derivation reads, so the tech/non-tech/other split stays in one place;
	// a test asserts the three sets partition CategoryValues exactly. Product/design/
	// project_management — and the IT-product-adjacent business_analysis/
	// solutions_engineering/developer_relations/technical_writing — count as technical
	// here (IT-industry roles), so they are enriched; the back-office roles are not.
	TechCategories = []string{
		"backend", "frontend", "fullstack", "mobile", "devops", "sre",
		"network_engineering",
		"data_engineering", "data_science", "data_analytics", "ml_ai", "ai_engineering",
		"qa", "security", "hardware", "embedded", "blockchain", "architecture",
		"design", "product", "project_management",
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
	CompanySizeValues = []string{"1-10", "11-50", "51-200", "201-500", "501-1000", "1000+"}
)

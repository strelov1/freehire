package skilltag

// professionalPhraseAliases is the non-engineering half of the IT-company role
// vocabulary: the recruiting, HR, finance, legal, operations, sales, customer-success
// and support craft a technical company hires for without hiring an engineer. Parse
// matches these exactly like any other phrase — a skills facet describes every posting,
// not only the technical ones.
//
// Developer relations, technical writing, business analysis and pre-sales stay in
// engineeringPhraseAliases: they are tech-industry craft, posted by technical employers.
var professionalPhraseAliases = []phraseAlias{
	// AI enablement — the adoption side of AI, and a skill rather than a role.
	// These postings are Program Managers, Architects, Change Managers and even
	// Interns whose SUBJECT is getting an organisation to use AI; tagging the
	// subject lets it filter alongside the role instead of displacing it, which
	// is what a named role did when this first shipped.
	//
	// "ai trainer" is deliberately absent: it names the opposite job — labelling
	// data so a model learns, not teaching people to use one.
	{"ai enablement", "ai-enablement"},
	{"ai adoption", "ai-enablement"},
	{"ai transformation", "ai-enablement"},
	// recruiting
	{"boolean search", "boolean-search"},
	{"talent sourcing", "talent-sourcing"},
	{"technical screening", "technical-screening"},
	{"candidate experience", "candidate-experience"},
	{"employer branding", "employer-branding"},
	{"full-cycle recruiting", "full-cycle-recruiting"}, {"full cycle recruiting", "full-cycle-recruiting"},
	{"linkedin recruiter", "linkedin-recruiter"},
	{"smartrecruiters", "smartrecruiters"},
	// hr / people
	{"employee relations", "employee-relations"},
	{"hr operations", "hr-operations"}, {"people operations", "hr-operations"},
	{"performance management", "performance-management"},
	{"talent management", "talent-management"},
	{"succession planning", "succession-planning"},
	{"people analytics", "people-analytics"},
	{"bamboohr", "bamboohr"}, {"successfactors", "successfactors"},
	// finance
	{"financial modeling", "financial-modeling"},
	{"revenue recognition", "revenue-recognition"},
	{"accounts payable", "accounts-payable"},
	{"accounts receivable", "accounts-receivable"},
	{"general ledger", "general-ledger"},
	{"financial reporting", "financial-reporting"},
	{"quickbooks", "quickbooks"}, {"netsuite", "netsuite"}, {"xero", "xero"},
	// legal
	{"contract negotiation", "contract-negotiation"},
	{"contract drafting", "contract-drafting"},
	{"contract lifecycle management", "contract-lifecycle-management"},
	{"legal research", "legal-research"},
	{"regulatory compliance", "regulatory-compliance"},
	// immigration-practice form numbers. Written with a space so the phrase
	// matcher's separator equivalence also resolves the hyphenated "I-129"/
	// "I-130" form the forms are actually styled with.
	{"i 129", "i-129"}, {"i 130", "i-130"},
	// operations
	{"process improvement", "process-improvement"},
	{"vendor management", "vendor-management"},
	{"sla management", "sla-management"},
	{"program management", "program-management"},
	{"strategic planning", "strategic-planning"},
	{"stakeholder management", "stakeholder-management"},
	// agile/PM certifications — spelled-out forms are unambiguous strong matches;
	// the acronym forms (CSM/PSM/PMP/SAFe) collide with other meanings in general
	// job text and resolve only via categoryScopedAcronyms below.
	{"certified scrummaster", "certified-scrummaster"}, {"certified scrum master", "certified-scrummaster"},
	{"professional scrum master", "professional-scrum-master"},
	{"project management professional", "pmp"},
	{"scaled agile framework", "safe-agile"},
	// sales
	{"account executive", "account-executive"},
	{"business development", "business-development"},
	{"pipeline management", "pipeline-management"},
	{"cold outreach", "cold-outreach"},
	{"sales enablement", "sales-enablement"},
	{"lead generation", "lead-generation"},
	// customer success
	{"customer onboarding", "customer-onboarding"},
	{"customer retention", "customer-retention"},
	{"churn prevention", "churn-prevention"},
	{"customer health score", "customer-health-score"},
	{"renewal management", "renewal-management"},
	{"gainsight", "gainsight"}, {"churnzero", "churnzero"},
	// support
	{"help desk", "help-desk"},
	{"service desk", "service-desk"},
	{"ticket resolution", "ticket-resolution"},
	// marketing — search optimization tooling. These are unambiguous product names,
	// so they are strong matches, which also makes them corroborators for the gated
	// `seo` canonical: before this block a posting could name its whole toolchain and
	// still lose the `seo` tag for want of an engineering token to corroborate it.
	{"semrush", "semrush"}, {"ahrefs", "ahrefs"},
	{"screaming frog", "screaming-frog"},
	{"google search console", "google-search-console"}, {"search console", "google-search-console"},
	// marketing — lifecycle, email and advertising platforms. The ad platforms are
	// phrases rather than bare vendor names on purpose: "meta", "google" and
	// "linkedin" alone name the company, not the ad product.
	{"klaviyo", "klaviyo"}, {"mailchimp", "mailchimp"}, {"customer.io", "customer-io"},
	{"google ads", "google-ads"}, {"google adwords", "google-ads"}, {"adwords", "google-ads"},
	{"meta ads", "meta-ads"}, {"facebook ads", "meta-ads"}, {"meta ads manager", "meta-ads"},
	{"tiktok ads", "tiktok-ads"}, {"linkedin ads", "linkedin-ads"},
	// marketing — measurement and social tooling. Segment, Buffer and Later are
	// missing on purpose: each is an ordinary English word in exactly the postings
	// this block serves ("the customer segment", "a content buffer", "apply later"),
	// and a strong alias would tag the corpus rather than describe the job.
	{"looker studio", "looker-studio"}, {"data studio", "looker-studio"},
	{"hootsuite", "hootsuite"}, {"sprout social", "sprout-social"},
	{"contentful", "contentful"},
	// marketing — the disciplines themselves, so a search can combine "does technical
	// SEO" with "uses Ahrefs". All multi-word, hence strong without gating. "SEM" is
	// absent by design: it is a Portuguese and Spanish preposition ("sem experiência")
	// and this catalogue carries that population in bulk.
	{"technical seo", "technical-seo"},
	{"link building", "link-building"}, {"linkbuilding", "link-building"},
	{"paid social", "paid-social"},
	{"paid search", "ppc"}, {"ppc campaigns", "ppc"}, {"ppc management", "ppc"},
	{"ppc specialist", "ppc"}, {"ppc manager", "ppc"}, {"pay-per-click", "ppc"},
	{"demand generation", "demand-generation"}, {"demand gen", "demand-generation"},
	{"lifecycle marketing", "lifecycle-marketing"}, {"crm marketing", "lifecycle-marketing"},
	{"marketing automation", "marketing-automation"},
	{"generative engine optimization", "generative-engine-optimization"},
	{"answer engine optimization", "generative-engine-optimization"},
	{"generative search optimization", "generative-engine-optimization"},
	{"content marketing", "content-marketing"},
	{"email marketing", "email-marketing"},
	{"influencer marketing", "influencer-marketing"},
	{"copywriting", "copywriting"},
	// GTM is go-to-market in a posting far more often than it is Google Tag Manager:
	// "GTM strategy" and "GTM motion" are the common forms, while the tag manager is
	// spelled out or named as a container. So the abbreviation belongs to this
	// canonical, and the product keeps its full name (declared with the other Google
	// products above) plus the container form.
	{"go-to-market", "go-to-market"}, {"go to market", "go-to-market"},
	{"gtm strategy", "go-to-market"}, {"gtm motion", "go-to-market"},
	{"gtm plan", "go-to-market"}, {"gtm execution", "go-to-market"},
	{"gtm container", "google-tag-manager"},

	// LLM-mined batch 4 — the business-software half. These sit here rather than with
	// the engineering vocabulary because they are business tools: a board that only ever
	// posts recruiters, AEs and payroll admins names Workday and Gong all day.
	//
	// Each is phrase-routed because the vendor took an ordinary word: a "flexible
	// workday", "I concur", a "gong", the god Apollo, the building "intercom", a
	// "greenhouse". Bare "adp" is three letters, so it carries its product suffix too.
	{"workday hcm", "workday"}, {"workday financials", "workday"},
	{"workday adaptive", "workday"}, {"workday integrations", "workday"},
	{"sap concur", "concur"},
	{"adp workforce now", "adp"}, {"adp payroll", "adp"},
	{"apollo.io", "apollo-io"},
	{"gong.io", "gong"},
	{"linkedin sales navigator", "linkedin-sales-navigator"},
	{"sales navigator", "linkedin-sales-navigator"},
	// Unambiguous single-token vendor names, same shape as quickbooks/bamboohr above.
	{"zoominfo", "zoominfo"}, {"salesloft", "salesloft"},
	{"veeva", "veeva"}, {"yardi", "yardi"},
	// Privacy and trade-control regimes. They belong to this list rather than the
	// engineering one: a posting whose only match is HIPAA is a healthcare compliance
	// role. All are non-corroborating — see below.
	{"hipaa", "hipaa"}, {"gdpr", "gdpr"}, {"ccpa", "ccpa"}, {"lgpd", "lgpd"},
	{"itar", "itar"}, {"dfars", "dfars"},
	// batch 6 (2026-10-25, German market) — mined from the requirement clauses of the
	// German ATS crawl ("Kenntnisse in …", "Erfahrung mit …"), which carries no LLM
	// skills to mine. Bare "sap" stays gated (tree sap); the German clause that binds
	// it to a requirement does not have that reading. Only German forms, so the English
	// decisions about "SAP system" and "CRM" stay as they were: "systeme"/"systemen"
	// are German plurals, never English. All non-corroborating — see below.
	{"sap kenntnisse", "sap"}, {"sap kenntnissen", "sap"}, {"sap anwenderkenntnisse", "sap"},
	{"kenntnisse in sap", "sap"}, {"erfahrung mit sap", "sap"}, {"umgang mit sap", "sap"},
	{"sap erfahrung", "sap"}, {"sap umfeld", "sap"},
	{"sap systeme", "sap"}, {"sap systemen", "sap"},
	{"sap r/3", "sap"}, {"sap hcm", "sap"}, {"sap is-u", "sap"}, {"sap sd", "sap"},
	{"sap fi", "sap-fico"}, {"sap co", "sap-fico"}, {"sap fico", "sap-fico"},
	{"datev", "datev"}, {"lodas", "datev"},
	{"erp systeme", "erp"}, {"erp systemen", "erp"},
	{"crm systeme", "crm"}, {"crm systemen", "crm"},
	// batch 7 (2026-10-25, Dutch market) — the HR, payroll and bookkeeping suites a Dutch
	// clerk's or a functional administrator's posting asks for. All non-corroborating.
	{"afas", "afas"}, {"exact online", "exact-online"}, {"nmbrs", "nmbrs"},
	{"youforce", "youforce"},
}

// occupationalSafetyPhraseAliases is its own slice rather than more lines in the list
// above, for one reason: every canonical it emits must also be a nonCorroboratingPhrases
// member, and that set now DERIVES from this slice instead of repeating its canonicals by
// hand. A hand-typed copy drifts: a thirty-sixth safety phrase added to a literal would
// silently become a strong corroborator, which is the bug this vocabulary shipped with once.
var occupationalSafetyPhraseAliases = []phraseAlias{
	// Occupational safety — health, safety and environment. This vocabulary carries more
	// weight than its size suggests: an HSE posting derives `is_tech = false` and the
	// enrichment enqueue gate reads `is_tech IS TRUE`, so the LLM never sees this
	// population and these entries are the only thing that will ever put a skill on it.
	// Measured on prod 2026-09-24, just 2,535 of 6,254 live HSE postings carried any
	// skill, and the profession had no representation here at all.
	//
	// The terms are mined, not written down: 2,500 live descriptions from this exact
	// population, HTML-stripped, counted as 1–3-grams by document frequency with the
	// existing vocabulary subtracted. The percentage on each line is that frequency, and
	// it is the admission evidence — a list written from what the work sounds like can
	// only ever be tested against itself.
	{"iso 45001", "iso-45001"}, {"iso45001", "iso-45001"}, // 19%
	{"iso 14001", "iso-14001"}, {"iso14001", "iso-14001"}, // 20%
	{"emergency response", "emergency-response"}, // 20%
	{"emergency preparedness", "emergency-response"},
	{"personal protective equipment", "personal-protective-equipment"},              // 14%
	{"root cause analysis", "root-cause-analysis"},                                  // 14%
	{"environmental compliance", "environmental-compliance"},                        // 14%
	{"risk assessment", "risk-assessment"}, {"risk assessments", "risk-assessment"}, // 13%
	{"incident investigation", "incident-investigation"}, {"incident investigations", "incident-investigation"}, // 13%
	{"safety management system", "safety-management-system"}, // 13%
	{"hse management system", "safety-management-system"},
	{"ehs management system", "safety-management-system"},
	{"industrial hygiene", "industrial-hygiene"},                                            // 12%
	{"corrective action", "corrective-action"}, {"corrective actions", "corrective-action"}, // 29%, the single most common of these
	{"corrective and preventive action", "corrective-action"},
	{"waste management", "waste-management"},                              // 9%
	{"first aid", "first-aid"},                                            // 9%
	{"hazardous waste", "hazardous-waste"},                                // 8%
	{"toolbox talk", "toolbox-talks"}, {"toolbox talks", "toolbox-talks"}, // 8%
	{"safety audit", "safety-audit"}, {"safety audits", "safety-audit"}, // 8%; the corpus spells it plural
	{"hazard identification", "hazard-identification"}, // 7%
	{"contractor safety", "contractor-safety"},         // 6%
	{"lockout tagout", "lockout-tagout"},               // 6%
	{"lockout/tagout", "lockout-tagout"},
	{"confined space", "confined-space"}, {"confined spaces", "confined-space"}, // 5%
	{"fall protection", "fall-protection"}, // 3%
	{"machine guarding", "machine-guarding"},
	{"hot work", "hot-work"},
	{"near miss", "near-miss-reporting"}, {"near misses", "near-miss-reporting"}, // the plural is the 11% form
	// The acronym PSM is NOT admitted: the scoped-acronym table below already maps that
	// key to professional-scrum-master, and it holds one canonical per key. Measured, the
	// acronym and this phrase each appear in 2% of the corpus, so the phrase alone loses
	// almost nothing — and widening the table's shape would touch every existing entry.
	{"process safety management", "process-safety-management"},
	{"permit to work", "permit-to-work"},           // 2%
	{"job safety analysis", "job-safety-analysis"}, // 2%
	{"job hazard analysis", "job-safety-analysis"},
	{"behavior based safety", "behavior-based-safety"},
	{"behaviour based safety", "behavior-based-safety"},
	{"working at height", "working-at-height"},
	{"work at height", "working-at-height"},
}

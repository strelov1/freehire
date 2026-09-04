package enrich

// CompanyTypeHints is a small, hand-curated map from company_slug to a confirmed
// vocab.CompanyTypeValues entry, for well-known IT outsourcing / staff-augmentation
// companies whose individual postings routinely say nothing about the employer's own
// business model — there is nothing in a single job description for the LLM to key off,
// so company_type comes back null far more often for these than for a product company
// ("we are hiring a backend engineer" reads the same whether the employer is Google or
// a body shop). See buildSystemPrompt and userPrompt for how the hint reaches the
// prompt, and cmd/backfill-company-type-hint for the one-off pass that re-enriches a
// curated company's EXISTING postings once it is added here.
//
// Curated by hand from public knowledge of each company's business model — there is no
// open, structured dataset of IT outsourcing/outstaffing companies to import instead
// (Clutch.co, GoodFirms and The Manifest carry the right categorization but no public
// API; Crunchbase's category is too broad and, since 2025, no longer free). Extend this
// map by adding an entry and, if the company is new to the set, running
// cmd/backfill-company-type-hint once.
//
// Keyed by the CATALOGUE's dominant slug for the company (normalize.CompanySlug of its
// most common listing), not every regional/legal-entity variant a source may emit
// separately (e.g. EPAM also appears under smaller per-country slugs) — collapsing those
// is cmd/merge-companies' job, not this map's.
var CompanyTypeHints = map[string]string{
	// outsource: the vendor's own team manages and delivers whole projects/products for
	// external clients.
	"solvd":          "outsource", // "Provides AI-native consulting and global software engineering services."
	"bairesdev":      "outsource", // sources/bairesdev.yml: "a Latin-American software-outsourcing company"
	"epam-systems":   "outsource",
	"grid-dynamics":  "outsource",
	"globant":        "outsource",
	"endava":         "outsource",
	"dataart":        "outsource",
	"n-ix":           "outsource",
	"ciklum":         "outsource",
	"softserve":      "outsource",
	"itransition":    "outsource",
	"sigma-software": "outsource",
	"intetics":       "outsource",

	// outstaff: places its own engineers to work embedded inside a client's team,
	// day-to-day managed by the client — a talent/staffing marketplace rather than a
	// project vendor.
	"andela": "outstaff",
	"toptal": "outstaff",
	"turing": "outstaff",
}

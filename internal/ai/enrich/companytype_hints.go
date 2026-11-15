package enrich

// CompanyTypeHints maps company_slug to a confirmed vocab.CompanyTypeValues entry for
// well-known outsourcing / staff-augmentation companies whose postings say nothing about
// the employer's business model. See buildSystemPrompt and userPrompt for how the hint
// reaches the prompt; adding a new company means running cmd/backfill-company-type-hint.
//
// Curated by hand. Keyed by the catalogue's dominant slug (normalize.CompanySlug), not by
// regional variants; collapsing those is cmd/merge-companies' job.
// is cmd/merge-companies' job, not this map's.
var CompanyTypeHints = map[string]string{
	// outsource: the vendor's own team delivers whole projects for external clients.
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

	// outstaff: places its engineers inside a client's team, managed by the client.
	"andela": "outstaff",
	"toptal": "outstaff",
	"turing": "outstaff",
}

package skilltag

import "slices"

// nonCorroboratingPhrases are the phrase canonicals that tag on their own but do NOT vouch for
// the gated single-word canonicals in ambiguousWords. Every other phrase match is strong, and a
// strong match rescues a weak one: right for a named product ("Ahrefs" evidences an SEO role),
// wrong for a discipline ("AI-powered marketing automation" is marketing prose, not an AI
// requirement).
var nonCorroboratingPhrases = func() map[string]bool {
	out := map[string]bool{
		// Occupational tools and duties do not establish a software toolchain.
		"lightroom":             true,
		"etix":                  true,
		"data-reconciliation":   true,
		"sop-development":       true,
		"event-ticketing":       true,
		"box-office-management": true,
		"pos-terminals":         true,
		"cctv":                  true,
		"structured-cabling":    true,
		// A design discipline: "AI-powered digital product design" is no AI requirement.
		"product-design": true,

		// An electrician installs "sieci komputerowe" too.
		"networking":                     true,
		"technical-seo":                  true,
		"link-building":                  true,
		"paid-social":                    true,
		"ppc":                            true,
		"demand-generation":              true,
		"lifecycle-marketing":            true,
		"marketing-automation":           true,
		"generative-engine-optimization": true,
		"content-marketing":              true,
		"email-marketing":                true,
		"influencer-marketing":           true,
		"copywriting":                    true,
		"go-to-market":                   true,
		// sales / support, same doctrine as the marketing disciplines: "manage our CRM as an account
		// executive" names the gated word "crm" without evidencing technical involvement with it.
		"account-executive":    true,
		"business-development": true,
		"pipeline-management":  true,
		"cold-outreach":        true,
		"sales-enablement":     true,
		"lead-generation":      true,
		"help-desk":            true,
		"service-desk":         true,
		"ticket-resolution":    true,
		// shop-floor craft. Same doctrine, opposite direction: the term is real, but the posting is a
		// machinist's and design vocabulary (wireframes, sketch) is ordinary prose there. Tag the craft,
		// but do not let it lift the gate.
		"cnc":          true,
		"soldering":    true,
		"oscilloscope": true,
		// media production, same doctrine as the marketing disciplines. Each is a duty a coordinator,
		// intern or product manager lists in passing, so tag the craft but do not let it lift the gate
		// (`spring`, `unity` and `sketch` would otherwise appear on a marketing post).
		"video-editing": true,
		"color-grading": true,
		"storyboarding": true,
		// disciplines and office tools a planner, a PM or a designer names in passing. See the matching
		// block at the end of engineeringPhraseAliases.
		"microsoft-project":        true,
		"business-intelligence":    true,
		"fmea":                     true,
		"design-of-experiments":    true,
		"design-for-manufacturing": true,
		"adobe-creative-suite":     true,
		"clickup":                  true,
		"lucidchart":               true,
		"midjourney":               true,
		// compliance frameworks and certifications. Naming a regime is evidence the posting is SUBJECT to
		// it, never that whoever fills it is technical: a nurse's posting carries HIPAA, a plant's ISO 9001.
		// As strong matches they would lift the gate off neighbouring words (`slack` on a nursing post).
		"nist":      true,
		"fedramp":   true,
		"cmmc":      true,
		"cissp":     true,
		"cism":      true,
		"ccna":      true,
		"ccnp":      true,
		"pci-dss":   true,
		"soc-2":     true,
		"iso-9001":  true,
		"iso-13485": true,
		"iso-26262": true,
		"as9100":    true,
		"hipaa":     true,
		"gdpr":      true,
		"ccpa":      true,
		"lgpd":      true,
		"itar":      true,
		"dfars":     true,
		// German market: a clerk's tools named in a requirement clause ("SAP-Kenntnisse", "Erfahrung mit
		// DATEV"), so the tool tags and does not vouch.
		"sap":      true,
		"sap-fico": true,
		"datev":    true,
		"erp":      true,
		"crm":      true,
		"eplan":    true,
		"knx":      true,
		"itwo":     true,
		"archicad": true,
		"allplan":  true,
		// Dutch market: the clerk's suites, a transit planner's tool, and two regimes; each tags, none
		// vouches.
		"afas":         true,
		"exact-online": true,
		"nmbrs":        true,
		"youforce":     true,
		"hastus":       true,
		"topdesk":      true,
		"nis2":         true,
		"nen-7510":     true,
	}
	// Occupational safety, derived rather than listed, for the reason already stated about naming a
	// regime. "First aid", "corrective action" and "risk assessment" are in every childcare, warehouse,
	// nursing, lifeguard and retail posting, and in retail "corrective action" is HR disciplinary
	// language. Naming a safety practice is evidence the posting is SUBJECT to it, never that whoever
	// fills it is technical. Deriving the set stops the next safety phrase from reopening the
	// false-tag bug (`react`, `sketch`, `crm` on warehouse, nursing and retail postings).
	//
	// Not covered, because neither is a phrase: the EHS platforms (Enablon, Cority are products you
	// administer, like "Ahrefs") and the word aliases osha/epa/nfpa/rcra/hazop, where a regulator's
	// name is not English prose. Both stay corroborating.
	for _, p := range slices.Concat(occupationalSafetyPhraseAliases, industrialMaintenancePhraseAliases, procurementPhraseAliases) {
		out[p.canonical] = true
	}
	return out
}()

// sharedAcronyms resolve in ALL text (jobs and résumés), matched by their exact UPPERCASE surface
// form as a whole word (case-preserved pass), because the lowercase form is excluded as ambiguous
// (`ml` = millilitre). Each value is an existing canonical: an acronym is another alias, never a
// new facet value.
var sharedAcronyms = map[string]string{
	"ML":   "machine-learning",
	"LOTO": "lockout-tagout",
	"OEE":  "overall-equipment-effectiveness",
	"MTBF": "mtbf",
	"MTTR": "mttr",
	"HVAC": "hvac",
	// Request for quotation / proposal. "RFI" is absent: radio-frequency interference.
	"RFQ": "rfx", "RFQs": "rfx", "RFP": "rfx", "RFPs": "rfx", "RFx": "rfx",
}

// resumeAcronyms resolve ONLY when parsing résumés (Parse with WithResumeAcronyms).
// Their uppercase form collides with a non-tech meaning in JOB text — "RAG status"
// (red/amber/green project health) — so tagging them on jobs would corrupt facets;
// in résumés that collision is near-absent. Each value is an existing canonical.
//
// The tier also holds terms a person asserts but job prose cannot be trusted with:
// "Nx" (the JS monorepo tool; job text also means Siemens NX CAD) and "Design System(s)"
// (job text means the verb phrase, "design systems that scale"). Job postings still tag
// them through the qualified phrases in dictionaries_phrases_engineering2.go.
var resumeAcronyms = map[string]string{
	"RAG":            "rag",
	"Nx":             "nx",
	"Design System":  "design-systems",
	"Design Systems": "design-systems",
	// "Prisma" is also Palo Alto's Prisma Cloud and the Spanish/Portuguese for prism, "Retool" an
	// English verb, "product design" an industrial-design duty: job text reaches them only
	// through the qualified phrases in dictionaries_phrases_engineering2.go.
	"Prisma":         "prisma",
	"Retool":         "retool",
	"Product Design": "product-design",
}

// categoryScopedAcronym pairs a canonical with the job categories it may resolve for (see
// WithAcronymCategory): a third tier, for a term ambiguous in job text generally but unambiguous
// within a category. The category (an AI-flavored title was needed to classify into it) stands in
// for corroboration without reopening the collision catalogue-wide.
type categoryScopedAcronym struct {
	canonical         string
	allowedCategories map[string]bool
}

// categoryScopedAcronyms resolve on JOB text only when the caller supplies a category on the
// acronym's own allow-list. RAG collides with "RAG status" in general job text (hence
// resumeAcronyms above), but within ai_engineering/ml_ai postings the collision is negligible
// and the acronym is the dominant spelling.
var categoryScopedAcronyms = map[string]categoryScopedAcronym{
	// MEP also means Member of the European Parliament; qualified physical-system
	// phrases remain available when the title has no construction category.
	"MEP": {canonical: "mep", allowedCategories: map[string]bool{
		"industrial_engineering": true, "engineering_design": true, "architecture": true, "skilled_trades": true,
	}},
	// These abbreviations have unrelated software or business meanings outside industrial roles.
	"RCM": {canonical: "reliability-centered-maintenance", allowedCategories: industrialMaintenanceCategories},
	"TPM": {canonical: "total-productive-maintenance", allowedCategories: industrialMaintenanceCategories},
	"APM": {canonical: "asset-performance-management", allowedCategories: industrialMaintenanceCategories},
	"ППР": {canonical: "preventive-maintenance", allowedCategories: industrialMaintenanceCategories},
	"RAG": {canonical: "rag", allowedCategories: map[string]bool{"ai_engineering": true, "ml_ai": true}},
	// MRP is a maximum retail price in an Indian retail posting and a review panel in a medical one.
	"MRP": {canonical: "mrp", allowedCategories: procurementCategories},
	// Each collides with another meaning in general job text (CSM with Customer Success Manager,
	// PSM/PMP are short enough to be noise, bare SAFe is an English word), but within a posting
	// already classified project_management the collision is negligible.
	"CSM": {canonical: "certified-scrummaster", allowedCategories: map[string]bool{"project_management": true}},
	"PSM": {canonical: "professional-scrum-master", allowedCategories: map[string]bool{"project_management": true}},
	"PMP": {canonical: "pmp", allowedCategories: map[string]bool{"project_management": true}},
	// "SAFe" is the framework's own stylization; the case-sensitive pass matches this exact form.
	// "SAFE" is also listed because ATS feeds that render titles in caps would otherwise miss it.
	"SAFe": {canonical: "safe-agile", allowedCategories: map[string]bool{"project_management": true}},
	"SAFE": {canonical: "safe-agile", allowedCategories: map[string]bool{"project_management": true}},
	// BBS is a Bulletin Board System in general job text and Behaviour-Based Safety
	// inside a posting the title dictionary has already placed in the profession.
	// CSP, CIH and CHMM are deliberately NOT here (the invariant test enforces it): an acronym in
	// this table must resolve to an existing canonical, and those three name credentials, not skills,
	// so they live in `internal/dict/certification` beside PMP and CISSP.
	"BBS": {canonical: "behavior-based-safety", allowedCategories: map[string]bool{"occupational_safety": true}},
}

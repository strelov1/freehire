package skilltag

import "slices"

// nonCorroboratingPhrases are the phrase canonicals that tag on their own but do NOT
// vouch for the gated single-word canonicals in ambiguousWords. Every phrase match is
// otherwise a strong match, and a strong match rescues a weak one — which is right for
// a named product ("Ahrefs" genuinely evidences an SEO role) and wrong for a
// discipline. "AI-powered marketing automation" is marketing prose, not an AI
// requirement, and marketing postings carry that phrasing at scale.
var nonCorroboratingPhrases = func() map[string]bool {
	out := map[string]bool{
		// An electrician installs "sieci komputerowe" too (batch 11).
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
		// sales / support — same doctrine as the marketing disciplines above: "manage our
		// CRM as an account executive" names the gated word "crm" without evidencing any
		// technical involvement with it, and that phrasing recurs at scale in this category.
		"account-executive":    true,
		"business-development": true,
		"pipeline-management":  true,
		"cold-outreach":        true,
		"sales-enablement":     true,
		"lead-generation":      true,
		"help-desk":            true,
		"service-desk":         true,
		"ticket-resolution":    true,
		// batch 4 — shop-floor craft. Same doctrine, opposite direction: the term is real
		// and specific, but the posting carrying it is a machinist's, not an engineer's,
		// and the design vocabulary (wireframes, sketch, prototyping) is ordinary prose
		// there. Tagging the craft is right; letting it lift the gate is how a CNC
		// operator came back tagged with wireframing.
		"cnc":          true,
		"soldering":    true,
		"oscilloscope": true,
		// media production — the same doctrine as the marketing disciplines above. Each is
		// a duty a coordinator, an intern or a product manager lists in passing ("Duties:
		// video editing…", "build storyboards, sketch out flows"), so tagging the craft is
		// right and letting it lift the gate is how `spring`, `unity` and `sketch` came
		// back on a marketing post.
		"video-editing": true,
		"color-grading": true,
		"storyboarding": true,
		// batch 5 — disciplines and office tools a planner, a PM or a designer names in
		// passing. See the matching block at the end of engineeringPhraseAliases.
		"microsoft-project":        true,
		"business-intelligence":    true,
		"fmea":                     true,
		"design-of-experiments":    true,
		"design-for-manufacturing": true,
		"adobe-creative-suite":     true,
		"clickup":                  true,
		"lucidchart":               true,
		"midjourney":               true,
		// batch 4 — compliance frameworks and certifications. Naming a regime is evidence
		// that the posting is SUBJECT to it, never that whoever fills it is technical: a
		// nurse's posting carries HIPAA, an injection-moulding plant's carries ISO 9001,
		// and a defence administrator's carries ITAR. As strong matches they lifted the
		// gate off the words beside them and tagged `slack` on a nursing post.
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
		// batch 6 (German market): a clerk's tools named in a German requirement clause.
		// "SAP-Kenntnisse" and "Erfahrung mit DATEV" are what a Sachbearbeiter or a
		// Steuerfachangestellte posting asks for, so the tool tags and does not vouch.
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
		// batch 7 (Dutch market): the clerk's suites, a transit planner's tool, and two
		// regimes — each tags, none vouches.
		"afas":         true,
		"exact-online": true,
		"nmbrs":        true,
		"youforce":     true,
		"hastus":       true,
		"topdesk":      true,
		"nis2":         true,
		"nen-7510":     true,
	}
	// Occupational safety, by construction rather than by hand, and for a reason this
	// map's own doctrine already states about naming a regime. "First aid", "corrective action" and "risk
	// assessment" are in every childcare, warehouse, nursing, lifeguard and retail
	// posting on the board, and in retail "corrective action" is HR disciplinary language
	// rather than safety work at all. Naming a safety practice is evidence the posting is
	// SUBJECT to it, never that whoever fills it is technical.
	//
	// Measured before it was fixed: a warehouse posting came back carrying `react`,
	// `sketch`, `assembly` and `agile`, a nursing posting carried `react`, a retail
	// posting carried `crm` and `analytics` — the "CNC operator came back tagged with
	// wireframing" bug, reintroduced wholesale. Deriving the set is what stops the next
	// safety phrase from quietly reopening it.
	//
	// Deliberately NOT covered, because neither is a phrase: the EHS platforms (naming
	// Enablon or Cority is naming a product you administer, the same shape as "Ahrefs"
	// evidencing an SEO role) and the word aliases osha/epa/nfpa/rcra/hazop, where a
	// regulator's name is not English prose. Both stay corroborating.
	for _, p := range slices.Concat(occupationalSafetyPhraseAliases, industrialMaintenancePhraseAliases) {
		out[p.canonical] = true
	}
	return out
}()

// sharedAcronyms resolve in ALL text (jobs and résumés). They are matched by their
// exact UPPERCASE surface form as a whole word (case-preserved pass), because their
// lowercase form is deliberately excluded as ambiguous (`ml` = millilitre). The
// uppercase form is unambiguous even in job descriptions. Each value is a canonical
// that already exists in the vocabulary — an acronym is another alias, never a new
// facet value.
var sharedAcronyms = map[string]string{
	"ML":   "machine-learning",
	"LOTO": "lockout-tagout",
	"OEE":  "overall-equipment-effectiveness",
	"MTBF": "mtbf",
	"MTTR": "mttr",
	"HVAC": "hvac",
}

// resumeAcronyms resolve ONLY when parsing résumés (Parse with WithResumeAcronyms).
// Their uppercase form collides with a non-tech meaning in JOB text — "RAG status"
// (red/amber/green project health) — so tagging them on jobs would corrupt facets;
// in résumés that collision is near-absent. Each value is an existing canonical.
var resumeAcronyms = map[string]string{
	"RAG": "rag",
}

// categoryScopedAcronym pairs a canonical with the job categories it may resolve
// for (see WithAcronymCategory) — a third acronym tier, for a term ambiguous in
// job text generally but unambiguous within a specific category. The category
// already evidences an AI-flavored posting (it required an AI-flavored title to
// classify into it), so it substitutes for corroboration without reopening the
// collision catalogue-wide.
type categoryScopedAcronym struct {
	canonical         string
	allowedCategories map[string]bool
}

// categoryScopedAcronyms resolve on JOB text only when the caller supplies a
// category on the acronym's own allow-list. RAG collides with "RAG status"
// (red/amber/green project health) in general job text — hence resumeAcronyms
// above — but within ai_engineering/ml_ai postings that collision is negligible
// and the acronym is the dominant real-world spelling (vs. the spelled-out
// "retrieval augmented generation" phrase).
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
	// Each collides with another meaning in general job text — CSM with Customer
	// Success Manager, PSM/PMP are short enough to be noise, bare SAFe is the common
	// English word — but within a posting already classified project_management the
	// collision is negligible and the acronym is the dominant real-world spelling.
	"CSM": {canonical: "certified-scrummaster", allowedCategories: map[string]bool{"project_management": true}},
	"PSM": {canonical: "professional-scrum-master", allowedCategories: map[string]bool{"project_management": true}},
	"PMP": {canonical: "pmp", allowedCategories: map[string]bool{"project_management": true}},
	// "SAFe" is the framework's own stylization and the dominant real-world spelling;
	// the case-sensitive acronym pass matches this exact form. "SAFE" (all-caps) is
	// also listed: ATS feeds that render whole titles in caps would otherwise miss it.
	"SAFe": {canonical: "safe-agile", allowedCategories: map[string]bool{"project_management": true}},
	"SAFE": {canonical: "safe-agile", allowedCategories: map[string]bool{"project_management": true}},
	// BBS is a Bulletin Board System in general job text and Behaviour-Based Safety
	// inside a posting the title dictionary has already placed in the profession.
	//
	// CSP, CIH and CHMM are deliberately NOT here, and the invariant test is what says
	// so: an acronym in this table must resolve to a canonical that already exists,
	// because an acronym is another alias and never a new facet value. Those three name
	// credentials, not skills, so they live in `internal/dict/certification` beside PMP
	// and CISSP — which is where the design put them for a different reason, and the two
	// arguments agree.
	"BBS": {canonical: "behavior-based-safety", allowedCategories: map[string]bool{"occupational_safety": true}},
}

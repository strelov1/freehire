package vocab

import (
	"regexp"
)

// The two EquityValues, named because the derivation writes them.
const (
	// EquityWithSalary: equity is offered beside cash pay. A salary FIGURE is not
	// required — most postings that offer equity state no figure.
	EquityWithSalary = "salary_and_equity"
	// EquityOnly: the posting states that equity is the whole compensation.
	EquityOnly = "equity_only"
)

// RetiredCategoryManagement is the category value that no longer exists. Saved
// searches, alert subscriptions, shared links and user profiles written before its
// retirement still carry it, so the two places that read stored values know it by
// name: the search filter answers it with role_type=people_manager (the axis it was
// a proxy for), and a profile save drops it from the specializations. Nothing may
// write it again — it is deliberately absent from CategoryValues.
const RetiredCategoryManagement = "management"

// The values of ClearanceLevelValues, named for the dictionary that writes them.
const (
	ClearanceConfidential = "confidential"
	ClearanceSecret       = "secret"
	ClearanceTopSecret    = "top_secret"
	ClearanceTSSCI        = "ts_sci"
	ClearancePublicTrust  = "public_trust"
	ClearanceInterim      = "interim"
	ClearanceOther        = "other"
)

// JobChallengeOther is the JobChallengeValues member that admits a written note. Named
// because the survey's note gate and the vocabulary both depend on the exact string.
const JobChallengeOther = "other"

// JobChallengeJustStarting is the answer that nothing is hard yet. It cannot sit beside
// another answer; the survey rejects the pair.
const JobChallengeJustStarting = "just_starting"

// currencyRE matches a well-formed ISO 4217 currency code.
var currencyRE = regexp.MustCompile(`^[A-Z]{3}$`)

// IsCurrencyCode reports whether s is a well-formed ISO 4217 code. Shape only:
// salary_currency is an open field with no closed dictionary, so a true does not mean
// "this currency exists".
//
// It lives here because its two callers (internal/ingest/screeninganswers,
// internal/candidate/survey) sit in blocks that cannot import each other.
func IsCurrencyCode(s string) bool { return currencyRE.MatchString(s) }

// MaxExperienceYears is the largest years-of-experience figure believed to be a
// statement about the CANDIDATE. Three independent producers write this facet (a
// source adapter's structured field, the description parse in internal/job/jobfacts,
// the enrichment model in internal/ai/enrich), so the bound lives here.
//
// "N years of experience" in a posting can describe the candidate, the employer or the
// product, and only the subject tells them apart ("over 40 years of semiconductor
// process control experience, chipmakers rely on KLA" is the company's age).
//
// A figure past this is DROPPED, never clamped: a clamp would state a requirement the
// posting never made, while an absent field correctly says "not stated".
//
// 25 is the evidence-based line. At and below it postings really do say this about the
// candidate ("25+ years of investment management experience"). At 30 and above the
// figures are the EMPLOYER's age ("for over 30 years", "with over 40 years"), and the
// round-number shape of the tail (30, 40, 50, 35, 45) says the same: anniversaries are
// round, careers are not. Measure the served COLUMN, not the enrichment blob: other
// producers write it too.
const MaxExperienceYears = 25

package mailmatch

import "strings"

// Tier records which deterministic signal resolved (or failed to resolve) an
// email. TierAmbiguous and TierNone-with-an-extractable-name are the cases the
// caller hands to the LLM disambiguation tier.
type Tier int

const (
	// TierNone means no deterministic candidate — leave unlinked (or ask the LLM
	// when a company name was extracted but matched nothing tracked).
	TierNone Tier = iota
	// TierThread means the email continues a thread already linked to an application.
	TierThread
	// TierName means the extracted company name matched exactly one application.
	TierName
	// TierAmbiguous means the extracted company name matched more than one application.
	TierAmbiguous
)

const (
	confidenceThread = 1.0
	confidenceName   = 0.9
)

// Email is the minimal projection of an inbox email the matcher reads.
type Email struct {
	ThreadID string
	FromName string
	Subject  string
}

// Candidate is one of the caller's open applications the email might belong to,
// carrying the company display name and the thread ids already linked to it.
type Candidate struct {
	JobID     int64
	Company   string
	ThreadIDs []string
}

// Match is the resolution result: the resolved application (0 when unresolved),
// a confidence in [0,1], and the tier that produced it.
type Match struct {
	JobID      int64
	Confidence float64
	Tier       Tier
}

// Resolve runs the deterministic cascade — thread continuity, then a unique
// company-name match against the candidates — returning the best result. An
// ambiguous or empty name resolves to no application, signalling the caller to
// fall through to the LLM or leave the email unlinked.
func Resolve(email Email, candidates []Candidate) Match {
	if email.ThreadID != "" {
		for _, c := range candidates {
			for _, tid := range c.ThreadIDs {
				if tid == email.ThreadID {
					return Match{JobID: c.JobID, Confidence: confidenceThread, Tier: TierThread}
				}
			}
		}
	}

	company := ExtractCompany(email.FromName, email.Subject)
	if company == "" {
		return Match{Tier: TierNone}
	}

	var matchedID int64
	var matches int
	for _, c := range candidates {
		if normalizeForMatch(c.Company) == company {
			matches++
			matchedID = c.JobID
		}
	}
	switch matches {
	case 1:
		return Match{JobID: matchedID, Confidence: confidenceName, Tier: TierName}
	case 0:
		return Match{Tier: TierNone}
	default:
		return Match{Tier: TierAmbiguous}
	}
}

// normalizeForMatch reduces a candidate company display name to the same form
// ExtractCompany produces (lowercased, legal-suffix stripped) so the two compare.
func normalizeForMatch(company string) string {
	s := strings.ToLower(strings.TrimSpace(company))
	return strings.TrimSpace(stripFirstSuffix(s, legalSuffixes))
}

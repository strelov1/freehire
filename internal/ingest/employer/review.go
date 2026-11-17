package employer

import "github.com/strelov1/freehire/internal/job/logodomain"

type ReviewReason string

const (
	ReviewUnknownWebsite ReviewReason = "unknown_website"
	ReviewDomainMismatch ReviewReason = "domain_mismatch"
	ReviewAcademicDomain ReviewReason = "academic_domain"
	ReviewManual         ReviewReason = "manual_review"
)

// PendingClaim explains the current domain check, not a historical verification attempt.
type PendingClaim struct {
	Account
	Website      string
	ReviewReason ReviewReason
}

func domainReviewReason(website, workEmail string) ReviewReason {
	knownDomain := logodomain.Domain(website)
	workDomain := emailDomain(workEmail)
	switch {
	case knownDomain == "":
		return ReviewUnknownWebsite
	case knownDomain != workDomain:
		return ReviewDomainMismatch
	case isAcademicDomain(workDomain):
		return ReviewAcademicDomain
	default:
		return ""
	}
}

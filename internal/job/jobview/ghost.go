package jobview

// Ghost is the served ghost signal: a hedged level, the criteria that produced it,
// and the denominator of the scale the interface shows. It carries facts so the UI
// can state them, never a bare accusation.
//
// Contributors is the number of distinct people behind the outcome criteria. It is
// ABSENT below the anonymity gate rather than zeroed or rounded, so no caller can
// forget to redact it (a count of one would identify that applicant to the employer).
//
// Computed on read and never stored: it is time-dependent.
type Ghost struct {
	Level         string   `json:"level"`
	Criteria      []string `json:"criteria"`
	CriteriaTotal int      `json:"criteria_total"`
	Contributors  *int     `json:"contributors,omitempty"`
	ATSCheckedAt  *string  `json:"ats_checked_at,omitempty"`
}

package jobview

// Ghost is the served ghost signal: a hedged level, the criteria that produced it,
// and the denominator of the scale the interface shows. It carries facts so the UI
// can state them, never a bare accusation — the same doctrine as Reality.
//
// Contributors is the number of DISTINCT people behind the outcome criteria, and it
// is ABSENT below the anonymity gate rather than zeroed or rounded. Absence is what
// makes the guarantee structural: with a single witness there is no count to serve,
// so no later caller can forget to redact one. (A count of one would identify that
// applicant to the employer.)
//
// The signal is computed on read and never stored — it is time-dependent, and a
// stored level would go stale sitting still.
type Ghost struct {
	Level         string   `json:"level"`
	Criteria      []string `json:"criteria"`
	CriteriaTotal int      `json:"criteria_total"`
	Contributors  *int     `json:"contributors,omitempty"`
	ATSCheckedAt  *string  `json:"ats_checked_at,omitempty"`
}

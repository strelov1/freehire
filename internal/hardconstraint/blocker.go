package hardconstraint

// This file holds ONLY the served wire shape (Blocker + its BlockerCategory/BlockerSeverity enums),
// so cmd/gen-contracts can generate the TypeScript type from this file alone without
// dragging in the evaluator inputs (JobRequirements/CVEvidence) or logic — the same
// wire-only split as matchanalysis.go vs analyzer.go.

// BlockerCategory names the requirement axis a blocker covers.
type BlockerCategory string

const (
	CategoryExperience       BlockerCategory = "experience"
	CategoryEducation        BlockerCategory = "education"
	CategoryLanguage         BlockerCategory = "language"
	CategoryWorkAuth         BlockerCategory = "work_authorization"
	CategoryLocationWorkMode BlockerCategory = "location_work_mode"
	CategoryCertification    BlockerCategory = "certification"
)

// BlockerSeverity grades how hard a blocker is: legal/binary constraints are hard, fit
// constraints are soft.
type BlockerSeverity string

const (
	SeverityHard   BlockerSeverity = "hard"
	SeverityMedium BlockerSeverity = "medium"
	SeveritySoft   BlockerSeverity = "soft"
)

// Blocker is one evaluated requirement. Met is true when the résumé satisfies it
// (kept so the UI can show a ✓); only Met==false entries count toward the cap.
type Blocker struct {
	Category BlockerCategory `json:"category"`
	Severity BlockerSeverity `json:"severity"`
	ScoreCap int             `json:"score_cap"`
	Reason   string          `json:"reason"`
	Action   string          `json:"action"`
	Met      bool            `json:"met"`
}

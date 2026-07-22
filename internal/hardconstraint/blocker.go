package hardconstraint

// This file holds ONLY the served wire shape (Blocker + its Category/Severity enums),
// so cmd/gen-contracts can generate the TypeScript type from this file alone without
// dragging in the evaluator inputs (JobRequirements/CVEvidence) or logic — the same
// wire-only split as matchanalysis.go vs analyzer.go.

// Category names the requirement axis a blocker covers.
type Category string

const (
	CategoryExperience       Category = "experience"
	CategoryEducation        Category = "education"
	CategoryLanguage         Category = "language"
	CategoryWorkAuth         Category = "work_authorization"
	CategoryLocationWorkMode Category = "location_work_mode"
	CategoryCertification    Category = "certification"
)

// Severity grades how hard a blocker is: legal/binary constraints are hard, fit
// constraints are soft.
type Severity string

const (
	SeverityHard   Severity = "hard"
	SeverityMedium Severity = "medium"
	SeveritySoft   Severity = "soft"
)

// Blocker is one evaluated requirement. Met is true when the résumé satisfies it
// (kept so the UI can show a ✓); only Met==false entries count toward the cap.
type Blocker struct {
	Category Category `json:"category"`
	Severity Severity `json:"severity"`
	ScoreCap int      `json:"score_cap"`
	Reason   string   `json:"reason"`
	Action   string   `json:"action"`
	Met      bool     `json:"met"`
}

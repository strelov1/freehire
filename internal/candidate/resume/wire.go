package resume

// The résumé wire shapes. Only this file is generated into web/src/lib/generated/contracts.ts
// (cmd/gen-contracts); the rules over them stay in owned.go and resume.go.

import "github.com/strelov1/freehire/internal/candidate/resumeextract"

// MetaResponse is GET /me/resume: storage flags, parse status, the candidate's owned
// overrides and the composed structured résumé.
type MetaResponse struct {
	Enabled          bool                      `json:"enabled"`
	Present          bool                      `json:"present"`
	UploadedAt       *string                   `json:"uploaded_at" tstype:"string | null,required"`
	Structured       *resumeextract.Structured `json:"structured" tstype:"Structured | null,required"`
	StructurePending bool                      `json:"structure_pending"`
	ParseStatus      string                    `json:"parse_status,omitempty"`
	ParseDetail      string                    `json:"parse_detail,omitempty"`
	// Contacts is the wire name for historical/API-stability reasons; the Go/DB concept
	// underneath is resume.Owned, which now also carries headline/summary/languages/
	// certifications — see Owned.
	Contacts *Owned `json:"contacts,omitempty"`
}

// Owned is everything about a candidate's résumé that the CANDIDATE edited directly, rather
// than what resume_structured last re-derived from an upload. It is the one owned-overlay
// mechanism for the whole profile, not identity alone: a re-upload regenerates
// headline/summary/languages/certifications/education as wholesale as email/phone.
//
// Deliberately NOT here: Skills (userprofile.Profile.Skills, a CV-independent resource) and
// Experience/Projects (the experience bank, provenance-tracked). Education stays a flat overlay.
//
// Persisted as one JSON blob on users.candidate_contacts; the column keeps its original name.
type Owned struct {
	FullName       string                    `json:"full_name,omitempty"`
	Email          string                    `json:"email,omitempty"`
	Phone          string                    `json:"phone,omitempty"`
	Location       string                    `json:"location,omitempty"`
	Links          []string                  `json:"links,omitempty"`
	Headline       string                    `json:"headline,omitempty"`
	Summary        string                    `json:"summary,omitempty"`
	Languages      []string                  `json:"languages,omitempty"`
	Certifications []string                  `json:"certifications,omitempty"`
	Education      []resumeextract.Education `json:"education,omitempty"`
	// TotalYears is the candidate's own years of experience: a re-upload recomputes the
	// extracted figure wholesale, so a correction must survive. The onboarding wizard saves
	// whatever the candidate leaves in the field.
	TotalYears int `json:"total_years,omitempty"`

	// The five body fields above are owned per field (see ApplyBody); a non-empty value implies
	// ownership. A cleared field has no value left to signal it, so an explicit save carries its
	// own *Set flag, true even while the value is "". Sanitize only turns these on, never off, so
	// an explicit clear survives it. Identity fields need no flag: they are overlaid as one
	// IdentityEmpty()-gated block.
	HeadlineSet       bool `json:"headline_set,omitempty"`
	SummarySet        bool `json:"summary_set,omitempty"`
	LanguagesSet      bool `json:"languages_set,omitempty"`
	CertificationsSet bool `json:"certifications_set,omitempty"`
	EducationSet      bool `json:"education_set,omitempty"`
	// TotalYearsSet is the same explicit-clear signal for a numeric field: zero is a legitimate
	// answer ("less than a year"), and without the flag the CV's own figure would replace it.
	TotalYearsSet bool `json:"total_years_set,omitempty"`
}

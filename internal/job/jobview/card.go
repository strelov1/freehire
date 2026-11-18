package jobview

// Card is the list-row projection of a job: what a card draws, and nothing else.
//
// It lives beside Job because both answer what of a posting is public, and two places
// answering it is how wire shapes drift. Card is a strict subset of Job's fields with
// the same names and derivations.
//
// It has no description: the tracking listing's query does not read one, and the text
// dominated the response size for something no list row renders.
type Card struct {
	// ApplicationDeadline is the source-stated last application day (YYYY-MM-DD), inclusive.
	ApplicationDeadline *string `json:"application_deadline,omitempty"`
	PublicSlug          string  `json:"public_slug"`
	Title               string  `json:"title"`
	Company             string  `json:"company"`
	ClosedAt            *string `json:"closed_at"`
	// Location is the posting's own location line, which tells one employer's postings
	// in different cities apart on a row whose flags all name the same country.
	Location string `json:"location,omitempty"`
	// The stated facets of the tag row. Empty means unstated and the row omits it.
	WorkMode       string   `json:"work_mode,omitempty"`
	Seniority      string   `json:"seniority,omitempty"`
	EmploymentType string   `json:"employment_type,omitempty"`
	Countries      []string `json:"countries,omitempty"`
	Regions        []string `json:"regions,omitempty"`
	Skills         []string `json:"skills,omitempty"`
	Collections    []string `json:"collections,omitempty"`
	// AIInterviewReports mirrors Job.AIInterviewReports: omitted at zero, since the
	// badge is the count.
	AIInterviewReports int32 `json:"ai_interview_reports,omitempty"`
	// PostedAt is the effective posting date: the stated date, or the row's creation
	// when the source gave none.
	PostedAt *string `json:"posted_at,omitempty"`
	// Blurb is the line under the title: the enrichment's summary, or the opening of
	// the description with tags stripped. Cut server-side to about 220 characters.
	Blurb string `json:"blurb,omitempty"`
	// Ghost mirrors Job.Ghost: computed at read time by ClassifyGhost, never stored.
	// Omitted (nil) when there is nothing to say. Unlike Job, Card carries no Reality
	// field; Reality is only an internal ingredient of ClassifyGhost here.
	Ghost *Ghost `json:"ghost,omitempty"`
}

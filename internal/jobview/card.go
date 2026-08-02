package jobview

// Card is the list-row projection of a job: what a card draws, and nothing else.
//
// It lives beside Job rather than in the package that lists, because both answer the same
// question — what of a posting is public — and answering it in two places is how the wire
// shapes drift. Card is a strict subset of Job's fields, with the same names and the same
// derivations, so a row and a detail page cannot disagree about a job's work mode or where it
// is open.
//
// The difference is what is absent. Card has no description, and the tracking listing's query
// does not read one: over 500 applications the descriptions were 2.37 MB of a 2.83 MB response
// and 13 ms of a 40 ms query, for text no list row renders. A caller that wants the posting
// reads the one application it opened.
type Card struct {
	PublicSlug string  `json:"public_slug"`
	Title      string  `json:"title"`
	Company    string  `json:"company"`
	ClosedAt   *string `json:"closed_at"`
	// The stated facets of the tag row. Empty means unstated, and the row simply omits it —
	// the same rule the full projection follows.
	WorkMode       string   `json:"work_mode,omitempty"`
	Seniority      string   `json:"seniority,omitempty"`
	EmploymentType string   `json:"employment_type,omitempty"`
	Countries      []string `json:"countries,omitempty"`
	Regions        []string `json:"regions,omitempty"`
	Skills         []string `json:"skills,omitempty"`
	Collections    []string `json:"collections,omitempty"`
	// PostedAt is the effective posting date, the same derivation the full projection makes:
	// the stated date, or the row's creation when the source never gave one.
	PostedAt *string `json:"posted_at,omitempty"`
	// Blurb is the line a row shows under the title — the enrichment's summary, or the
	// opening of the description with its tags stripped. Cut server-side: a row displays
	// about 220 characters, and shipping the whole posting to reach them was 84% of this
	// listing's payload.
	Blurb string `json:"blurb,omitempty"`
}

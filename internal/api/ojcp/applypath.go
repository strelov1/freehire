package ojcp

import (
	"github.com/strelov1/freehire/internal/ingest/applyform"
	"github.com/strelov1/freehire/internal/job/jobview"
)

// Projector holds the facts about THIS DEPLOYMENT that a projection needs and cannot
// discover for itself — the site's own origin, and which ATS providers it can actually
// submit an application to unattended.
//
// Both are configuration, not properties of a posting, which is why they sit here rather
// than being threaded through every call or read from the environment inside a package
// that is meant to stay pure.
type Projector struct {
	// Origin is the absolute site origin (e.g. https://freehire.me) a posting's own page
	// is served from.
	Origin string
	// CanonicalURLProviders names the sources whose stored URL really is the employer's own
	// page — a direct ATS or a company careers site. Only those may fill
	// `official_job_url`, which the schema defines as a domain-verification anchor.
	//
	// Nil means "ask the source taxonomy", which is what NewProjector wires up. It is a
	// field rather than a call inside the projection so this package stays pure and so one
	// taxonomy build serves a whole page of postings.
	CanonicalURLProviders map[string]bool
	// Submittable names the ATS providers this deployment can complete an application on
	// without a person. Today that is Greenhouse alone (fillProviders in
	// internal/api/atsapply); Ashby and Workable join it only where the cloud-agent
	// fallback is enabled and funded, which is a per-deployment fact. Lever is absent
	// even though it HAS a fill path: an invisible hCaptcha defeats it about seven
	// attempts in eight.
	Submittable map[string]bool
}

// ApplyPath is OJCP's description of one way to apply, and the part of the standard this
// catalogue is unusually well placed to answer: `required_fields` and `ats_provider` come
// straight from the form `cmd/capture-apply-form` already stored.
type ApplyPath struct {
	Type                    string   `json:"type"`
	URL                     string   `json:"url,omitempty"`
	ATSProvider             string   `json:"ats_provider,omitempty"`
	RequiredFields          []string `json:"required_fields,omitempty"`
	SupportsAgentSubmission bool     `json:"supports_agent_submission"`
}

// JobPosting projects one catalogue posting, with the application form captured for it if
// there is one. A nil form is the ordinary case for most of the catalogue, not an error.
func (p Projector) JobPosting(j jobview.Job, form *applyform.Form) JobPosting {
	posting := jobPostingFrom(j, p.Origin)
	posting.OfficialJobURL = p.officialJobURL(j)
	posting.ApplyPaths = p.applyPaths(j, form)
	return posting
}

// applyPaths describes how to apply. There is ALWAYS at least one path: the source's own
// page. Omitting the field for a posting whose form we never captured would read as "there
// is no way to apply", which is never true.
func (p Projector) applyPaths(j jobview.Job, form *applyform.Form) []ApplyPath {
	if form == nil {
		return []ApplyPath{{
			Type: "external_redirect",
			URL:  j.URL,
			// Never true here: with no captured form there is nothing for an agent to fill,
			// whatever this deployment can otherwise drive.
			SupportsAgentSubmission: false,
		}}
	}

	return []ApplyPath{{
		Type:                    "ats_direct",
		URL:                     j.URL,
		ATSProvider:             form.Provider,
		RequiredFields:          requiredFieldNames(form),
		SupportsAgentSubmission: p.Submittable[form.Provider],
	}}
}

// requiredFieldNames lists the questions the platform refuses the application without.
//
// A DEMOGRAPHIC question is excluded even when the platform marks it required: the
// platform files those separately from the employer's own questions, OJCP carries them in
// its own eeo-data schema, and listing one here would tell an agent that answering it is
// part of what decides the application.
//
// A field with no label falls back to its opaque platform identifier. Dropping it instead
// would have an agent count one fewer question than the form will actually refuse without.
func requiredFieldNames(form *applyform.Form) []string {
	var names []string
	for _, field := range form.Fields {
		if !field.Required || field.Demographic {
			continue
		}
		if field.Label != "" {
			names = append(names, field.Label)
			continue
		}
		names = append(names, field.ID)
	}
	return names
}

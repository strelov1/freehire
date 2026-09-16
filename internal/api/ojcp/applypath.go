package ojcp

import (
	"strings"

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
	// NIL MEANS NOTHING IS CANONICAL: a zero-value Projector publishes no official_job_url
	// at all. That is the safe direction (we never vouch for a link we cannot vouch for) but
	// it is not a default — NewProjector resolves the set from the source taxonomy, and
	// anything built by hand must supply it or accept the silence.
	//
	// It is a field rather than a call inside the projection so this package stays pure, and
	// so one taxonomy build serves a whole page of postings.
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
		URL:                     applyEntryPoint(j),
		ATSProvider:             form.Provider,
		RequiredFields:          requiredFieldNames(form),
		SupportsAgentSubmission: p.Submittable[form.Provider] && !demandsAnotherUpload(form),
	}}
}

// applyFormPath is the suffix a platform's application form lives under, where it is not
// on the posting page itself. The schema calls an apply path's URL "the application entry
// point", so a platform that separates the two must be followed.
//
// Lever is the one we know about, and we know because this repo paid for it: a live
// auto-apply attempt parked as `unrecognized_form_layout` on a page that loaded fine and
// simply had no form on it. The authoritative copy is `layouts` in internal/api/atsapply,
// which cannot be imported here — it is unexported and would drag chromedp into a package
// that must stay pure — so this is a second copy on purpose. Adding a platform there means
// adding it here.
var applyFormPath = map[string]string{
	"lever": "/apply",
}

// applyEntryPoint is where an agent (or a candidate) actually finds the form.
func applyEntryPoint(j jobview.Job) string {
	suffix, separate := applyFormPath[j.Source]
	if !separate || j.URL == "" {
		return j.URL
	}

	// The tag jobview appends is a query parameter, so the suffix belongs before it.
	base, query, hasQuery := strings.Cut(j.URL, "?")
	base = strings.TrimSuffix(base, "/")
	if strings.HasSuffix(base, suffix) {
		return j.URL
	}
	if !hasQuery {
		return base + suffix
	}
	return base + suffix + "?" + query
}

// demandsAnotherUpload reports whether the form requires a file that is not the résumé.
//
// Such a field is never resolved — atsapply refuses it and parks the whole attempt before
// the provider is even consulted — so `supports_agent_submission` must be false however
// capable the provider otherwise is.
//
// The rule here is deliberately STRICTER than atsapply's own `isResumeField`, which also
// accepts résumé words appearing anywhere in the label. This one takes only the identifier,
// so a field atsapply would have resolved may still be counted against us. The asymmetry is
// the safe direction: reporting false for a form we could actually submit costs an agent a
// missed opportunity, while reporting true for one we cannot costs a candidate a daily
// tailoring turn on an attempt that parks.
func demandsAnotherUpload(form *applyform.Form) bool {
	for _, field := range form.Fields {
		if field.Type != applyform.TypeFile || !field.Required {
			continue
		}
		if !isResumeUpload(field) {
			return true
		}
	}
	return false
}

// isResumeUpload recognises the one file this system can actually supply. Ashby keys its
// built-ins as `_systemfield_<name>`, so the prefix is stripped before comparing — the same
// allowance atsapply's own résumé check makes.
func isResumeUpload(field applyform.Field) bool {
	id := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(field.ID)), "_systemfield_")
	return id == "resume" || id == "cv"
}

// requiredFieldNames lists what a candidate must supply for the application to be accepted:
// the standard controls every application demands (name, contact details, CV) plus the
// employer's own questions the platform will not submit without.
//
// It goes through Form.ForDisplay rather than filtering Field values itself. That reader
// already knows which controls a person can answer at all — it drops the platform's hidden
// fields (a real Greenhouse form requires "Longitude"), the mid-form text blocks, the
// consent boilerplate and the equal-opportunity survey, flattens labels authored as HTML,
// and names a question once where the platform spread it over several controls. A second
// copy of those rules here would be a second thing to keep current, and the first rule
// forgotten would publish "Longitude" as something to answer.
//
// Basics are listed unconditionally: they are what every application demands, so their
// per-control Required flag is not the question.
func requiredFieldNames(form *applyform.Form) []string {
	display := form.ForDisplay()

	names := make([]string, 0, len(display.Basics)+len(display.Questions))
	names = append(names, display.Basics...)
	for _, question := range display.Questions {
		if question.Required {
			names = append(names, question.Text)
		}
	}
	if len(names) == 0 {
		return nil
	}
	return names
}

package ojcp

import (
	"strings"

	"github.com/strelov1/freehire/internal/ingest/applyform"
	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/job/jobview"
)

// Projector holds the facts about THIS DEPLOYMENT that a projection needs: the site's own
// origin, and which ATS providers it can submit an application to unattended. They are
// configuration, kept here so the package stays pure.
type Projector struct {
	// Origin is the absolute site origin (e.g. https://freehire.me) a posting's own page
	// is served from.
	Origin string
	// CanonicalURLProviders names the sources whose stored URL really is the employer's own
	// page — a direct ATS or a company careers site. Only those may fill
	// `official_job_url`, which the schema defines as a domain-verification anchor.
	//
	// NIL MEANS NOTHING IS CANONICAL: a zero-value Projector publishes no official_job_url
	// (safe, but not a default): NewProjector resolves the set from the source taxonomy, and
	// anything built by hand must supply it.
	//
	// It is a field so this package stays pure and one taxonomy build serves a whole page.
	CanonicalURLProviders map[string]bool
	// Submittable names the ATS providers this deployment can complete an application on
	// without a person (fillProviders in internal/api/atsapply). Ashby and Workable join only
	// where the cloud-agent fallback is enabled and funded. Lever is absent despite having a
	// fill path: an invisible hCaptcha defeats it most of the time.
	Submittable map[string]bool
}

// The apply path types this provider emits, from the standard's own enum. ApplyPathTypes
// is what the manifest declares, so declaration and projection cannot drift apart.
const (
	pathATSDirect        = "ats_direct"
	pathExternalRedirect = "external_redirect"
	pathPlatformNative   = "platform_native"
)

// ApplyPathTypes is every apply path type JobPosting can emit.
var ApplyPathTypes = []string{pathATSDirect, pathExternalRedirect, pathPlatformNative}

// ApplyPath is OJCP's description of one way to apply; `required_fields` and `ats_provider`
// come straight from the form `cmd/capture-apply-form` stored.
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
	posting.OfficialJobURL = sources.EmployerURL(p.CanonicalURLProviders, j.Source, j.URL)
	if j.SourceURL != "" {
		// A form captured from the original source says nothing about a separately
		// verified destination's fields, ATS provider or submission capability.
		form = nil
		posting.OfficialJobURL = ""
	}
	posting.ApplyPaths = p.applyPaths(j, form, posting.URL)
	return posting
}

// applyPaths describes how to apply. There is ALWAYS at least one path, the source's own
// page: omitting it would read as "there is no way to apply". pageURL is the posting's page
// on this site.
func (p Projector) applyPaths(j jobview.Job, form *applyform.Form, pageURL string) []ApplyPath {
	if j.DirectApply {
		// An employer vacancy taking applications here has no outside page: the way in is
		// this platform's own. Its questions are the employer's, not an ATS's, and only a
		// signed-in candidate can send it, so no agent submits it.
		path := ApplyPath{Type: pathPlatformNative, URL: pageURL}
		if form != nil {
			path.RequiredFields = requiredFieldNames(form)
		}
		return []ApplyPath{path}
	}
	if form == nil {
		return []ApplyPath{{
			Type: pathExternalRedirect,
			URL:  j.URL,
			// Never true here: with no captured form there is nothing for an agent to fill,
			// whatever this deployment can otherwise drive.
			SupportsAgentSubmission: false,
		}}
	}

	return []ApplyPath{{
		Type:                    pathATSDirect,
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
// Lever is the known case: its posting page has no form. The authoritative copy is
// `layouts` in internal/api/atsapply, which cannot be imported (unexported, and would drag
// chromedp into a pure package), so this is a second copy on purpose. Adding a platform
// there means adding it here.
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
// Such a field is never resolved (atsapply parks the attempt before consulting the
// provider), so `supports_agent_submission` must be false however capable the provider is.
//
// Deliberately STRICTER than atsapply's `isResumeField`, which also accepts résumé words
// anywhere in the label; this takes only the identifier. The asymmetry is the safe
// direction: a false negative costs a missed opportunity, a false positive costs a
// candidate a tailoring turn on an attempt that parks.
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
// built-ins as `_systemfield_<name>`, so the prefix is stripped before comparing, as atsapply does.
func isResumeUpload(field applyform.Field) bool {
	id := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(field.ID)), "_systemfield_")
	return id == "resume" || id == "cv"
}

// requiredFieldNames lists what a candidate must supply for the application to be accepted:
// the standard controls every application demands (name, contact details, CV) plus the
// employer's own questions the platform will not submit without.
//
// It goes through Form.ForDisplay rather than filtering Field values itself: that reader
// already drops hidden fields (a real Greenhouse form requires "Longitude"), text blocks,
// consent boilerplate and the equal-opportunity survey, flattens HTML labels, and names a
// multi-control question once. A second copy of those rules would drift.
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

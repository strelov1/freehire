// Package ojcp projects freehire's catalogue into the wire shapes of the Open Job
// Context Protocol (OJCP), the standard agents use to read job data.
//
// Everything here is a pure projection: each function takes an already-loaded domain
// value and returns a struct, with no database, connection or HTTP framework. The REST
// handlers and the MCP server share it, and each projection is tested against OJCP's
// published JSON Schemas vendored under testdata/schemas (its README says what a green
// schema check does NOT prove).
package ojcp

import (
	"strings"
	"time"

	"github.com/strelov1/freehire/internal/dict/vocab"
	"github.com/strelov1/freehire/internal/job/jobview"
	"github.com/strelov1/freehire/internal/platform/htmltext"
)

// Version is the OJCP specification version this projection targets. It travels in every
// response envelope so an agent can tell which spec produced what it received.
const Version = "0.1"

// validThroughBufferDays is how far past the freshest "still open" evidence an open
// posting is estimated to stay valid. Keep it equal to VALID_THROUGH_BUFFER_DAYS in
// web/src/lib/seo.ts so Google and agents see the same expiry.
const validThroughBufferDays = 30

// JobPosting is OJCP's posting shape: schema.org/JobPosting plus the protocol's agent-facing
// additions. Field names follow the standard's JSON, not Go convention.
type JobPosting struct {
	Context []string `json:"@context,omitempty"`
	Type    string   `json:"@type,omitempty"`

	OJCPID     string   `json:"ojcp_id"`
	Title      string   `json:"title"`
	Employer   Employer `json:"employer"`
	DatePosted string   `json:"datePosted,omitempty"`

	ValidThrough string `json:"validThrough,omitempty"`
	// URL is our own page for the posting. OfficialJobURL is the employer's own, filled by
	// Projector.JobPosting only for a source that publishes one — see officialJobURL.
	// Keeping both carries source attribution into this channel.
	URL            string `json:"url,omitempty"`
	OfficialJobURL string `json:"official_job_url,omitempty"`

	Description string `json:"description,omitempty"`

	EmploymentType  string  `json:"employmentType,omitempty"`
	ExperienceLevel string  `json:"experienceLevel,omitempty"`
	RemotePolicy    string  `json:"remote_policy,omitempty"`
	JobLocation     Places  `json:"jobLocation,omitempty"`
	BaseSalary      *Salary `json:"baseSalary,omitempty"`

	// SkillsRequired and SkillsPreferred are split by what the posting itself demanded —
	// see skills.go for why the skills facet cannot fill either.
	SkillsRequired  []string `json:"skills_required,omitempty"`
	SkillsPreferred []string `json:"skills_preferred,omitempty"`
	// AgentNotes carries the posting-reality verdict; see agentnotes.go.
	AgentNotes string `json:"agent_notes,omitempty"`
	// ApplyPaths is filled by Projector.JobPosting, which knows what this deployment can
	// submit to; jobPostingFrom on its own leaves it nil.
	ApplyPaths []ApplyPath `json:"apply_paths,omitempty"`
}

// Salary is OJCP's compensation block. It is a pointer on JobPosting: a zeroed block would
// read as a posting that pays nothing.
type Salary struct {
	Type     string   `json:"@type,omitempty"`
	Currency string   `json:"currency,omitempty"`
	MinValue *float64 `json:"minValue,omitempty"`
	MaxValue *float64 `json:"maxValue,omitempty"`
	UnitText string   `json:"unitText,omitempty"`
}

// remotePolicy translates our work-mode vocabulary into OJCP's closed enum; an untranslated
// value fails the whole posting. An unknown work mode omits the field: OJCP has no
// "unknown", and `flexible` is a real arrangement, not a stand-in.
var remotePolicy = map[string]string{
	"remote": "remote",
	"hybrid": "hybrid",
	"onsite": "on_site",
}

// experienceLevel translates the two seniority levels whose meaning is identical in both
// vocabularies. Everything else is emitted verbatim, which the schema permits (plain
// string). Folding intern/junior or staff/principal together would make a principal search
// return ordinary seniors; an unrecognised word is visibly unknown, a wrong one is not.
var experienceLevel = map[string]string{
	"middle":  "mid",
	"c_level": "executive",
}

// salaryUnit translates our pay-period vocabulary into OJCP's closed enum.
var salaryUnit = map[string]string{
	"year":  "YEAR",
	"month": "MONTH",
	"hour":  "HOUR",
	// OJCP's enum has no daily rate. The empty value records that gap (so the vocabulary
	// guard sees a decided omission) and salaryFrom drops the block.
	"day": "",
}

// Employer is OJCP's employer block. Only `name` is required, but OJCPEmployerID is the
// only way an agent learns the employer_id that `get_employer_context` takes.
type Employer struct {
	Type string `json:"@type,omitempty"`
	Name string `json:"name"`
	// OJCPEmployerID is our company slug, the key /companies/<slug> is served under.
	OJCPEmployerID string `json:"ojcp_employer_id,omitempty"`
}

// jobPostingFrom projects one catalogue posting into OJCP's shape. origin is the absolute
// site origin (e.g. https://freehire.me) the posting's own page is served from.
//
// A field the posting does not state is omitted, never emitted empty: `datePosted` is
// `format: date`, so an empty string is invalid and reads as a date we claim to know.
func jobPostingFrom(j jobview.Job, origin string) JobPosting {
	posting := JobPosting{
		Context: []string{"https://schema.org"},
		Type:    "JobPosting",
		OJCPID:  j.PublicSlug,
		Title:   j.Title,
		Employer: Employer{
			Type:           "Organization",
			Name:           j.Company,
			OJCPEmployerID: j.CompanySlug,
		},
		DatePosted: datePosted(j),

		ValidThrough: validThrough(j),
		// The stored description is markup; OJCP's field is plain text with no format parameter.
		Description:     htmltext.ToText(j.Description),
		EmploymentType:  j.Enrichment.EmploymentType,
		ExperienceLevel: seniorityFor(j.Enrichment.Seniority),
		RemotePolicy:    remotePolicy[j.WorkMode],
		JobLocation:     jobLocationFor(j),
		BaseSalary:      salaryFrom(j),
		AgentNotes:      agentNotesFor(j),
	}
	posting.SkillsRequired, posting.SkillsPreferred = skillsFor(j)
	// Without an origin the URL would be a relative path, which `format: uri` rejects.
	if j.PublicSlug != "" && origin != "" {
		posting.URL = strings.TrimSuffix(origin, "/") + "/jobs/" + j.PublicSlug
	}
	return posting
}

// seniorityFor translates where the vocabularies agree exactly and passes ours through
// otherwise. See experienceLevel.
func seniorityFor(ours string) string {
	if translated, ok := experienceLevel[ours]; ok {
		return translated
	}
	return ours
}

// salaryFrom builds the compensation block, or nil when the posting states no pay at all.
// A stated bound is carried even when the other bound is missing.
func salaryFrom(j jobview.Job) *Salary {
	e := j.Enrichment
	if e.SalaryMin == nil && e.SalaryMax == nil {
		return nil
	}

	// A figure without its period or currency is wrong, not partial: "day" has no OJCP unit,
	// so EUR 400/day published bare would read as an annual salary.
	unit := salaryUnit[e.SalaryPeriod]
	if unit == "" || !vocab.IsCurrencyCode(e.SalaryCurrency) {
		return nil
	}

	return &Salary{
		Type:     "MonetaryAmountDistribution",
		Currency: e.SalaryCurrency,
		MinValue: asFloat(e.SalaryMin),
		MaxValue: asFloat(e.SalaryMax),
		UnitText: unit,
	}
}

func asFloat(v *int) *float64 {
	if v == nil {
		return nil
	}
	f := float64(*v)
	return &f
}

// datePosted is when the employer published the posting; the schema requires it.
//
// No fallback: jobview already serves PostedAt as `effectivePosted` (creation date when the
// source stated none or a future one). An empty value means something upstream is wrong,
// and inventing a date would hide it from the validator.
func datePosted(j jobview.Job) string {
	return calendarDate(j.PostedAt)
}

// calendarDate renders one of our RFC3339 timestamps as the YYYY-MM-DD the schema's
// `format: date` requires. An absent or unparseable timestamp yields "", which the
// caller omits.
func calendarDate(ts *string) string {
	if ts == nil || *ts == "" {
		return ""
	}
	t, err := time.Parse(time.RFC3339, *ts)
	if err != nil {
		return ""
	}
	return t.UTC().Format(time.DateOnly)
}

// validThrough estimates how long an open posting stays valid, from the freshest evidence
// that it is still live. Most sources carry no expiry date of their own.
//
// A closed posting is valid through the moment it closed, with no buffer.
func validThrough(j jobview.Job) string {
	if j.ClosedAt != nil && *j.ClosedAt != "" {
		return calendarDate(j.ClosedAt)
	}

	evidence := firstPresent(j.LastSeenAt, j.PostedAt, j.CreatedAt)
	if evidence == nil {
		return ""
	}
	t, err := time.Parse(time.RFC3339, *evidence)
	if err != nil {
		return ""
	}
	return t.UTC().AddDate(0, 0, validThroughBufferDays).Format(time.DateOnly)
}

func firstPresent(candidates ...*string) *string {
	for _, c := range candidates {
		if c != nil && *c != "" {
			return c
		}
	}
	return nil
}

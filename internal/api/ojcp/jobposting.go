// Package ojcp projects freehire's catalogue into the wire shapes of the Open Job
// Context Protocol (OJCP), the standard agents use to read job data.
//
// Everything here is a PURE PROJECTION: each function takes an already-loaded domain
// value and returns a struct. Nothing in this package reads the database, opens a
// connection, or knows the HTTP framework exists. That is what lets the REST handlers
// and the MCP server share one implementation instead of two that drift, and what lets
// every projection be tested against OJCP's own published JSON Schemas with no fixtures
// beyond a domain value.
//
// The schemas are vendored under testdata/schemas — see that directory's README for
// their provenance and, more importantly, for the two things a green schema check does
// NOT prove.
package ojcp

import (
	"strings"
	"time"

	"github.com/strelov1/freehire/internal/job/jobview"
)

// Version is the OJCP specification version this projection targets. It travels in every
// response envelope so an agent can tell which spec produced what it received.
const Version = "0.1"

// validThroughBufferDays is how far past the freshest "still open" evidence an open
// posting is estimated to stay valid. It mirrors VALID_THROUGH_BUFFER_DAYS in
// web/src/lib/seo.ts deliberately: the same posting is described to Google by that
// markup and to an agent by this projection, and two channels disagreeing about when
// one posting expires is worse than either estimate being imperfect.
const validThroughBufferDays = 30

// JobPosting is OJCP's posting shape — schema.org/JobPosting with the protocol's own
// agent-facing additions. Field names follow the standard's JSON, not Go convention,
// because the wire shape is not ours to name.
type JobPosting struct {
	Context []string `json:"@context,omitempty"`
	Type    string   `json:"@type,omitempty"`

	OJCPID     string   `json:"ojcp_id"`
	Title      string   `json:"title"`
	Employer   Employer `json:"employer"`
	DatePosted string   `json:"datePosted,omitempty"`

	ValidThrough string `json:"validThrough,omitempty"`
	// URL is our own page for the posting; OfficialJobURL is the employer's own. Keeping
	// both is what carries source attribution into this channel (OJCP RFC 0002).
	URL            string `json:"url,omitempty"`
	OfficialJobURL string `json:"official_job_url,omitempty"`

	EmploymentType  string  `json:"employmentType,omitempty"`
	ExperienceLevel string  `json:"experienceLevel,omitempty"`
	RemotePolicy    string  `json:"remote_policy,omitempty"`
	BaseSalary      *Salary `json:"baseSalary,omitempty"`

	SkillsRequired []string `json:"skills_required,omitempty"`
	// AgentNotes carries the posting-reality verdict — see agentnotes.go for why it lives
	// in a free-text field and how carefully it is worded.
	AgentNotes string `json:"agent_notes,omitempty"`
}

// Salary is OJCP's compensation block. It is a POINTER on JobPosting: a zeroed block would
// read to an agent as a posting that pays nothing, which is a claim no posting made.
type Salary struct {
	Type     string   `json:"@type,omitempty"`
	Currency string   `json:"currency,omitempty"`
	MinValue *float64 `json:"minValue,omitempty"`
	MaxValue *float64 `json:"maxValue,omitempty"`
	UnitText string   `json:"unitText,omitempty"`
}

// remotePolicy translates our work-mode vocabulary into OJCP's, which is a CLOSED enum —
// an untranslated value would fail the whole posting, not just the field.
//
// A work mode we do not hold omits the field. OJCP has no "unknown", and its `flexible`
// is a real arrangement an employer states, not a stand-in for one we could not read.
var remotePolicy = map[string]string{
	"remote": "remote",
	"hybrid": "hybrid",
	"onsite": "on_site",
}

// experienceLevel translates the two seniority levels whose meaning is identical in both
// vocabularies. Everything else is emitted verbatim, which the schema permits (the field
// is a plain string and the standard's own description invites additional values).
//
// The alternative — folding intern and junior into `entry`, staff and principal into
// `senior` — would make an agent's search for a principal role return ordinary senior
// ones. Four distinct levels collapsing into two is a worse answer than a word the agent
// may not recognise, because an unrecognised word is visibly unknown while a wrong one is
// not.
var experienceLevel = map[string]string{
	"middle":  "mid",
	"c_level": "executive",
}

// salaryUnit translates our pay-period vocabulary into OJCP's, which is a closed enum.
// Our "day" has no counterpart there; such a posting keeps its figures and omits the
// period rather than being rounded into a week.
var salaryUnit = map[string]string{
	"year":  "YEAR",
	"month": "MONTH",
	"hour":  "HOUR",
}

// Employer is OJCP's employer block. Only `name` is required by the schema.
type Employer struct {
	Type string `json:"@type,omitempty"`
	Name string `json:"name"`
}

// JobPostingFrom projects one catalogue posting into OJCP's shape. origin is the absolute
// site origin (e.g. https://freehire.me) the posting's own page is served from.
//
// A field the posting does not state is OMITTED, never emitted empty: `datePosted` is
// `format: date` in the schema, so an empty string is both invalid and — worse — reads to
// an agent as a date we claim to know.
func JobPostingFrom(j jobview.Job, origin string) JobPosting {
	posting := JobPosting{
		Context:         []string{"https://schema.org"},
		Type:            "JobPosting",
		OJCPID:          j.PublicSlug,
		Title:           j.Title,
		Employer:        Employer{Type: "Organization", Name: j.Company},
		DatePosted:      datePosted(j),
		ValidThrough:    validThrough(j),
		OfficialJobURL:  j.URL,
		EmploymentType:  j.Enrichment.EmploymentType,
		ExperienceLevel: seniorityFor(j.Enrichment.Seniority),
		RemotePolicy:    remotePolicy[j.WorkMode],
		BaseSalary:      salaryFrom(j),
		SkillsRequired:  j.Skills,
		AgentNotes:      agentNotesFor(j),
	}
	if j.PublicSlug != "" {
		posting.URL = strings.TrimSuffix(origin, "/") + "/jobs/" + j.PublicSlug
	}
	return posting
}

// seniorityFor translates where the two vocabularies agree exactly and passes ours through
// where the standard has no word for the level. See experienceLevel for why.
func seniorityFor(ours string) string {
	if translated, ok := experienceLevel[ours]; ok {
		return translated
	}
	return ours
}

// salaryFrom builds the compensation block, or nil when the posting states no pay at all.
// A figure the posting did state is carried even when the other bound or the period is
// missing — a stated minimum is information, and withholding it because the maximum is
// unknown would serve less than the posting says.
func salaryFrom(j jobview.Job) *Salary {
	e := j.Enrichment
	if e.SalaryMin == nil && e.SalaryMax == nil {
		return nil
	}
	return &Salary{
		Type:     "MonetaryAmountDistribution",
		Currency: e.SalaryCurrency,
		MinValue: asFloat(e.SalaryMin),
		MaxValue: asFloat(e.SalaryMax),
		UnitText: salaryUnit[e.SalaryPeriod],
	}
}

func asFloat(v *int) *float64 {
	if v == nil {
		return nil
	}
	f := float64(*v)
	return &f
}

// datePosted is when the employer published the posting, falling back to when this
// catalogue first recorded it. The schema REQUIRES the field, and the fallback is the
// honest lower bound: a source that states no publication date still gives us the day we
// first saw the posting, which is never later than the day it appeared.
//
// A posting with neither yields "", which the schema rejects. That is deliberate: no
// stored row lacks both, so an empty value means something upstream is wrong, and inventing
// a date to make the validator quiet would hide it.
func datePosted(j jobview.Job) string {
	return calendarDate(firstPresent(j.PostedAt, j.CreatedAt))
}

// calendarDate renders one of our RFC3339 timestamps as the YYYY-MM-DD the schema's
// `format: date` requires. An absent or unparseable timestamp yields "", which the
// caller omits — a date we could not read must not reach an agent as one we could.
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
// that it is still live. Most sources carry no real expiry date of their own, so without
// an estimate an agent has nothing to age a posting by.
//
// A CLOSED posting is valid through the moment it closed: that is a fact, not an estimate,
// and it must not be pushed into the future by a buffer.
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

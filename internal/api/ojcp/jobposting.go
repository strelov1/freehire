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

	"github.com/strelov1/freehire/internal/dict/vocab"
	"github.com/strelov1/freehire/internal/job/jobview"
	"github.com/strelov1/freehire/internal/platform/htmltext"
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
	// URL is our own page for the posting. OfficialJobURL is the employer's own, filled by
	// Projector.JobPosting only for a source that publishes one — see officialJobURL.
	// Keeping both is what carries source attribution into this channel (OJCP RFC 0002).
	URL            string `json:"url,omitempty"`
	OfficialJobURL string `json:"official_job_url,omitempty"`

	Description string `json:"description,omitempty"`

	EmploymentType  string  `json:"employmentType,omitempty"`
	ExperienceLevel string  `json:"experienceLevel,omitempty"`
	RemotePolicy    string  `json:"remote_policy,omitempty"`
	JobLocation     *Place  `json:"jobLocation,omitempty"`
	BaseSalary      *Salary `json:"baseSalary,omitempty"`

	// SkillsRequired and SkillsPreferred are split by what the posting itself demanded —
	// see skills.go for why the skills facet cannot fill either.
	SkillsRequired  []string `json:"skills_required,omitempty"`
	SkillsPreferred []string `json:"skills_preferred,omitempty"`
	// AgentNotes carries the posting-reality verdict — see agentnotes.go for why it lives
	// in a free-text field and how carefully it is worded.
	AgentNotes string `json:"agent_notes,omitempty"`
	// ApplyPaths is filled by Projector.JobPosting, which knows what this deployment can
	// submit to; jobPostingFrom on its own leaves it nil.
	ApplyPaths []ApplyPath `json:"apply_paths,omitempty"`
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
// Our "day" has no counterpart there, and salaryFrom drops the whole block rather than
// publishing figures with no period — a daily rate read as an annual one is a wrong
// answer, not a partial one.
var salaryUnit = map[string]string{
	"year":  "YEAR",
	"month": "MONTH",
	"hour":  "HOUR",
	// OJCP's enum has no daily rate. The empty value RECORDS that gap rather than leaving
	// the key absent, so the vocabulary guard can tell a decided omission from a forgotten
	// one — and salaryFrom drops the whole block, because a daily rate read as an annual
	// one is a wrong answer, not a partial one.
	"day": "",
}

// Employer is OJCP's employer block. Only `name` is required by the schema, but
// OJCPEmployerID is what makes the third tool reachable: `get_employer_context` takes an
// employer_id, and a posting is the only place an agent can learn one. Without it, an agent
// that found a job has no key to ask about the company.
type Employer struct {
	Type string `json:"@type,omitempty"`
	Name string `json:"name"`
	// OJCPEmployerID is our company slug — the same key /companies/<slug> is served under,
	// so an id an agent stores keeps resolving.
	OJCPEmployerID string `json:"ojcp_employer_id,omitempty"`
}

// jobPostingFrom projects one catalogue posting into OJCP's shape. origin is the absolute
// site origin (e.g. https://freehire.me) the posting's own page is served from.
//
// A field the posting does not state is OMITTED, never emitted empty: `datePosted` is
// `format: date` in the schema, so an empty string is both invalid and — worse — reads to
// an agent as a date we claim to know.
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
		// The stored description is markup. OJCP's field is "Full job description text" and
		// the schema offers no format parameter, so the conversion happens here rather than
		// being left to every agent — which would also make each of them pay several times
		// the tokens to read past the tags.
		Description:     htmltext.ToText(j.Description),
		EmploymentType:  j.Enrichment.EmploymentType,
		ExperienceLevel: seniorityFor(j.Enrichment.Seniority),
		RemotePolicy:    remotePolicy[j.WorkMode],
		JobLocation:     jobLocationFor(j),
		BaseSalary:      salaryFrom(j),
		AgentNotes:      agentNotesFor(j),
	}
	posting.SkillsRequired, posting.SkillsPreferred = skillsFor(j)
	// An origin the deployment never configured would produce "/jobs/<slug>" — a relative
	// path no agent can dereference, and one `format: uri` rejects. Publishing nothing is
	// the better failure: the posting is still reachable through official_job_url.
	if j.PublicSlug != "" && origin != "" {
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

	// A figure without its period or its currency is not a smaller answer, it is a wrong
	// one. Our vocabulary has "day" and OJCP's unitText enum does not, so a EUR 400/day
	// contract published as a bare 400 reads to an agent as an annual salary — it buries a
	// six-figure role at the bottom of a ranking, or drops it against a salary_min filter.
	// A bare number with no currency is uncomparable in the same way.
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

// datePosted is when the employer published the posting. The schema REQUIRES the field.
//
// There is no fallback here because there is nothing left to fall back to: jobview already
// serves PostedAt as `effectivePosted`, which stands the creation date in whenever the
// source stated no publication date (or stated one in the future). Repeating that decision
// would be a branch no read path can reach.
//
// An empty value therefore means the posting has no timestamp at all, which the schema then
// rejects. That is deliberate: no stored row lacks both, so it means something upstream is
// wrong, and inventing a date to quiet the validator would hide it.
func datePosted(j jobview.Job) string {
	return calendarDate(j.PostedAt)
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

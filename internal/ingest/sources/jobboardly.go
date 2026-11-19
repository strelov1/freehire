package sources

import (
	"context"
	"fmt"
	"net/url"
	"path"
	"regexp"
	"strings"
)

// jobboardly adapts Jobboardly (jobboardly.com), a hosted job-board builder: every board is a
// tenant subdomain, <board>.jobboardly.com, often fronted by the board owner's own domain
// (kubernetes-jobs.jobboardly.com serves kubernetesjobs.com). A board is a NICHE job board, not
// an employer — each posting names its own company, and much of a board is backfilled from
// first-party ATS postings (Workday, JazzHR, ...) — so the provider is an aggregator whose board
// is the subdomain, the shape getro has.
//
// Every board serves its whole catalogue as one unpaginated JSON array at /jobs.json, bodies
// inline, so there is no detail request. ?page=2 is ignored and returns the identical array; it
// holds as many postings as the board's sitemap.xml lists.
type jobboardly struct {
	http JSONGetter
}

// jobboardlyListURL is the board's own catalogue. A board on a custom domain redirects it there,
// which the client follows.
const jobboardlyListURL = "https://%s.jobboardly.com/jobs.json"

// jobboardlyBoard is the shape of a tenant label: one DNS label, lowercase.
var jobboardlyBoard = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`)

// NewJobboardly builds the Jobboardly adapter over the given HTTP client.
func NewJobboardly(c JSONGetter) Source { return jobboardly{http: c} }

func (jobboardly) Provider() string { return "jobboardly" }

// jobboardly aggregates postings from many companies (company per posting), so it stays in the
// source facet and its ATS-duplicate copies are suppressed by the cross-source dedup pass. It is
// NOT boardless — the board is the tenant subdomain.
func (jobboardly) Aggregator() {}

// fullBoardListing: /jobs.json is one unpaginated document carrying the board's whole catalogue,
// and a failed or undecodable response fails the crawl. See the fullBoardListing interface for
// the bar.
func (jobboardly) FullBoardListing() {}

// jobboardlyMoney is one salary bound in minor units.
type jobboardlyMoney struct {
	Cents    int64  `json:"cents"`
	Currency string `json:"currency_iso"`
}

// jobboardlyPosting is one element of /jobs.json.
//
// location is the work ARRANGEMENT (onsite/hybrid/remote), not a place; the place is
// company.location, which the posting page renders as the job's location (Booz Allen's
// "Fort Bragg, North Carolina, United States"), and is empty on a remote posting.
// location_limits are the countries a remote posting hires from, as alpha-2 codes, names or
// "Worldwide". application_link is the employer's own apply target and is often an email
// address or a malformed URL, so it never becomes Job.URL; links.self is the posting page.
type jobboardlyPosting struct {
	Title          string   `json:"title"`
	Arrangement    string   `json:"arrangement"`
	Location       string   `json:"location"`
	LocationLimits []string `json:"location_limits"`
	PublishedAt    string   `json:"published_at"`
	Description    struct {
		HTML string `json:"html"`
	} `json:"description"`
	Company struct {
		Name     string `json:"name"`
		Location string `json:"location"`
	} `json:"company"`
	Salary *struct {
		Schedule string           `json:"schedule"`
		Minimum  *jobboardlyMoney `json:"minimum"`
		Maximum  *jobboardlyMoney `json:"maximum"`
	} `json:"salary"`
	Links struct {
		Self string `json:"self"`
	} `json:"links"`
}

func (s jobboardly) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	board := strings.ToLower(strings.TrimSpace(e.Board))
	if !jobboardlyBoard.MatchString(board) {
		return nil, fmt.Errorf("jobboardly: board %q is not a tenant subdomain", e.Board)
	}
	var postings []jobboardlyPosting
	if err := s.http.GetJSON(ctx, fmt.Sprintf(jobboardlyListURL, board), &postings); err != nil {
		return nil, fmt.Errorf("jobboardly: board %s: %w", board, err)
	}
	jobs := make([]Job, 0, len(postings))
	for _, p := range postings {
		if job, ok := p.toJob(); ok {
			jobs = append(jobs, job)
		}
	}
	return jobs, nil
}

// toJob maps a posting to a Job, returning ok=false for one with no posting page (no identity)
// or no company (no slug). The external id is the page's last path segment: the posting's
// title slug plus an 8-hex suffix, and neither half alone resolves the page.
func (p jobboardlyPosting) toJob() (Job, bool) {
	id := jobboardlySlug(p.Links.Self)
	if id == "" || p.Company.Name == "" {
		return Job{}, false
	}
	mode := workplaceTypeMode(p.Location)
	place := strings.TrimSpace(p.Company.Location)
	if place == "" {
		place = joinNonEmpty(p.LocationLimits...)
	}
	job := Job{
		ExternalID:     id,
		URL:            p.Links.Self,
		Title:          p.Title,
		Company:        p.Company.Name,
		Location:       place,
		Description:    sanitizeHTML(p.Description.HTML),
		Remote:         mode == "remote",
		WorkMode:       mode,
		EmploymentType: jobboardlyEmploymentType(p.Arrangement),
		Countries:      countriesFromCodes(p.LocationLimits),
		PostedAt:       parseRFC3339(p.PublishedAt),
	}
	job.SalaryMin, job.SalaryMax, job.SalaryCurrency, job.SalaryPeriod = p.salary()
	return job, true
}

// jobboardlySlug is the last path segment of a posting page URL, or "" for a URL that does not
// name a posting under /jobs/.
func jobboardlySlug(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return ""
	}
	dir, slug := path.Split(strings.TrimSuffix(u.Path, "/"))
	if dir != "/jobs/" || slug == "" {
		return ""
	}
	return slug
}

// salary reads the structured salary: bounds in minor units with an ISO currency and a
// schedule. A schedule freehire has no period for, or bounds in two currencies, states no
// salary rather than a half-qualified one.
func (p jobboardlyPosting) salary() (salaryMin, salaryMax *int, currency, period string) {
	if p.Salary == nil {
		return nil, nil, "", ""
	}
	period = jobboardlySalaryPeriod(p.Salary.Schedule)
	if period == "" {
		return nil, nil, "", ""
	}
	if m := p.Salary.Minimum; m != nil {
		salaryMin, currency = roundSalaryPart(float64(m.Cents)/100), m.Currency
	}
	if m := p.Salary.Maximum; m != nil {
		if currency != "" && m.Currency != currency {
			return nil, nil, "", ""
		}
		salaryMax, currency = roundSalaryPart(float64(m.Cents)/100), m.Currency
	}
	if (salaryMin == nil && salaryMax == nil) || currency == "" {
		return nil, nil, "", ""
	}
	return salaryMin, salaryMax, strings.ToUpper(currency), period
}

// jobboardlySalaryPeriod maps the platform's salary schedule onto vocab.SalaryPeriodValues.
func jobboardlySalaryPeriod(schedule string) string {
	switch schedule {
	case "yearly":
		return "year"
	case "monthly":
		return "month"
	case "daily":
		return "day"
	case "hourly":
		return "hour"
	default:
		return ""
	}
}

// jobboardlyEmploymentType maps the platform's arrangement picklist onto
// vocab.EmploymentTypeValues. "temporary" and "volunteer" have no freehire value and are left
// to the description parser, like an absent arrangement.
func jobboardlyEmploymentType(arrangement string) string {
	switch arrangement {
	case "fulltime":
		return "full_time"
	case "parttime":
		return "part_time"
	case "contract":
		return "contract"
	case "internship":
		return "internship"
	default:
		return ""
	}
}

package sources

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/strelov1/freehire/internal/dict/location"
)

// kalibrr adapts Kalibrr (kalibrr.com), a general-population Southeast Asian job board whose
// postings each name their own employer. Board-based aggregator: a board is one of the
// platform's COUNTRY markets ("Indonesia", "Philippines"), and every board is read through the
// platform's own "IT and Software" job function, which is what keeps a general board on its
// technical slice.
//
// The listing is the public JSON search the site's own job board calls. A row carries the whole
// posting, body included, so a board is one request and no hydration. The search ignores a
// country or function it does not know and answers the unfiltered market instead, so every row
// is checked against the board's country and the function before it is believed.
//
// fullBoardListing: the board is read in ONE request whose row count must equal the source's own
// reported count, every row distinct; a board larger than kalibrrLimit FAILS rather than paging
// (offset paging under the default relevance sort repeats and skips rows). See the Kalibrr
// traps in AGENTS.md.
type kalibrr struct {
	http JSONGetter
}

const (
	kalibrrSearchURL  = "https://www.kalibrr.com/kjs/job_board/search"
	kalibrrPostingURL = "https://www.kalibrr.com/c/%s/jobs/%d/%s"
	// kalibrrFunction is the job function every board is read through.
	kalibrrFunction = "IT and Software"
	// kalibrrLimit is the most rows one request asks for: eight times the largest technical
	// board, and a ceiling that fails the crawl when a board outgrows it, never a silent end.
	kalibrrLimit = 2000
)

var (
	// kalibrrBoardRE is a country name as the platform spells it ("Indonesia", "Philippines").
	kalibrrBoardRE = regexp.MustCompile(`^[A-Z][A-Za-z]+(?: [A-Z][A-Za-z]+)*$`)
	// kalibrrSegmentRE keeps a company code or posting slug to safe path characters.
	kalibrrSegmentRE = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
)

// kalibrrShorteners are redirectors: an apply link through one is opaque, so the posting page
// is kept instead.
var kalibrrShorteners = map[string]bool{
	"bit.ly": true, "forms.gle": true, "goo.gl": true, "lnkd.in": true, "s.id": true,
	"t.co": true, "tinyurl.com": true,
}

// NewKalibrr builds the Kalibrr adapter over the given HTTP client.
func NewKalibrr(c JSONGetter) Source { return kalibrr{http: c} }

func (kalibrr) Provider() string { return "kalibrr" }

// Every posting names its own employer, so kalibrr stays in the source facet and in the
// cross-source duplicate suppression set.
func (kalibrr) aggregator() {}

// fullBoardListing: Fetch reads the board in one request and fails unless the rows read equal
// the source's own count, all distinct, all of the board's country and function, and the count
// fits kalibrrLimit. See the fullBoardListing interface (source.go) for the bar.
func (kalibrr) fullBoardListing() {}

type kalibrrResult struct {
	Count *int         `json:"count"`
	Jobs  []kalibrrJob `json:"jobs"`
}

// kalibrrJob is one posting as the search states it. Nullable numbers decode to nil.
type kalibrrJob struct {
	ID               int64    `json:"id"`
	Name             string   `json:"name"`
	Slug             string   `json:"slug"`
	Function         string   `json:"function"`
	CompanyName      string   `json:"company_name"`
	Description      string   `json:"description"`
	Qualifications   string   `json:"qualifications"`
	ActivationDate   string   `json:"activation_date"`
	ApplyRedirectURL string   `json:"apply_redirect_url"`
	Tenure           string   `json:"tenure"`
	IsWorkFromHome   bool     `json:"is_work_from_home"`
	IsHybrid         bool     `json:"is_hybrid"`
	SalaryShown      bool     `json:"salary_shown"`
	BaseSalary       *float64 `json:"base_salary"`
	MaximumSalary    *float64 `json:"maximum_salary"`
	SalaryCurrency   string   `json:"salary_currency"`
	SalaryInterval   string   `json:"salary_interval"`
	Company          struct {
		Code string `json:"code"`
		Name string `json:"name"`
	} `json:"company"`
	GoogleLocation *struct {
		Address struct {
			City    string `json:"city"`
			Region  string `json:"region"`
			Country string `json:"country"`
		} `json:"address_components"`
	} `json:"google_location"`
}

func (s kalibrr) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	if !kalibrrBoardRE.MatchString(e.Board) {
		return nil, fmt.Errorf("kalibrr: board %q is not a country name", e.Board)
	}
	q := url.Values{}
	q.Set("country", e.Board)
	q.Set("function", kalibrrFunction)
	q.Set("limit", strconv.Itoa(kalibrrLimit))
	q.Set("offset", "0")
	var r kalibrrResult
	if err := s.http.GetJSON(ctx, kalibrrSearchURL+"?"+q.Encode(), &r); err != nil {
		return nil, fmt.Errorf("kalibrr: board %q: %w", e.Board, err)
	}
	if r.Count == nil {
		return nil, fmt.Errorf("kalibrr: board %q: the search states no count", e.Board)
	}
	count := *r.Count
	switch {
	case count == 0:
		// The board's market always lists technical postings; nothing at all is a renamed
		// function or a changed search, and must not read as a board whose postings all closed.
		return nil, fmt.Errorf("kalibrr: board %q matches no postings", e.Board)
	case count > kalibrrLimit:
		return nil, fmt.Errorf("kalibrr: board %q lists %d postings, over the %d one request reads", e.Board, count, kalibrrLimit)
	case len(r.Jobs) != count:
		return nil, fmt.Errorf("kalibrr: board %q: read %d of %d postings", e.Board, len(r.Jobs), count)
	}
	links := map[string]int{}
	seen := map[int64]bool{}
	for _, j := range r.Jobs {
		// An unknown country or function is ignored by the search, which then answers the
		// unfiltered market; a single row outside the slice means the filter was not applied.
		if j.Function != kalibrrFunction || !strings.EqualFold(j.country(), e.Board) {
			return nil, fmt.Errorf("kalibrr: board %q: posting %d is %q in %q, outside the slice",
				e.Board, j.ID, j.Function, j.country())
		}
		if seen[j.ID] {
			return nil, fmt.Errorf("kalibrr: board %q: posting %d listed twice", e.Board, j.ID)
		}
		seen[j.ID] = true
		if l := j.applyLink(); l != "" {
			links[l]++
		}
	}
	jobs := make([]Job, 0, len(r.Jobs))
	for _, j := range r.Jobs {
		if job, ok := j.toJob(links); ok {
			jobs = append(jobs, job)
		}
	}
	return jobs, nil
}

func (j kalibrrJob) country() string {
	if j.GoogleLocation == nil {
		return ""
	}
	return strings.TrimSpace(j.GoogleLocation.Address.Country)
}

// applyLink is the posting's own external apply link when it states a usable one: http(s) and
// not through a redirector.
func (j kalibrrJob) applyLink() string {
	raw := strings.TrimSpace(j.ApplyRedirectURL)
	if !validHTTPURL(raw) {
		return ""
	}
	u, _ := url.Parse(raw)
	if kalibrrShorteners[strings.ToLower(u.Hostname())] {
		return ""
	}
	return raw
}

func (j kalibrrJob) toJob(linkCounts map[string]int) (Job, bool) {
	title := strings.Join(strings.Fields(j.Name), " ")
	company := strings.TrimSpace(j.CompanyName)
	if company == "" {
		company = strings.TrimSpace(j.Company.Name)
	}
	if j.ID <= 0 || title == "" || company == "" || !kalibrrSegmentRE.MatchString(j.Company.Code) ||
		!kalibrrSegmentRE.MatchString(j.Slug) {
		return Job{}, false
	}
	job := Job{
		ExternalID: strconv.FormatInt(j.ID, 10),
		// The page Kalibrr itself names as canonical.
		URL:            fmt.Sprintf(kalibrrPostingURL, j.Company.Code, j.ID, j.Slug),
		Title:          title,
		Company:        company,
		Description:    sanitizeHTML(j.Description + "\n" + j.Qualifications),
		PostedAt:       parseRFC3339(strings.TrimSpace(j.ActivationDate)),
		EmploymentType: kalibrrEmploymentType(j.Tenure),
		IsTechHint:     true, // every board is read through the IT and Software function
	}
	// A link more than one posting shares is an intake form, not that posting's own page.
	if l := j.applyLink(); l != "" && linkCounts[l] == 1 {
		job.URL = l
	}
	// Both flags false is the platform saying nothing about remote work, not stating on-site.
	switch {
	case j.IsWorkFromHome:
		job.WorkMode = "remote"
	case j.IsHybrid:
		job.WorkMode = "hybrid"
	}
	job.Remote = job.WorkMode == "remote"
	if g := j.GoogleLocation; g != nil {
		a := g.Address
		job.Location = distinctJoin([]string{a.City, a.Region, a.Country}, ", ", strings.TrimSpace)
		if c := location.NormalizeCountry(strings.TrimSpace(a.Country)); c != "" {
			job.Countries = []string{c}
		}
	}
	j.salary(&job)
	return job, true
}

var kalibrrSalaryPeriods = map[string]string{
	"hour":  "hour",
	"day":   "day",
	"month": "month",
	"year":  "year",
}

// salary takes the posting's range only when the employer chose to show it: a hidden salary
// still carries its figures (20,000,000 IDR/month on a posting the site shows no pay for).
func (j kalibrrJob) salary(job *Job) {
	period := kalibrrSalaryPeriods[j.SalaryInterval]
	currency := strings.ToUpper(strings.TrimSpace(j.SalaryCurrency))
	if !j.SalaryShown || period == "" || len(currency) != 3 {
		return
	}
	var lo, hi *int
	if j.BaseSalary != nil {
		lo = roundSalaryPart(*j.BaseSalary)
	}
	if j.MaximumSalary != nil {
		hi = roundSalaryPart(*j.MaximumSalary)
	}
	if lo == nil && hi == nil {
		return
	}
	job.SalaryMin, job.SalaryMax, job.SalaryCurrency, job.SalaryPeriod = lo, hi, currency, period
}

// kalibrrEmploymentType maps the platform's tenure picklist onto vocab.EmploymentTypeValues.
func kalibrrEmploymentType(t string) string {
	switch t {
	case "Full time":
		return "full_time"
	case "Part time":
		return "part_time"
	case "Contractual", "Freelance":
		return "contract"
	case "Internship":
		return "internship"
	default:
		return ""
	}
}

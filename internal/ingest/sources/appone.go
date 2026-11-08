package sources

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// appone adapts AppOne, Paychex's applicant-tracking product. A board is the employer's job
// PORTAL slug — the path of jobs.appone.com/<slug>, the page AppOne itself links as "Open jobs"
// from every posting. The slug is case-insensitive at the platform ("QCRP" and "qcrp" answer
// the same portal) and is stored as the platform spells it in a posting's jobPortalUrl.
//
// Two public JSON endpoints serve the portal and posting pages, both Angular SPAs:
//   - apponePortalURL lists a portal's open postings with title, location, job type, workplace
//     type, structured salary and the posted date, but no body;
//   - apponePostingURL is one posting's detail, the only source of its description.
//
// So this is a HydratingSource: the detail is read once, for postings the catalogue does not
// hold yet.
//
// A posting URL (apply.appone.com/job/<id>) does NOT name its portal — the portal slug only
// appears in the detail's jobPortalUrl, and some postings have none at all. The legacy AppOne
// product (MainInfoReq.asp?R_ID=…&SearchScreenID=…, on www.appone.com and
// recruiting.myapps.paychex.com) is a different system with no per-employer listing this
// adapter can read; see internal/ingest/sources/providers/appone.md.
type appone struct {
	http       JSONGetter
	retryDelay time.Duration
}

// NewAppOne builds the AppOne adapter over the given HTTP client.
func NewAppOne(c JSONGetter) Source { return appone{http: c, retryDelay: apponeRetryDelay} }

func (appone) Provider() string { return "appone" }

const (
	apponePortalURL  = "https://jobs.appone.com/api/portal/v1/companyjobposts/"
	apponePostingURL = "https://apply.appone.com/api/apply/v2/jobposting/"
	apponeJobURL     = "https://apply.appone.com/job/"
)

// apponeAttempts and apponeRetryDelay bound the adapter's own retry of a 403. The platform's
// edge answers a bare "Forbidden" — never a 429 — when it judges a client too fast, and also
// to a scattering of requests at a gentle pace (measured 2026-10-20: 5 of 30 at one request a
// second, the same URL answering 200 before and after). The shared client treats a 403 as
// final, which would fail a board's listing or mark a posting unreadable on a stray refusal,
// so the adapter tries again after a pause that doubles each time. A posting still refused
// after that is an Unreadable marker and is simply asked for again on the next run.
const (
	apponeAttempts   = 3
	apponeRetryDelay = 2 * time.Second
)

// apponePosting is one entry of a portal's listing.
type apponePosting struct {
	ID            string        `json:"jobPostId"`
	URL           string        `json:"jobPostUrl"`
	Title         string        `json:"jobTitle"`
	JobType       string        `json:"jobType"`
	Location      string        `json:"location"`
	DatePosted    string        `json:"datePosted"`
	WorkplaceType string        `json:"workplaceType"`
	Salary        *apponeSalary `json:"salary"`
}

// apponeSalary is the platform's structured pay field. salaryOption says which bounds are set:
// RANGE (both), EXACT_AMOUNT and STARTING_AMOUNT (minimum only), MAXIMUM_AMOUNT (maximum only).
type apponeSalary struct {
	Minimum    float64 `json:"minimum"`
	Maximum    float64 `json:"maximum"`
	Option     string  `json:"salaryOption"`
	PeriodType string  `json:"periodType"`
}

// Fetch reads a portal and every posting's detail.
func (a appone) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	postings, err := a.list(ctx, e.Board)
	if err != nil {
		return nil, err
	}
	return fetchDetails(postings, defaultDetailWorkers, func(p apponePosting) (Job, bool) {
		return a.detail(ctx, e, p)
	}), nil
}

// FetchNew is Fetch that spends a detail request only on postings the catalogue does not hold;
// a seen posting is re-listed as a SeenRefresh, the listing itself proving it is still open.
func (a appone) FetchNew(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	postings, err := a.list(ctx, e.Board)
	if err != nil {
		return nil, err
	}
	return fetchDetails(postings, defaultDetailWorkers, func(p apponePosting) (Job, bool) {
		if seen(p.ID) {
			return Job{
				ExternalID:  p.ID,
				URL:         p.jobURL(),
				Title:       strings.TrimSpace(p.Title),
				Company:     e.Company,
				SeenRefresh: true,
			}, true
		}
		return a.detail(ctx, e, p)
	}), nil
}

// list reads a portal's listing in one unpaginated answer. An unknown portal is a 404, which
// fails the board — never an empty board.
//
// The answer is capped at the 100 most recent postings and accepts no paging parameter
// (measured 2026-10-20: page, pageNumber, pageSize, skip and offset all return the same 100 on
// a portal holding exactly 100). That is why appone does not claim fullBoardListing: a portal
// at the cap may hold more than it shows.
func (a appone) list(ctx context.Context, board string) ([]apponePosting, error) {
	var resp struct {
		JobPosts []apponePosting `json:"jobPosts"`
	}
	if err := a.getJSON(ctx, apponePortalURL+url.PathEscape(board), &resp); err != nil {
		return nil, fmt.Errorf("appone: list portal %s: %w", board, err)
	}
	return resp.JobPosts, nil
}

// apponeDetail is the part of a posting's detail the listing lacks. The description is plain
// text with newline-delimited paragraphs; the detail also carries the application form, which
// is not read.
type apponeDetail struct {
	Description string `json:"description"`
}

// detail fetches one posting's body and maps it, with the listing's fields, to a Job. A 404 is
// the platform's own "closed or removed" and drops the posting; any other failure, or a detail
// with no body, is an unreadableDetail marker — the detail is the only source of the body.
func (a appone) detail(ctx context.Context, e CompanyEntry, p apponePosting) (Job, bool) {
	if p.ID == "" {
		return Job{}, false
	}
	jobURL := p.jobURL()
	var d apponeDetail
	if err := a.getJSON(ctx, apponePostingURL+url.PathEscape(p.ID), &d); err != nil {
		if detailUnreadable(err) {
			return unreadableDetail(p.ID, jobURL, e.Company), true
		}
		return Job{}, false
	}
	body := strings.TrimSpace(d.Description)
	if body == "" {
		return unreadableDetail(p.ID, jobURL, e.Company), true
	}
	mode := workplaceTypeMode(p.WorkplaceType)
	j := Job{
		ExternalID:     p.ID,
		URL:            jobURL,
		Title:          strings.TrimSpace(p.Title),
		Company:        e.Company,
		Location:       strings.TrimSpace(p.Location),
		Description:    sanitizeHTML(plainTextToHTML(body)),
		Remote:         mode == "remote",
		WorkMode:       mode,
		EmploymentType: apponeEmploymentType(p.JobType),
		PostedAt:       parseRFC3339(p.DatePosted),
	}
	p.Salary.apply(&j)
	return j, true
}

// jobURL is the posting's public page: the listing's own link, or the platform's fixed shape
// when the listing omits it.
func (p apponePosting) jobURL() string {
	if p.URL != "" {
		return p.URL
	}
	return apponeJobURL + p.ID
}

// apply writes the structured salary onto j. The currency is stated rather than read: the
// platform names none, and AppOne is a Paychex US payroll product whose employers post in
// dollars — the same argument gusto.go makes. A period freehire has no value for (or an
// option this adapter does not know) writes nothing.
func (s *apponeSalary) apply(j *Job) {
	if s == nil {
		return
	}
	period := strings.ToLower(s.PeriodType)
	if !isSalaryPeriod(period) {
		return
	}
	var lo, hi *int
	switch s.Option {
	case "RANGE":
		lo, hi = roundSalaryPart(s.Minimum), roundSalaryPart(s.Maximum)
	case "EXACT_AMOUNT":
		lo = roundSalaryPart(s.Minimum)
		hi = lo
	case "STARTING_AMOUNT":
		lo = roundSalaryPart(s.Minimum)
	case "MAXIMUM_AMOUNT":
		hi = roundSalaryPart(s.Maximum)
	}
	if lo == nil && hi == nil {
		return
	}
	j.SalaryMin, j.SalaryMax = lo, hi
	j.SalaryCurrency, j.SalaryPeriod = "USD", period
}

// apponeEmploymentType maps the platform's job type to our vocabulary. "Temporary" has no
// value of ours and is left to the pipeline's own dictionaries.
func apponeEmploymentType(jobType string) string {
	switch strings.ToLower(strings.TrimSpace(jobType)) {
	case "full time":
		return "full_time"
	case "part time":
		return "part_time"
	case "internship":
		return "internship"
	case "contract", "contractor":
		return "contract"
	default:
		return ""
	}
}

// getJSON is GetJSON with the 403 retry apponeAttempts explains. Every other outcome is the
// shared client's, returned as is.
func (a appone) getJSON(ctx context.Context, u string, out any) error {
	return retryWhile(ctx, apponeAttempts-1, a.retryDelay, func(err error) bool {
		var se *StatusError
		return errors.As(err, &se) && se.Code == http.StatusForbidden
	}, func() error { return a.http.GetJSON(ctx, u, out) })
}

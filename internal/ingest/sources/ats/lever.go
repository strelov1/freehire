package ats

import (
	"context"
	"fmt"
	"strings"

	"github.com/strelov1/freehire/internal/ingest/sources/kit"
)

// leverBaseURL is the Lever postings API root (default/US host). leverEUBaseURL is the
// EU data-residency host, selected by a board entry's region: eu — boards provisioned
// in the EU 404 on the default host.
const (
	leverBaseURL   = "https://api.lever.co/v0/postings"
	leverEUBaseURL = "https://api.eu.lever.co/v0/postings"
)

// lever adapts the Lever postings API. The JSON-mode endpoint returns a bare array of
// postings whose body is split across HTML description/lists/additional fields, which
// the adapter assembles, so no per-posting detail request is needed.
type lever struct {
	http kit.JSONGetter
}

// NewLever builds the Lever adapter over the given HTTP client.
func NewLever(c kit.JSONGetter) kit.Source { return lever{http: c} }

func (lever) Provider() string { return "lever" }

// fullBoardListing: Fetch is a single unpaginated request returning the board's whole
// postings array — no loop that could stop early. See the fullBoardListing interface for
// the bar.
func (lever) FullBoardListing() {}

// emptyBoardListing: an unknown board answers 404, and an emptied one answers 200 with a bare [];
// a null body fails Fetch. See the emptyBoardListing interface for the bar.
func (lever) EmptyBoardListing() {}

func (l lever) Fetch(ctx context.Context, e kit.CompanyEntry) ([]kit.Job, error) {
	base := leverBaseURL
	if e.Region == "eu" {
		base = leverEUBaseURL
	}
	url := fmt.Sprintf("%s/%s?mode=json", base, e.Board)

	var postings []LeverPosting
	if err := l.http.GetJSON(ctx, url, &postings); err != nil {
		return nil, fmt.Errorf("lever: fetch board %s: %w", e.Board, err)
	}
	if postings == nil {
		return nil, kit.ErrListingAbsent("lever", e.Board, "postings")
	}

	jobs := make([]kit.Job, 0, len(postings))
	for _, p := range postings {
		job := MapLeverPosting(p)
		job.ExternalID = p.ID
		job.Company = e.Company
		jobs = append(jobs, job)
	}
	return jobs, nil
}

// LeverPosting is one job from the Lever postings API, exported so the
// link-following adapter (internal/ingest/linksource) decodes the same payload.
type LeverPosting struct {
	ID            string      `json:"id"`
	Text          string      `json:"text"`
	HostedURL     string      `json:"hostedUrl"`
	CreatedAt     int64       `json:"createdAt"`
	Description   string      `json:"description"`
	Additional    string      `json:"additional"`
	WorkplaceType string      `json:"workplaceType"`
	Country       string      `json:"country"`
	Lists         []LeverList `json:"lists"`
	Categories    struct {
		Location   string `json:"location"`
		Commitment string `json:"commitment"`
		Team       string `json:"team"`
		Department string `json:"department"`
	} `json:"categories"`
	SalaryRange *struct {
		Min      float64 `json:"min"`
		Max      float64 `json:"max"`
		Currency string  `json:"currency"`
		Interval string  `json:"interval"`
	} `json:"salaryRange"`
}

// LeverList is one of a Lever posting's body sections: a heading and its HTML items.
type LeverList struct {
	Text    string `json:"text"`
	Content string `json:"content"`
}

// MapLeverPosting maps a Lever postings-API posting into a Job, so the board adapter and
// the link-following adapter produce identical facets for the same posting. ExternalID and
// Company are left to the caller: the board adapter sets the raw id (the pipeline namespaces
// it by board) and the configured company; the link resolver namespaces the id itself and
// humanizes the company from the board slug (the per-posting API carries no company name).
func MapLeverPosting(p LeverPosting) kit.Job {
	job := kit.Job{
		URL:            p.HostedURL,
		Title:          p.Text,
		Location:       p.Categories.Location,
		Description:    kit.SanitizeHTML(LeverBody(p.Description, p.Lists, p.Additional)),
		Remote:         kit.IsRemote(p.Categories.Location),
		WorkMode:       kit.WorkplaceTypeMode(p.WorkplaceType),
		Countries:      kit.CountryFromCode(p.Country),
		PostedAt:       kit.ParseEpochMillis(p.CreatedAt),
		EmploymentType: leverEmploymentType(p.Categories.Commitment),
		// team is the narrower label, nested under department on Lever.
		Team: kit.FirstNonEmpty(strings.TrimSpace(p.Categories.Team), strings.TrimSpace(p.Categories.Department)),
	}
	if p.SalaryRange != nil {
		if period := leverSalaryPeriod(p.SalaryRange.Interval); period != "" {
			min, max := kit.RoundSalaryPart(p.SalaryRange.Min), kit.RoundSalaryPart(p.SalaryRange.Max)
			if min != nil || max != nil {
				job.SalaryMin, job.SalaryMax = min, max
				job.SalaryCurrency = p.SalaryRange.Currency
				job.SalaryPeriod = period
			}
		}
	}
	return job
}

// LeverBody assembles the posting's description, lists and additional fields into one HTML
// document. Lever splits the body across description + lists (each a heading and its HTML
// items) + additional; the plain mirror is unreliable, so the HTML fields are assembled.
func LeverBody(description string, lists []LeverList, additional string) string {
	var body strings.Builder
	body.WriteString(description)
	for _, list := range lists {
		if list.Text != "" {
			body.WriteString("<h3>")
			body.WriteString(list.Text)
			body.WriteString("</h3>")
		}
		body.WriteString(list.Content)
	}
	body.WriteString(additional)
	return body.String()
}

// leverSalaryPeriod maps Lever's salaryRange.interval (per-year-salary, per-month-salary,
// per-hour-wage, and a one-time bonus/stipend that is not a recurring wage) onto freehire's
// salary_period vocabulary. An unrecognized interval maps to "" so the caller drops the whole
// salary rather than mislabel its period.
func leverSalaryPeriod(interval string) string {
	switch interval {
	case "per-year-salary":
		return "year"
	case "per-month-salary":
		return "month"
	case "per-hour-wage":
		return "hour"
	default:
		return ""
	}
}

// leverEmploymentType maps Lever's free-text categories.commitment (a per-company label
// like "Regular Full-Time" or "Full-Time Maintenance") onto the freehire vocabulary via
// keyword containment, in priority order. An unrecognized/ambiguous value (e.g. "Variable
// Hour") maps to "" so the description parser decides — structured signal only, never a guess.
func leverEmploymentType(commitment string) string {
	c := strings.ToLower(commitment)
	switch {
	case strings.Contains(c, "intern"):
		return "internship"
	case strings.Contains(c, "part-time") || strings.Contains(c, "part time"):
		return "part_time"
	case strings.Contains(c, "full-time") || strings.Contains(c, "full time"):
		return "full_time"
	case strings.Contains(c, "contract") || strings.Contains(c, "temporary") || strings.Contains(c, "seasonal"):
		return "contract"
	default:
		return ""
	}
}

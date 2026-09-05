package sources

import (
	"context"
	"fmt"
)

// ashbyBaseURL is the Ashby public job-board API root.
const ashbyBaseURL = "https://api.ashbyhq.com/posting-api/job-board"

// ashby adapts the Ashby public job-board API. The list endpoint carries an HTML
// description and the posting's workplace type, so no per-posting detail request is needed.
type ashby struct {
	http JSONGetter
}

// NewAshby builds the Ashby adapter over the given HTTP client.
func NewAshby(c JSONGetter) Source { return ashby{http: c} }

func (ashby) Provider() string { return "ashby" }

// fullBoardListing: Fetch is a single unpaginated request returning the board's whole jobs
// array — no loop that could stop early. See the fullBoardListing interface for the bar.
func (ashby) fullBoardListing() {}

func (a ashby) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	// includeCompensation=true is required — omitted, the API silently drops the
	// compensation object entirely rather than erroring (confirmed live 2026-08-14).
	url := fmt.Sprintf("%s/%s?includeCompensation=true", ashbyBaseURL, e.Board)

	var resp struct {
		Jobs []AshbyPosting `json:"jobs"`
	}
	if err := a.http.GetJSON(ctx, url, &resp); err != nil {
		return nil, fmt.Errorf("ashby: fetch board %s: %w", e.Board, err)
	}

	jobs := make([]Job, 0, len(resp.Jobs))
	for _, j := range resp.Jobs {
		job := MapAshbyPosting(j)
		job.ExternalID = j.ID
		job.Company = e.Company
		jobs = append(jobs, job)
	}
	return jobs, nil
}

// AshbyPosting is one job from the Ashby public job-board API, exported so the
// link-following adapter (internal/ingest/linksource) decodes the same payload.
type AshbyPosting struct {
	ID              string `json:"id"`
	Title           string `json:"title"`
	Location        string `json:"location"`
	JobURL          string `json:"jobUrl"`
	PublishedAt     string `json:"publishedAt"`
	DescriptionHTML string `json:"descriptionHtml"`
	IsRemote        bool   `json:"isRemote"`
	WorkplaceType   string `json:"workplaceType"`
	EmploymentType  string `json:"employmentType"`
	Address         struct {
		PostalAddress struct {
			AddressCountry string `json:"addressCountry"`
		} `json:"postalAddress"`
	} `json:"address"`
	// Compensation is present only with includeCompensation=true on the request, and
	// only some employers configure it at all.
	Compensation *AshbyCompensation `json:"compensation"`
}

// AshbyCompensation is the compensation object includeCompensation=true unlocks.
type AshbyCompensation struct {
	CompensationTiers []AshbyCompensationTier `json:"compensationTiers"`
}

// AshbyCompensationTier groups the components of one pay tier (a posting can offer
// more than one, e.g. a level range) — each component is a distinct kind of pay.
type AshbyCompensationTier struct {
	Components []AshbyCompensationComponent `json:"components"`
}

// AshbyCompensationComponent is one line of a tier: CompensationType distinguishes a
// wage (Salary) from Bonus/Commission/EquityCashValue/EquityPercentage, none of which
// are a wage — only "Salary" is read.
type AshbyCompensationComponent struct {
	CompensationType string   `json:"compensationType"`
	Interval         string   `json:"interval"`
	CurrencyCode     string   `json:"currencyCode"`
	MinValue         *float64 `json:"minValue"`
	MaxValue         *float64 `json:"maxValue"`
}

// MapAshbyPosting maps an Ashby API posting into a Job, so the board adapter and the
// link-following adapter produce identical facets for the same posting. workplaceType —
// what the board renders as "Location Type" — decides the work mode; isRemote only says
// "not strictly onsite" (a hybrid posting sets it too), so it is the fallback for boards
// that omit workplaceType. Remote unions the resolved mode with the location heuristic,
// keeping the greenhouse convention that a "Remote" location alone marks a job remote.
// ExternalID and Company are left to the caller: the board adapter sets the raw id (the
// pipeline namespaces it by board) and the configured company; the link resolver
// namespaces the id itself and derives the company from the board slug (the per-board API
// carries no company name).
func MapAshbyPosting(j AshbyPosting) Job {
	mode := firstNonEmpty(workplaceTypeMode(j.WorkplaceType), workModeFromRemote(j.IsRemote))
	job := Job{
		URL:            j.JobURL,
		Title:          j.Title,
		Location:       j.Location,
		Description:    sanitizeHTML(j.DescriptionHTML),
		Remote:         mode == "remote" || isRemote(j.Location),
		WorkMode:       mode,
		Countries:      countryFromCode(j.Address.PostalAddress.AddressCountry),
		PostedAt:       parseRFC3339(j.PublishedAt),
		EmploymentType: ashbyEmploymentType(j.EmploymentType),
	}
	if c, ok := ashbySalaryComponent(j.Compensation); ok {
		if period := ashbySalaryPeriod(c.Interval); period != "" {
			var min, max *int
			if c.MinValue != nil {
				min = roundSalaryPart(*c.MinValue)
			}
			if c.MaxValue != nil {
				max = roundSalaryPart(*c.MaxValue)
			}
			if min != nil || max != nil {
				job.SalaryMin, job.SalaryMax = min, max
				job.SalaryCurrency = c.CurrencyCode
				job.SalaryPeriod = period
			}
		}
	}
	return job
}

// ashbySalaryComponent finds the first Salary-typed compensation component, if any.
func ashbySalaryComponent(comp *AshbyCompensation) (AshbyCompensationComponent, bool) {
	if comp == nil {
		return AshbyCompensationComponent{}, false
	}
	for _, tier := range comp.CompensationTiers {
		for _, c := range tier.Components {
			if c.CompensationType == "Salary" {
				return c, true
			}
		}
	}
	return AshbyCompensationComponent{}, false
}

// ashbySalaryPeriod maps Ashby's compensation interval ("1 YEAR", "1 HOUR", confirmed
// live; "1 MONTH"/"1 DAY" by the same convention, "NONE" for non-recurring
// equity/bonus components) onto freehire's salary_period vocabulary. Unrecognized maps
// to "" so the caller drops the whole component rather than mislabelling its period.
func ashbySalaryPeriod(interval string) string {
	switch interval {
	case "1 YEAR":
		return "year"
	case "1 MONTH":
		return "month"
	case "1 DAY":
		return "day"
	case "1 HOUR":
		return "hour"
	default:
		return ""
	}
}

// ashbyEmploymentType maps Ashby's employmentType enum onto the freehire vocabulary,
// returning "" for an unknown/absent value so the description parser decides.
func ashbyEmploymentType(t string) string {
	switch t {
	case "FullTime":
		return "full_time"
	case "PartTime":
		return "part_time"
	case "Contract", "Temporary":
		return "contract"
	case "Intern", "Internship":
		return "internship"
	default:
		return ""
	}
}

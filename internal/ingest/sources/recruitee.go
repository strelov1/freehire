package sources

import (
	"context"
	"fmt"
	"strconv"

	"github.com/strelov1/freehire/internal/ingest/applyform"
)

// recruiteeSalary is Recruitee's salary object — every field is present but null when
// unstated, confirmed live 2026-08-14. min/max are JSON strings, not numbers.
type recruiteeSalary struct {
	Min      *string `json:"min"`
	Max      *string `json:"max"`
	Period   *string `json:"period"`
	Currency *string `json:"currency"`
}

// recruiteeBaseURL templates the Recruitee public offers API; each board is its own
// subdomain.
const recruiteeBaseURL = "https://%s.recruitee.com/api/offers/"

// recruitee adapts the Recruitee public offers API. The list endpoint splits the body
// across separate description and requirements HTML fields, which the adapter combines,
// so no per-posting detail request is needed.
type recruitee struct {
	http JSONGetter
}

// NewRecruitee builds the Recruitee adapter over the given HTTP client.
func NewRecruitee(c JSONGetter) Source { return recruitee{http: c} }

func (recruitee) Provider() string { return "recruitee" }

// fullBoardListing: Fetch is a single unpaginated request returning the board's whole offers
// array — no loop that could stop early. See the fullBoardListing interface for the bar.
func (recruitee) fullBoardListing() {}

func (r recruitee) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	url := fmt.Sprintf(recruiteeBaseURL, e.Board)

	var resp struct {
		Offers []struct {
			ID           int64           `json:"id"`
			Title        string          `json:"title"`
			CareersURL   string          `json:"careers_url"`
			Location     string          `json:"location"`
			CreatedAt    string          `json:"created_at"`
			Remote       bool            `json:"remote"`
			Hybrid       bool            `json:"hybrid"`
			Description  string          `json:"description"`
			Requirements string          `json:"requirements"`
			Salary       recruiteeSalary `json:"salary"`
			// The same listing also describes the application form, which is why
			// Recruitee is the one provider whose form costs nothing to capture.
			applyform.RecruiteeOffer
		} `json:"offers"`
	}
	if err := r.http.GetJSON(ctx, url, &resp); err != nil {
		return nil, fmt.Errorf("recruitee: fetch board %s: %w", e.Board, err)
	}

	jobs := make([]Job, 0, len(resp.Offers))
	for _, o := range resp.Offers {
		form := applyform.FromRecruitee(o.RecruiteeOffer)
		// Recruitee splits the body across separate description and requirements HTML.
		job := Job{
			ApplyForm:   &form,
			ExternalID:  strconv.FormatInt(o.ID, 10),
			URL:         o.CareersURL,
			Title:       o.Title,
			Company:     e.Company,
			Location:    o.Location,
			Description: sanitizeHTML(o.Description + o.Requirements),
			Remote:      o.Remote,
			WorkMode:    workModeFromRemoteHybrid(o.Remote, o.Hybrid),
			PostedAt:    parseSpaceTime(o.CreatedAt),
		}
		if o.Salary.Period != nil && isSalaryPeriod(*o.Salary.Period) {
			min, max := recruiteeSalaryPart(o.Salary.Min), recruiteeSalaryPart(o.Salary.Max)
			if min != nil || max != nil {
				job.SalaryMin, job.SalaryMax = min, max
				job.SalaryPeriod = *o.Salary.Period
				if o.Salary.Currency != nil {
					job.SalaryCurrency = *o.Salary.Currency
				}
			}
		}
		jobs = append(jobs, job)
	}
	return jobs, nil
}

// recruiteeSalaryPart parses one salary bound — a JSON string, not a number, on
// Recruitee's API — into freehire's rounded integer form. nil/unparseable reports
// absent rather than a guess.
func recruiteeSalaryPart(s *string) *int {
	if s == nil {
		return nil
	}
	v, err := strconv.ParseFloat(*s, 64)
	if err != nil {
		return nil
	}
	return roundSalaryPart(v)
}

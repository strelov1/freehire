package ats

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/strelov1/freehire/internal/ingest/applyform"
	"github.com/strelov1/freehire/internal/ingest/sources/kit"
)

// recruiteeSalary is Recruitee's salary object: every field is present but null when unstated.
// min/max are JSON strings, not numbers.
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
	http kit.JSONGetter
}

// NewRecruitee builds the Recruitee adapter over the given HTTP client.
func NewRecruitee(c kit.JSONGetter) kit.Source { return recruitee{http: c} }

func (recruitee) Provider() string { return "recruitee" }

// fullBoardListing: Fetch is a single unpaginated request returning the board's whole offers
// array — no loop that could stop early. See the fullBoardListing interface for the bar.
func (recruitee) FullBoardListing() {}

// emptyBoardListing: an unknown board answers 404, and an emptied one answers 200 with
// "offers": []; an envelope without the array fails Fetch. See the emptyBoardListing interface.
func (recruitee) EmptyBoardListing() {}

func (r recruitee) Fetch(ctx context.Context, e kit.CompanyEntry) ([]kit.Job, error) {
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
			// EmploymentTypeCode and Department are what the board states about the role;
			// see recruiteeEmploymentType for the codes.
			EmploymentTypeCode string `json:"employment_type_code"`
			Department         string `json:"department"`
			// The same listing also describes the application form, which is why
			// Recruitee is the one provider whose form costs nothing to capture.
			applyform.RecruiteeOffer
		} `json:"offers"`
	}
	if err := r.http.GetJSON(ctx, url, &resp); err != nil {
		return nil, fmt.Errorf("recruitee: fetch board %s: %w", e.Board, err)
	}
	if resp.Offers == nil {
		return nil, kit.ErrListingAbsent("recruitee", e.Board, "offers")
	}

	jobs := make([]kit.Job, 0, len(resp.Offers))
	for _, o := range resp.Offers {
		form := applyform.FromRecruitee(o.RecruiteeOffer)
		// Recruitee splits the body across separate description and requirements HTML.
		job := kit.Job{
			ApplyForm:   &form,
			ExternalID:  strconv.FormatInt(o.ID, 10),
			URL:         o.CareersURL,
			Title:       o.Title,
			Company:     e.Company,
			Location:    o.Location,
			Description: kit.SanitizeHTML(o.Description + o.Requirements),
			Remote:      o.Remote,
			WorkMode:    kit.WorkModeFromRemoteHybrid(o.Remote, o.Hybrid),
			PostedAt:    kit.ParseSpaceTime(o.CreatedAt),
			// The department the board files the offer under, verbatim.
			Team:           strings.TrimSpace(o.Department),
			EmploymentType: recruiteeEmploymentType(o.EmploymentTypeCode),
		}
		if o.Salary.Period != nil && kit.IsSalaryPeriod(*o.Salary.Period) {
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

// recruiteeEmploymentType maps Recruitee's employment_type_code onto the freehire
// vocabulary. The codes seen across 60 boards (2026-11-24) fold a schedule and a term into
// one value — fulltime_permanent, parttime_fixed_term, parttime_minijob — so the schedule
// prefix decides. "temporary" says how long, not on what terms, so it states nothing, as in
// the sources package's schemaFixedTermEmploymentType; an apprenticeship files as an internship
// as freework's does.
func recruiteeEmploymentType(code string) string {
	code = strings.ToLower(strings.TrimSpace(code))
	switch {
	case strings.HasPrefix(code, "fulltime"):
		return "full_time"
	case strings.HasPrefix(code, "parttime"):
		return "part_time"
	}
	switch code {
	case "internship", "apprenticeship":
		return "internship"
	case "contract", "freelance":
		return "contract"
	}
	return ""
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
	return kit.RoundSalaryPart(v)
}

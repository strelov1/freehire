package sources

import (
	"context"
	"fmt"
	"strconv"

	"github.com/strelov1/freehire/internal/applyform"
)

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

func (r recruitee) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	url := fmt.Sprintf(recruiteeBaseURL, e.Board)

	var resp struct {
		Offers []struct {
			ID           int64  `json:"id"`
			Title        string `json:"title"`
			CareersURL   string `json:"careers_url"`
			Location     string `json:"location"`
			CreatedAt    string `json:"created_at"`
			Remote       bool   `json:"remote"`
			Hybrid       bool   `json:"hybrid"`
			Description  string `json:"description"`
			Requirements string `json:"requirements"`
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
		jobs = append(jobs, Job{
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
		})
	}
	return jobs, nil
}

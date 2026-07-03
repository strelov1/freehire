package sources

import (
	"context"
	"fmt"
	"strings"
)

// hireology adapts careers.hireology.com career sites. The board is the tenant slug. The public
// JSON:API at api.hireology.com/v1/careers/<slug> returns all of a company's postings with the
// description inline (job-description), so a whole board comes from one JSON call with no
// per-posting detail fetch.
type hireology struct {
	http JSONGetter
}

const hireologyAPI = "https://api.hireology.com/v1/careers/%s"

// NewHireology builds the careers.hireology.com adapter over the given JSON client.
func NewHireology(c JSONGetter) Source { return hireology{http: c} }

func (hireology) Provider() string { return "hireology" }

func (s hireology) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	var resp struct {
		Data []struct {
			ID         string           `json:"id"`
			Attributes hireologyPosting `json:"attributes"`
		} `json:"data"`
	}
	if err := s.http.GetJSON(ctx, fmt.Sprintf(hireologyAPI, e.Board), &resp); err != nil {
		return nil, fmt.Errorf("hireology: board %q: %w", e.Board, err)
	}

	var jobs []Job
	for _, d := range resp.Data {
		a := d.Attributes
		if d.ID == "" || !strings.EqualFold(a.Status, "Open") {
			continue // keep only live postings
		}
		location := hireologyLocationString(a.Locations)
		jobs = append(jobs, Job{
			ExternalID:  d.ID,
			URL:         firstNonEmpty(a.CareerSiteURL, fmt.Sprintf("https://careers.hireology.com/%s/%s/description", e.Board, d.ID)),
			Title:       a.Name,
			Company:     e.Company,
			Location:    location,
			Description: sanitizeHTML(a.JobDescription),
			Remote:      a.Remote || isRemote(location),
		})
	}
	return jobs, nil
}

// hireologyLocationString joins the posting's location strings, which the API often leaves
// empty (small-business tenants) — then the location is simply unknown.
func hireologyLocationString(locs []string) string {
	return joinNonEmpty(locs...)
}

// hireologyPosting is the `attributes` object of one api.hireology.com/v1/careers/<slug> entry.
type hireologyPosting struct {
	Name           string   `json:"name"`
	Remote         bool     `json:"remote"`
	JobDescription string   `json:"job-description"`
	Locations      []string `json:"locations"`
	Status         string   `json:"status"`
	CareerSiteURL  string   `json:"career-site-url"`
}

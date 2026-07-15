package sources

import (
	"context"
	"fmt"
	"net/url"
)

// workableMarketplaceBaseURL is the Workable public marketplace API root. Unlike the
// company-hosted widget API (apply.workable.com/.../widget/accounts/<board>), this serves
// postings a company published to the jobs.workable.com marketplace but not to a widget
// board — so a "marketplace-only" employer, empty on the widget endpoint, is reachable here.
const workableMarketplaceBaseURL = "https://jobs.workable.com/api/v1/companies"

// wkMarketplaceMaxPages bounds token pagination so a feed that never stops handing back a
// non-empty token cannot loop forever.
const wkMarketplaceMaxPages = 100

// workableMarketplace adapts the Workable marketplace API. The board is the company's
// opaque marketplace hashid (from a posting's company URL). The list carries an inline HTML
// description, so no per-posting detail request is needed; pages chain via nextPageToken.
type workableMarketplace struct {
	http JSONGetter
}

// NewWorkableMarketplace builds the Workable marketplace adapter over the given HTTP client.
func NewWorkableMarketplace(c JSONGetter) Source { return workableMarketplace{http: c} }

func (workableMarketplace) Provider() string { return "workablemarketplace" }

// wkMarketplaceJob is one posting in the marketplace company feed.
type wkMarketplaceJob struct {
	ID             string `json:"id"`
	Title          string `json:"title"`
	State          string `json:"state"`
	Description    string `json:"description"`
	EmploymentType string `json:"employmentType"`
	Workplace      string `json:"workplace"`
	URL            string `json:"url"`
	Created        string `json:"created"`
	Location       struct {
		City        string `json:"city"`
		CountryName string `json:"countryName"`
	} `json:"location"`
}

func (w workableMarketplace) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	var jobs []Job
	seen := make(map[string]bool)
	token := ""
	for page := 0; page < wkMarketplaceMaxPages; page++ {
		u := fmt.Sprintf("%s/%s", workableMarketplaceBaseURL, e.Board)
		if token != "" {
			u += "?pageToken=" + url.QueryEscape(token)
		}

		var resp struct {
			Jobs          []wkMarketplaceJob `json:"jobs"`
			NextPageToken string             `json:"nextPageToken"`
		}
		if err := w.http.GetJSON(ctx, u, &resp); err != nil {
			if page == 0 {
				return nil, fmt.Errorf("workablemarketplace: fetch company %s: %w", e.Board, err)
			}
			break // a later page failing ends enumeration with the jobs gathered so far
		}

		newCount := 0
		for _, j := range resp.Jobs {
			// The feed carries drafts/archived alongside live postings; only a published
			// one is a real vacancy. seen guards against a token that re-serves a page.
			if j.State != "published" || seen[j.ID] {
				continue
			}
			seen[j.ID] = true
			newCount++
			remote := j.Workplace == "remote"
			jobs = append(jobs, Job{
				ExternalID:     j.ID,
				URL:            j.URL,
				Title:          j.Title,
				Company:        e.Company,
				Location:       joinNonEmpty(j.Location.City, j.Location.CountryName),
				Description:    sanitizeHTML(j.Description),
				Remote:         remote,
				WorkMode:       workableWorkplaceMode(j.Workplace),
				PostedAt:       parseRFC3339(j.Created),
				EmploymentType: workableEmploymentType(j.EmploymentType),
			})
		}

		// Stop when the feed signals the end, or a page adds nothing new (an empty page or
		// a token that loops back), so enumeration terminates without trusting the token alone.
		if resp.NextPageToken == "" || newCount == 0 {
			break
		}
		token = resp.NextPageToken
	}
	return jobs, nil
}

// workableWorkplaceMode maps Workable's marketplace "workplace" enum onto the freehire
// work-mode vocabulary, returning "" for an absent/unknown value (the location parser decides).
func workableWorkplaceMode(v string) string {
	switch v {
	case "remote":
		return "remote"
	case "hybrid":
		return "hybrid"
	case "on-site", "onsite":
		return "onsite"
	default:
		return ""
	}
}

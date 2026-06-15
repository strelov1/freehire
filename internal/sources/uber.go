package sources

import (
	"context"
	"fmt"
	"strconv"
)

// uber adapts Uber's public careers search API (www.uber.com/api/loadSearchJobsResults),
// a single-company source with no per-tenant board id (boardless). The search endpoint is
// a POST that returns the full posting — including the Markdown description — inline, so
// unlike Ozon/Gem there is no per-job detail request: one paged list call assembles every
// Job. The HTML career pages sit behind bot protection, but this JSON API is open.
type uber struct {
	http HeaderJSONPoster
}

const (
	uberSearchURL  = "https://www.uber.com/api/loadSearchJobsResults?localeCode=en"
	uberVacancyURL = "https://www.uber.com/global/en/careers/list/%d/"
	// uberPageLimit is how many postings to request per page.
	uberPageLimit = 100
	// uberCSRFToken is a non-secret header Uber's search API requires: any value is
	// accepted, but without it the endpoint 403s (it gates casual scraping, not auth).
	uberCSRFToken = "x"
)

// NewUber builds the Uber adapter over the given HTTP client.
func NewUber(c HeaderJSONPoster) Source { return uber{http: c} }

func (uber) Provider() string { return "uber" }

// uber is single-company, so its config entries carry no board.
func (uber) boardless() {}

// uberRequest is the search POST body. An empty Params returns the whole catalogue;
// pagination is driven by Page (0-based) at a fixed Limit.
type uberRequest struct {
	Params map[string]any `json:"params"`
	Page   int            `json:"page"`
	Limit  int            `json:"limit"`
}

// uberResponse is the search response. Status is "success" on a healthy read; Results is
// null past the last page. TotalResults.Low is the catalogue size (the API splits a 64-bit
// count into low/high words; the job count never overflows Low).
type uberResponse struct {
	Status string `json:"status"`
	Data   struct {
		Results      []uberResult `json:"results"`
		TotalResults struct {
			Low int `json:"low"`
		} `json:"totalResults"`
	} `json:"data"`
}

// uberResult is one posting from the search response. The description is Markdown.
type uberResult struct {
	ID          int64  `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Location    struct {
		City        string `json:"city"`
		Region      string `json:"region"`
		CountryName string `json:"countryName"`
	} `json:"location"`
	CreationDate string `json:"creationDate"`
}

func (u uber) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	var jobs []Job
	for page := 0; ; page++ {
		var resp uberResponse
		req := uberRequest{Params: map[string]any{}, Page: page, Limit: uberPageLimit}
		headers := map[string]string{"x-csrf-token": uberCSRFToken}
		if err := u.http.PostJSONWithHeaders(ctx, uberSearchURL, headers, req, &resp); err != nil {
			return nil, fmt.Errorf("uber: list page %d: %w", page, err)
		}
		if resp.Status != "success" {
			return nil, fmt.Errorf("uber: list page %d: status %q", page, resp.Status)
		}
		if len(resp.Data.Results) == 0 {
			break
		}
		for _, r := range resp.Data.Results {
			jobs = append(jobs, u.toJob(e, r))
		}
		if len(jobs) >= resp.Data.TotalResults.Low {
			break
		}
	}
	return jobs, nil
}

// toJob maps a search result to a Job. The Markdown description is rendered to HTML and
// sanitized; the location collapses blank city/region fields.
func (uber) toJob(e CompanyEntry, r uberResult) Job {
	return Job{
		ExternalID:  strconv.FormatInt(r.ID, 10),
		URL:         fmt.Sprintf(uberVacancyURL, r.ID),
		Title:       r.Title,
		Company:     e.Company,
		Location:    joinNonEmpty(r.Location.City, r.Location.Region, r.Location.CountryName),
		Description: sanitizeHTML(markdownToHTML(r.Description)),
		PostedAt:    parseRFC3339(r.CreationDate),
	}
}

package sources

import (
	"context"
	"fmt"
	"html"
	"strconv"
)

// greenhouseBaseURL is the Greenhouse public boards API root. Unlike Lever, Greenhouse
// serves EU-hosted boards (job-boards.eu.greenhouse.io front-ends) from this same single
// API host, so no per-region host selection is needed.
const greenhouseBaseURL = "https://boards-api.greenhouse.io/v1/boards"

// greenhouse adapts the Greenhouse public boards API. The list endpoint carries the
// description inline when asked with content=true, so no per-posting detail request
// is needed.
type greenhouse struct {
	http JSONGetter
}

// NewGreenhouse builds the Greenhouse adapter over the given HTTP client.
func NewGreenhouse(c JSONGetter) Source { return greenhouse{http: c} }

func (greenhouse) Provider() string { return "greenhouse" }

func (g greenhouse) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	url := fmt.Sprintf("%s/%s/jobs?content=true", greenhouseBaseURL, e.Board)

	var resp struct {
		Jobs []struct {
			ID          int64  `json:"id"`
			Title       string `json:"title"`
			AbsoluteURL string `json:"absolute_url"`
			UpdatedAt   string `json:"updated_at"`
			Content     string `json:"content"`
			Location    struct {
				Name string `json:"name"`
			} `json:"location"`
		} `json:"jobs"`
	}
	if err := g.http.GetJSON(ctx, url, &resp); err != nil {
		return nil, fmt.Errorf("greenhouse: fetch board %s: %w", e.Board, err)
	}

	jobs := make([]Job, 0, len(resp.Jobs))
	for _, j := range resp.Jobs {
		jobs = append(jobs, Job{
			ExternalID:  strconv.FormatInt(j.ID, 10),
			URL:         j.AbsoluteURL,
			Title:       j.Title,
			Company:     e.Company,
			Location:    j.Location.Name,
			Description: sanitizeHTML(html.UnescapeString(j.Content)),
			Remote:      isRemote(j.Location.Name),
			PostedAt:    parseRFC3339(j.UpdatedAt),
		})
	}
	return jobs, nil
}

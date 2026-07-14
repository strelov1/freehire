package sources

import (
	"context"
	"fmt"
	"strings"
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
	http JSONGetter
}

// NewLever builds the Lever adapter over the given HTTP client.
func NewLever(c JSONGetter) Source { return lever{http: c} }

func (lever) Provider() string { return "lever" }

func (l lever) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	base := leverBaseURL
	if e.Region == "eu" {
		base = leverEUBaseURL
	}
	url := fmt.Sprintf("%s/%s?mode=json", base, e.Board)

	var postings []struct {
		ID            string `json:"id"`
		Text          string `json:"text"`
		HostedURL     string `json:"hostedUrl"`
		CreatedAt     int64  `json:"createdAt"`
		Description   string `json:"description"`
		Additional    string `json:"additional"`
		WorkplaceType string `json:"workplaceType"`
		Lists         []struct {
			Text    string `json:"text"`
			Content string `json:"content"`
		} `json:"lists"`
		Categories struct {
			Location   string `json:"location"`
			Commitment string `json:"commitment"`
		} `json:"categories"`
	}
	if err := l.http.GetJSON(ctx, url, &postings); err != nil {
		return nil, fmt.Errorf("lever: fetch board %s: %w", e.Board, err)
	}

	jobs := make([]Job, 0, len(postings))
	for _, p := range postings {
		// Lever splits the body across description + lists (each a heading and its
		// HTML items) + additional; the plain mirror is unreliable, so assemble the
		// HTML fields into one document.
		var body strings.Builder
		body.WriteString(p.Description)
		for _, list := range p.Lists {
			if list.Text != "" {
				body.WriteString("<h3>")
				body.WriteString(list.Text)
				body.WriteString("</h3>")
			}
			body.WriteString(list.Content)
		}
		body.WriteString(p.Additional)

		jobs = append(jobs, Job{
			ExternalID:     p.ID,
			URL:            p.HostedURL,
			Title:          p.Text,
			Company:        e.Company,
			Location:       p.Categories.Location,
			Description:    sanitizeHTML(body.String()),
			Remote:         isRemote(p.Categories.Location),
			WorkMode:       workplaceTypeMode(p.WorkplaceType),
			PostedAt:       parseEpochMillis(p.CreatedAt),
			EmploymentType: leverEmploymentType(p.Categories.Commitment),
		})
	}
	return jobs, nil
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

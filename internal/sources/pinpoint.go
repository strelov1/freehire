package sources

import (
	"context"
	"fmt"
)

// pinpoint adapts the Pinpoint public postings API. Each board is its own subdomain and
// the list endpoint carries the full posting: the body is split across several inline
// HTML sections (description, responsibilities, skills, benefits), so no per-posting
// detail request is needed.
type pinpoint struct {
	http JSONGetter
}

// NewPinpoint builds the Pinpoint adapter over the given HTTP client.
func NewPinpoint(c JSONGetter) Source { return pinpoint{http: c} }

func (pinpoint) Provider() string { return "pinpoint" }

func (p pinpoint) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	url := fmt.Sprintf("https://%s.pinpointhq.com/postings.json", e.Board)

	var resp struct {
		Data []struct {
			ID                       string `json:"id"`
			Title                    string `json:"title"`
			URL                      string `json:"url"`
			Description              string `json:"description"`
			KeyResponsibilities      string `json:"key_responsibilities"`
			SkillsKnowledgeExpertise string `json:"skills_knowledge_expertise"`
			Benefits                 string `json:"benefits"`
			WorkplaceType            string `json:"workplace_type"`
			Location                 struct {
				City     string `json:"city"`
				Province string `json:"province"`
				Name     string `json:"name"`
			} `json:"location"`
		} `json:"data"`
	}
	if err := p.http.GetJSON(ctx, url, &resp); err != nil {
		return nil, fmt.Errorf("pinpoint: fetch board %s: %w", e.Board, err)
	}

	jobs := make([]Job, 0, len(resp.Data))
	for _, d := range resp.Data {
		// Pinpoint splits the body across separate HTML sections.
		body := d.Description + d.KeyResponsibilities + d.SkillsKnowledgeExpertise + d.Benefits
		jobs = append(jobs, Job{
			ExternalID: d.ID,
			URL:        d.URL,
			Title:      d.Title,
			Company:    e.Company,
			// Prefer the structured city/province; some boards leave both empty and
			// only fill the human-readable name ("Remote", "Pittsburgh, PA"), so fall
			// back to it rather than yielding a blank location.
			Location:    firstNonEmpty(joinNonEmpty(d.Location.City, d.Location.Province), d.Location.Name),
			Description: sanitizeHTML(body),
			Remote:      d.WorkplaceType == "remote",
			PostedAt:    nil, // the postings feed carries no publish date
		})
	}
	return jobs, nil
}

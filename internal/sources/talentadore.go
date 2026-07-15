package sources

import (
	"context"
	"fmt"
	"strings"
)

// talentadoreFeedURL templates TalentAdore Hire's public positions feed. TalentAdore
// (careers.talentadore.com is only their WordPress marketing site) hosts each employer's
// live openings as a JSON feed at ats.talentadore.com/positions/<token>/json — the same
// endpoint the ta-json-careers.js widget reads to render a customer's career page. The
// board is that per-employer feed token (e.g. "9wmfASE"), not a human-readable slug; it
// is the data-url token embedded in the career-site widget. The v=2 query param is
// load-bearing: without it start_date/updated arrive as timezone-less "2006-01-02T15:04:05"
// strings that RFC3339 rejects (posted_at silently null); v=2 emits proper RFC3339 with Z.
const talentadoreFeedURL = "https://ats.talentadore.com/positions/%s/json?v=2"

// talentadore adapts TalentAdore Hire, a Nordic AI-assisted ATS. The feed carries each
// posting's full description_html inline, so no per-posting detail request is needed. It
// exposes no structured remote/work-mode field, so remote is inferred from the location
// text like the other free-text-location adapters.
type talentadore struct {
	http JSONGetter
}

// NewTalentAdore builds the TalentAdore adapter over the given HTTP client.
func NewTalentAdore(c JSONGetter) Source { return talentadore{http: c} }

func (talentadore) Provider() string { return "talentadore" }

// talentadoreFeed is the slice of the positions feed we read. Each job carries its own
// description_html, so the list is the only request per board.
type talentadoreFeed struct {
	Jobs []talentadoreJob `json:"jobs"`
}

// talentadoreJob is one posting. job_token is the stable native id — it is what the public
// apply URL (link, e.g. .../apply/<slug>/<token>) is keyed on, so it survives the internal
// id being regenerated. description_html is the full body (description_text is the plain
// fallback); start_date is the publish date and updated the last-modified fallback. city,
// county, and country compose the location; the plain location field is a street address we
// omit as noise. employment_type is present in the schema but unset in observed feeds, so it
// is left to the description parser/dictionaries rather than mapped from an unknown vocabulary.
type talentadoreJob struct {
	JobToken        string `json:"job_token"`
	Name            string `json:"name"`
	Link            string `json:"link"`
	DescriptionHTML string `json:"description_html"`
	DescriptionText string `json:"description_text"`
	StartDate       string `json:"start_date"`
	Updated         string `json:"updated"`
	City            string `json:"city"`
	County          string `json:"county"`
	Country         string `json:"country"`
}

func (s talentadore) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	url := fmt.Sprintf(talentadoreFeedURL, e.Board)
	var feed talentadoreFeed
	if err := s.http.GetJSON(ctx, url, &feed); err != nil {
		return nil, fmt.Errorf("talentadore: fetch board %s: %w", e.Board, err)
	}

	jobs := make([]Job, 0, len(feed.Jobs))
	for _, j := range feed.Jobs {
		token := strings.TrimSpace(j.JobToken)
		if token == "" {
			continue // no native id → would collide on the dedup key; skip it
		}
		location := joinNonEmpty(j.City, j.County, j.Country)
		description := sanitizeHTML(j.DescriptionHTML)
		if description == "" {
			description = strings.TrimSpace(j.DescriptionText)
		}
		jobs = append(jobs, Job{
			ExternalID:  token,
			URL:         j.Link,
			Title:       j.Name,
			Company:     e.Company,
			Location:    location,
			Description: description,
			Remote:      isRemote(location),
			PostedAt:    parseRFC3339(firstNonEmpty(j.StartDate, j.Updated)),
		})
	}
	return jobs, nil
}

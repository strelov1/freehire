package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"strconv"
	"strings"
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
		Jobs []GreenhousePosting `json:"jobs"`
	}
	if err := g.http.GetJSON(ctx, url, &resp); err != nil {
		return nil, fmt.Errorf("greenhouse: fetch board %s: %w", e.Board, err)
	}

	jobs := make([]Job, 0, len(resp.Jobs))
	for _, j := range resp.Jobs {
		job := MapGreenhousePosting(j)
		job.ExternalID = strconv.FormatInt(j.ID, 10)
		job.Company = e.Company
		jobs = append(jobs, job)
	}
	return jobs, nil
}

// GreenhousePosting is one job from the Greenhouse public boards API (the list endpoint
// with content=true and the per-job endpoint share the shape), exported so the
// link-following adapter (internal/linksource) decodes the same payload.
type GreenhousePosting struct {
	ID          int64  `json:"id"`
	Title       string `json:"title"`
	AbsoluteURL string `json:"absolute_url"`
	UpdatedAt   string `json:"updated_at"`
	Content     string `json:"content"`
	// CompanyName is served by the per-job endpoint only; the board list leaves it empty.
	CompanyName string `json:"company_name"`
	Location    struct {
		Name string `json:"name"`
	} `json:"location"`
	// Metadata carries the employer's own custom application-question fields — most
	// boards set none (null), some carry unrelated ones ("Headcount #"), and a minority
	// configure an "Employment Type" field read by greenhouseMetadataEmploymentType.
	Metadata []GreenhouseMetadataField `json:"metadata"`
}

// GreenhouseMetadataField is one employer-configured custom field on a posting.
// Value is decoded lazily (raw JSON) because it varies by field: a plain string for
// a single_select/short_text field, null when unset, or an empty array for an unset
// multi-select — greenhouseMetadataEmploymentType treats every shape but "a string"
// as absent rather than guessing.
type GreenhouseMetadataField struct {
	Name  string          `json:"name"`
	Value json.RawMessage `json:"value"`
}

// MapGreenhousePosting maps a Greenhouse boards-API posting into a Job, so the board
// adapter and the link-following adapter produce identical facets for the same posting.
// ExternalID and Company are left to the caller: the board adapter sets the raw id (the
// pipeline namespaces it by board) and the configured company; the link resolver
// namespaces the id itself and takes the company from the API's company_name (a TG link
// can point at a board the crawl config does not list).
func MapGreenhousePosting(j GreenhousePosting) Job {
	return Job{
		URL:            j.AbsoluteURL,
		Title:          j.Title,
		Location:       j.Location.Name,
		Description:    sanitizeHTML(html.UnescapeString(j.Content)),
		Remote:         isRemote(j.Location.Name),
		PostedAt:       parseRFC3339(j.UpdatedAt),
		EmploymentType: greenhouseMetadataEmploymentType(j.Metadata),
	}
}

// greenhouseMetadataEmploymentType reads the employer-configured "Employment Type"
// custom field a minority of Greenhouse boards carry (confirmed live: 1800contacts,
// solidpower, carrotfertility all set it to "Full-time") — a genuine structured
// signal, not a free-text guess: the employer explicitly labeled and typed this
// value, the same trust level as Lever's commitment or SmartRecruiters'
// typeOfEmployment. Most boards carry no such field at all.
func greenhouseMetadataEmploymentType(metadata []GreenhouseMetadataField) string {
	for _, m := range metadata {
		if !strings.EqualFold(strings.TrimSpace(m.Name), "Employment Type") {
			continue
		}
		var value string
		if json.Unmarshal(m.Value, &value) != nil {
			return "" // null, an empty array (unset multi-select), or another shape
		}
		return greenhouseEmploymentType(value)
	}
	return ""
}

// greenhouseEmploymentType maps the "Employment Type" field's free-text value (e.g.
// "Full-time", "Part-Time", "1099 Contractor") onto the freehire vocabulary via
// keyword containment — the same style as leverEmploymentType, since the field is
// employer-typed free text, not a fixed enum. "" for an unrecognized value, so the
// description parser decides rather than a guess here.
func greenhouseEmploymentType(value string) string {
	v := strings.ToLower(value)
	switch {
	case strings.Contains(v, "intern"):
		return "internship"
	case strings.Contains(v, "part-time") || strings.Contains(v, "part time"):
		return "part_time"
	case strings.Contains(v, "full-time") || strings.Contains(v, "full time"):
		return "full_time"
	case strings.Contains(v, "contract") || strings.Contains(v, "temporary") || strings.Contains(v, "seasonal"):
		return "contract"
	default:
		return ""
	}
}

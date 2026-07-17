package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"strings"
)

// briefhq adapts a hand-rolled static careers page (briefhq.ai/careers) that inlines every
// vacancy as its own schema.org JobPosting ld+json block on a single listing page. There is no
// board API and no per-posting detail page — the listing itself carries each posting's full
// fields — so one fetch yields every job. The board is the careers page's "<host>/<path>" (no
// scheme), e.g. "briefhq.ai/careers/"; the native posting id is the JobPosting identifier.value
// (a stable per-job slug), which also anchors the posting's deeplink (…/careers/#<slug>).
type briefhq struct {
	http HTMLGetter
}

// NewBriefHQ builds the briefhq static-careers-page adapter over the given HTML client.
func NewBriefHQ(c HTMLGetter) Source { return briefhq{http: c} }

func (briefhq) Provider() string { return "briefhq" }

func (s briefhq) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	pageURL := "https://" + e.Board
	root, err := s.http.GetHTML(ctx, pageURL)
	if err != nil {
		return nil, fmt.Errorf("briefhq: fetch %s: %w", e.Board, err)
	}

	nodes := LDJobPostings(root)
	jobs := make([]Job, 0, len(nodes))
	for _, raw := range nodes {
		var p briefhqPosting
		if json.Unmarshal(raw, &p) != nil || p.Title == "" {
			continue // a block that fails to decode or carries no title is not a storable job
		}
		id := firstNonEmpty(p.Identifier.Value, p.URL, pageURL)
		// jobLocationType is the schema.org work-arrangement signal: TELECOMMUTE means remote,
		// ON_SITE means not; anything else falls back to the free-text location.
		remote := strings.EqualFold(p.JobLocationType, "TELECOMMUTE")
		jobs = append(jobs, Job{
			ExternalID:     id,
			URL:            firstNonEmpty(p.URL, pageURL),
			Title:          p.Title,
			Company:        firstNonEmpty(p.HiringOrganization.Name, e.Company),
			Location:       p.location(),
			Description:    sanitizeHTML(html.UnescapeString(p.Description)),
			Remote:         remote || isRemote(p.location()),
			WorkMode:       workModeFromRemote(remote),
			EmploymentType: schemaEmploymentType(p.EmploymentType),
			PostedAt:       parseRFC3339OrDate(p.DatePosted),
		})
	}
	return jobs, nil
}

// briefhqPosting is one schema.org JobPosting decoded from the careers page. jobLocation is a
// single Place; jobLocationType (ON_SITE/TELECOMMUTE) is the work-arrangement signal.
type briefhqPosting struct {
	Title              string `json:"title"`
	Description        string `json:"description"`
	URL                string `json:"url"`
	DatePosted         string `json:"datePosted"`
	EmploymentType     string `json:"employmentType"`
	JobLocationType    string `json:"jobLocationType"`
	HiringOrganization struct {
		Name string `json:"name"`
	} `json:"hiringOrganization"`
	JobLocation schemaPlace `json:"jobLocation"`
	Identifier  struct {
		Value string `json:"value"`
	} `json:"identifier"`
}

// location builds the free-text location from the jobLocation address.
func (p briefhqPosting) location() string {
	return p.JobLocation.Address.Location()
}

package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// talenthr adapts TalentHR (talenthr.io), a multi-tenant ATS. Each tenant's public careers
// site is a Next.js app at https://jobs.talenthr.io/<board> (board = the tenant slug)
// embedding its data in the __NEXT_DATA__ script: the listing carries every open position's id
// and slug, and each position's own detail page carries the full record (description,
// structured location, a work-mode and an employment enum, and the publish date). The body and
// facets therefore come from a per-posting detail fetch, fanned out like the other Next.js
// detail adapters (alignerr/micro1). The native posting id namespaces external_id.
type talenthr struct {
	http TextGetter
}

// NewTalentHR builds the TalentHR adapter over the given HTTP client.
func NewTalentHR(c TextGetter) Source { return talenthr{http: c} }

func (talenthr) Provider() string { return "talenthr" }

// talenthrBaseURL is the shared host serving every tenant's careers site.
const talenthrBaseURL = "https://jobs.talenthr.io"

// talenthrNextData is the slice of __NEXT_DATA__ we read: the listing exposes positions, a
// detail page exposes the single job under data.
type talenthrNextData struct {
	Props struct {
		PageProps struct {
			Positions []talenthrListItem `json:"positions"`
			Data      *talenthrJob       `json:"data"`
		} `json:"pageProps"`
	} `json:"props"`
}

// talenthrListItem is one posting from the listing: the id and slug that build its detail URL.
type talenthrListItem struct {
	ID   json.Number `json:"id"`
	Slug string      `json:"slug"`
}

// talenthrJob is a detail page's job record. job_description is the full HTML body; is_remote
// and employment_type carry structured work-mode signal; employment_status_name is the
// full-time/part-time enum; publish_date (created_at fallback) is the post date.
type talenthrJob struct {
	Title                string `json:"job_position_title"`
	Description          string `json:"job_description"`
	IsRemote             bool   `json:"is_remote"`
	EmploymentType       string `json:"employment_type"`
	EmploymentStatusName string `json:"employment_status_name"`
	City                 string `json:"city"`
	Region               string `json:"region"`
	Country              string `json:"country"`
	LocationName         string `json:"location_name"`
	LocationAddress      string `json:"location_address"`
	PublishDate          string `json:"publish_date"`
	CreatedAt            string `json:"created_at"`
}

func (s talenthr) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	listURL := talenthrBaseURL + "/" + e.Board
	body, err := s.http.GetText(ctx, listURL)
	if err != nil {
		return nil, fmt.Errorf("talenthr: listing %s: %w", e.Board, err)
	}
	raw, ok := bracketSlice(body, "__NEXT_DATA__", '{', '}')
	if !ok {
		return nil, fmt.Errorf("talenthr: no __NEXT_DATA__ in listing %s", e.Board)
	}
	var data talenthrNextData
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		return nil, fmt.Errorf("talenthr: decode listing %s: %w", e.Board, err)
	}

	// Each posting's body and structured fields come from its own detail page, fanned out
	// under a bounded pool.
	return fetchDetails(data.Props.PageProps.Positions, defaultDetailWorkers, func(it talenthrListItem) (Job, bool) {
		return s.detail(ctx, e, it)
	}), nil
}

// detail fetches one position's detail page and maps its record to a Job, returning ok=false
// when the id/slug is missing, the fetch fails, or the page carries no job — so the caller
// skips just that posting.
func (s talenthr) detail(ctx context.Context, e CompanyEntry, it talenthrListItem) (Job, bool) {
	id := strings.TrimSpace(it.ID.String())
	slug := strings.TrimSpace(it.Slug)
	if id == "" || id == "0" || slug == "" {
		return Job{}, false // no stable id/slug → can't address the posting; skip it
	}
	jobURL := fmt.Sprintf("%s/%s/%s/%s", talenthrBaseURL, e.Board, slug, id)
	body, err := s.http.GetText(ctx, jobURL)
	if err != nil {
		return Job{}, false
	}
	raw, ok := bracketSlice(body, "__NEXT_DATA__", '{', '}')
	if !ok {
		return Job{}, false
	}
	var data talenthrNextData
	if json.Unmarshal([]byte(raw), &data) != nil {
		return Job{}, false
	}
	j := data.Props.PageProps.Data
	if j == nil || strings.TrimSpace(j.Title) == "" {
		return Job{}, false
	}

	location := talenthrLocation(j)
	workMode := talenthrWorkMode(j.EmploymentType, j.IsRemote)
	return Job{
		ExternalID:     id,
		URL:            jobURL,
		Title:          strings.TrimSpace(j.Title),
		Company:        e.Company,
		Location:       location,
		Description:    sanitizeHTML(j.Description),
		Remote:         workMode == "remote" || isRemote(location),
		WorkMode:       workMode,
		EmploymentType: talenthrEmploymentType(j.EmploymentStatusName),
		PostedAt:       parseLayout("2006-01-02 15:04:05", firstNonEmpty(j.PublishDate, j.CreatedAt)),
	}, true
}

// talenthrLocation composes the free-text location from the structured parts, most specific
// first, falling back to the location name/address the tenant configured.
func talenthrLocation(j *talenthrJob) string {
	var parts []string
	for _, p := range []string{j.City, j.Region, j.Country} {
		if p = strings.TrimSpace(p); p != "" {
			parts = append(parts, p)
		}
	}
	if len(parts) > 0 {
		return strings.Join(parts, ", ")
	}
	return firstNonEmpty(strings.TrimSpace(j.LocationAddress), strings.TrimSpace(j.LocationName))
}

// talenthrWorkMode maps TalentHR's employment_type work-arrangement enum onto freehire's
// work-mode vocabulary, falling back to the is_remote flag; "" when neither is decisive so the
// pipeline's location heuristic decides.
func talenthrWorkMode(employmentType string, isRemote bool) string {
	switch strings.ToLower(strings.TrimSpace(employmentType)) {
	case "remote":
		return "remote"
	case "hybrid":
		return "hybrid"
	case "onsite", "on-site", "on site":
		return "onsite"
	}
	return workModeFromRemote(isRemote)
}

// talenthrEmploymentType maps TalentHR's employment_status_name enum onto the freehire
// vocabulary, returning "" for an unknown/absent value so the description parser decides.
func talenthrEmploymentType(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "full-time", "full time", "fulltime":
		return "full_time"
	case "part-time", "part time", "parttime":
		return "part_time"
	case "contract", "contractor", "temporary", "freelance":
		return "contract"
	case "internship", "intern", "trainee":
		return "internship"
	}
	return ""
}

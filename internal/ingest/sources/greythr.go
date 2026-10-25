package sources

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
)

// greythr adapts greytHR Hire career sites. greytHR is the payroll and HRMS most Indian small
// and mid-sized employers run, and its recruiting module serves each tenant's openings at
// <tenant>.greythr.com/hire/jobs/. The board is that whole host, as with Darwinbox: the same host
// is the tenant's employee HRMS login.
//
// Two open JSON endpoints on the tenant host carry everything (measured 2026-10-25 across 57
// tenants, 843 postings): POST /hire/api/career/published_jobs/ answers every published opening
// in one unpaginated array, description included, so there is no detail request; and GET
// /hire/api/greythr/emp-category/ maps the opening's location ids to the tenant's own names
// ("519" -> "KARAIKUDI").
type greythr struct {
	http greythrHTTP
}

type greythrHTTP interface {
	JSONGetter
	JSONPoster
}

// NewGreytHR builds the greytHR Hire adapter over the given client.
func NewGreytHR(c greythrHTTP) Source { return greythr{http: c} }

func (greythr) Provider() string { return "greythr" }

// fullBoardListing: published_jobs returns the board's whole set of published openings in one
// response, so a listing failure fails the crawl rather than leaving a partial board.
func (greythr) fullBoardListing() {}

// greythrHostPattern is the shape of a board: one tenant label on greythr.com. The adapter builds
// its request URLs from the board, so nothing else may pass.
var greythrHostPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*\.greythr\.com$`)

type greythrPosting struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Slug        string   `json:"slug"`
	Locations   []string `json:"locations"`
	Description string   `json:"description"`
	JobType     string   `json:"job_type"`
	IsRemote    bool     `json:"is_remote"`
	ApplyURL    string   `json:"apply_url"`
	PublishedOn string   `json:"published_on_career_page"`
	CreatedAt   string   `json:"created_at"`
}

func (s greythr) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	if !greythrHostPattern.MatchString(e.Board) {
		return nil, fmt.Errorf("greythr: board %q is not a <tenant>.greythr.com host", e.Board)
	}
	base := "https://" + e.Board + "/hire"

	var listing struct {
		Data *[]greythrPosting `json:"data"`
	}
	if err := s.http.PostJSON(ctx, base+"/api/career/published_jobs/", map[string]any{}, &listing); err != nil {
		// A label no tenant holds, or a tenant without the Hire module, answers 404.
		var se *StatusError
		if errors.As(err, &se) && se.Code == http.StatusNotFound {
			return nil, fmt.Errorf("greythr: %s: %w", e.Board, ErrBoardGone)
		}
		return nil, fmt.Errorf("greythr: list %s: %w", e.Board, err)
	}
	if listing.Data == nil {
		return nil, fmt.Errorf("greythr: %s: answer carries no data", e.Board)
	}

	// Best effort: a tenant whose category map cannot be read still has its openings, and the
	// location is then left to the description parser.
	var cats struct {
		Value map[string]struct {
			Name string `json:"name"`
		} `json:"value"`
	}
	if err := s.http.GetJSON(ctx, base+"/api/greythr/emp-category/", &cats); err != nil {
		cats.Value = nil
	}

	jobs := make([]Job, 0, len(*listing.Data))
	for _, p := range *listing.Data {
		if p.ID == "" {
			continue
		}
		var names []string
		for _, id := range p.Locations {
			if n := strings.TrimSpace(cats.Value[id].Name); n != "" {
				names = append(names, n)
			}
		}
		jobs = append(jobs, greythrJob(base, e.Company, p, strings.Join(names, "; ")))
	}
	return jobs, nil
}

func greythrJob(base, company string, p greythrPosting, location string) Job {
	url := p.ApplyURL
	if url == "" {
		url = base + "/jobs/" + p.Slug
	}
	posted := p.PublishedOn
	if posted == "" {
		posted = p.CreatedAt
	}
	j := Job{
		ExternalID:     p.ID,
		URL:            url,
		Title:          strings.TrimSpace(p.Title),
		Company:        company,
		Location:       location,
		Description:    sanitizeHTML(p.Description),
		EmploymentType: greythrEmploymentType(p.JobType),
		PostedAt:       parseRFC3339(posted),
	}
	// is_remote is a structured flag, but false is also what a tenant that never sets it serves,
	// so only true is a statement about the work arrangement.
	if p.IsRemote {
		j.WorkMode = "remote"
		j.Remote = true
	} else {
		j.Remote = isRemote(location) || isRemote(j.Title)
	}
	return j
}

// greythrEmploymentType maps the platform's job_type picklist; measured values: Full-time,
// Internship, Contract, Direct Contract, Part-time.
func greythrEmploymentType(t string) string {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "full-time":
		return "full_time"
	case "part-time":
		return "part_time"
	case "internship":
		return "internship"
	case "contract", "direct contract":
		return "contract"
	}
	return ""
}

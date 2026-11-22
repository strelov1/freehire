package ats

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/strelov1/freehire/internal/dict/location"
	"github.com/strelov1/freehire/internal/ingest/sources/kit"
)

var (
	talentgladeBoardID = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)
	talentgladeJobID   = regexp.MustCompile(`^[0-9a-f]{32}$`)
)

type talentglade struct{ http kit.JSONGetter }

// NewTalentglade reads the JSON API that powers the public career site and application page.
func NewTalentglade(c kit.JSONGetter) kit.Source { return talentglade{http: c} }

func (talentglade) Provider() string { return "talentglade" }

func (talentglade) FullBoardListing() {}

// Empty listings lack a verified distinction from unknown tenants, so there is no
// EmptyBoardListing marker.
func (talentglade) ValidateBoard(board string) error {
	if !talentgladeBoardID.MatchString(board) || board == "www" {
		return fmt.Errorf("talentglade: invalid tenant %q", board)
	}
	return nil
}

type talentgladeCompany struct {
	Slug  string `json:"slug"`
	Name  string `json:"name"`
	Brand string `json:"brand_name"`
}

type talentgladePosting struct {
	ID          string `json:"public_id"`
	Title       string `json:"title"`
	City        string `json:"city"`
	Country     string `json:"country_code"`
	Department  string `json:"department"`
	PublishedAt string `json:"published_at"`
	Workplace   string `json:"workplace_type"`
}

func (s talentglade) Fetch(ctx context.Context, entry kit.CompanyEntry) ([]kit.Job, error) {
	return s.FetchNew(ctx, entry, func(string) bool { return false })
}

func (s talentglade) FetchNew(ctx context.Context, entry kit.CompanyEntry, seen func(string) bool) ([]kit.Job, error) {
	if err := s.ValidateBoard(entry.Board); err != nil {
		return nil, err
	}
	host := "https://" + entry.Board + ".talentglade.com"
	api := host + "/__careers?company_slug=" + entry.Board
	var listing struct {
		Company talentgladeCompany   `json:"company"`
		Jobs    []talentgladePosting `json:"jobs"`
		Total   *int                 `json:"total"`
	}
	if err := s.http.GetJSON(ctx, api+"&type=jobs", &listing); err != nil {
		return nil, fmt.Errorf("talentglade: list %s: %w", entry.Board, err)
	}
	if listing.Company.Slug != entry.Board || listing.Jobs == nil || listing.Total == nil || len(listing.Jobs) != *listing.Total {
		return nil, fmt.Errorf("talentglade: incomplete listing for %s", entry.Board)
	}
	company := strings.TrimSpace(listing.Company.Brand)
	if company == "" {
		company = strings.TrimSpace(listing.Company.Name)
	}
	if company == "" {
		return nil, fmt.Errorf("talentglade: missing employer name for %s", entry.Board)
	}

	ids := make(map[string]bool, len(listing.Jobs))
	for _, posting := range listing.Jobs {
		if !talentgladeJobID.MatchString(posting.ID) || ids[posting.ID] {
			return nil, fmt.Errorf("talentglade: invalid or repeated job ID for %s", entry.Board)
		}
		ids[posting.ID] = true
	}
	jobs := kit.FetchDetails(listing.Jobs, kit.DefaultDetailWorkers, func(posting talentgladePosting) (kit.Job, bool) {
		job := kit.Job{ExternalID: posting.ID, URL: host + "/apply/" + posting.ID, Title: posting.Title, Company: company}
		if seen(posting.ID) {
			job.SeenRefresh = true
			return job, true
		}
		var detail struct {
			talentgladePosting
			Company         talentgladeCompany `json:"company"`
			DescriptionHTML string             `json:"description_html"`
			ClosedTitle     string             `json:"closed_title"`
		}
		if err := s.http.GetJSON(ctx, api+"&type=job&job_id="+posting.ID, &detail); err != nil {
			job.Unreadable = kit.DetailUnreadable(err)
			return job, job.Unreadable
		}
		// A closed job can return HTTP 200 with a different active posting embedded.
		if detail.Company.Slug == entry.Board && detail.ClosedTitle != "" {
			return kit.Job{}, false
		}
		job.Title = strings.TrimSpace(detail.Title)
		job.Description = kit.SanitizeHTML(detail.DescriptionHTML)
		if detail.ID != posting.ID || detail.Company.Slug != entry.Board || job.Title == "" || job.Description == "" {
			job.Unreadable = true
			return job, true
		}
		job.Location = detail.City
		job.Team = detail.Department
		job.PostedAt = kit.ParseRFC3339(detail.PublishedAt)
		job.WorkMode = kit.WorkplaceTypeMode(detail.Workplace)
		job.Remote = job.WorkMode == "remote"
		if country := location.NormalizeCountry(detail.Country); country != "" {
			job.Countries = []string{country}
		}
		return job, true
	})
	for _, job := range jobs {
		if !job.Unreadable {
			return jobs, nil
		}
	}
	if len(jobs) > 0 {
		return nil, fmt.Errorf("talentglade: no readable postings for %s", entry.Board)
	}
	return jobs, nil
}

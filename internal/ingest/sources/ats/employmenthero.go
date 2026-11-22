package ats

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/strelov1/freehire/internal/ingest/sources/kit"
)

const (
	employmentHeroAPI      = "https://services.employmenthero.com/ats/api/v1/career_page/organisations/"
	employmentHeroJobURL   = "https://employmenthero.com/jobs/position/"
	employmentHeroPageSize = 50
)

var employmentHeroSlug = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

type employmentHero struct{ http kit.JSONGetter }

// NewEmploymentHero reads one employer's public jobs from Employment Hero.
func NewEmploymentHero(c kit.JSONGetter) kit.Source { return employmentHero{http: c} }

func (employmentHero) Provider() string { return "employmenthero" }

func (employmentHero) FullBoardListing() {}

// Unknown organisations return HTTP 404. Successful responses from known
// employers with zero jobs have not been verified, so there is no
// EmptyBoardListing marker.

func (employmentHero) ValidateBoard(board string) error {
	if !employmentHeroSlug.MatchString(board) {
		return fmt.Errorf("employmenthero: invalid employer slug %q", board)
	}
	return nil
}

type employmentHeroPosting struct {
	ID          string `json:"job_id"`
	Slug        string `json:"friendly_id"`
	Board       string `json:"organisation_friendly_id"`
	Company     string `json:"organisation_name"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Location    string `json:"vendor_location_name"`
	Country     string `json:"country_code"`
	Team        string `json:"team_name"`
	PostedAt    string `json:"created_at"`
	Workplace   string `json:"workplace_type"`
}

func (s employmentHero) Fetch(ctx context.Context, entry kit.CompanyEntry) ([]kit.Job, error) {
	if err := s.ValidateBoard(entry.Board); err != nil {
		return nil, err
	}
	var jobs []kit.Job
	var total int
	seen := make(map[string]bool)
	for page := 1; ; page++ {
		var response struct {
			Data struct {
				Index json.Number             `json:"page_index"`
				Total json.Number             `json:"total_items"`
				Items []employmentHeroPosting `json:"items"`
			} `json:"data"`
		}
		u := fmt.Sprintf("%s%s/jobs?page_index=%d&item_per_page=%d", employmentHeroAPI, entry.Board, page, employmentHeroPageSize)
		if err := s.http.GetJSON(ctx, u, &response); err != nil {
			return nil, fmt.Errorf("employmenthero: list %s page %d: %w", entry.Board, page, err)
		}
		batch := response.Data
		index, indexErr := strconv.Atoi(batch.Index.String())
		pageTotal, totalErr := strconv.Atoi(batch.Total.String())
		count := len(jobs) + len(batch.Items)
		if indexErr != nil || totalErr != nil || batch.Items == nil || index != page ||
			count > pageTotal || (len(batch.Items) == 0 && count != pageTotal) ||
			(page > 1 && pageTotal != total) {
			return nil, fmt.Errorf("employmenthero: incomplete listing for %s at page %d", entry.Board, page)
		}
		total = pageTotal
		for _, posting := range batch.Items {
			if posting.Board != entry.Board || !employmentHeroSlug.MatchString(posting.Slug) ||
				strings.TrimSpace(posting.ID) == "" || seen[posting.ID] {
				return nil, fmt.Errorf("employmenthero: invalid or repeated posting on %s page %d", entry.Board, page)
			}
			job := posting.job()
			if job.Company == "" || job.Title == "" || job.Description == "" {
				return nil, fmt.Errorf("employmenthero: incomplete posting %s for %s", posting.ID, entry.Board)
			}
			seen[posting.ID] = true
			jobs = append(jobs, job)
		}
		if count == total {
			return jobs, nil
		}
	}
}

func (p employmentHeroPosting) job() kit.Job {
	mode := kit.WorkplaceTypeMode(p.Workplace)
	if p.Workplace == "remote_anywhere" {
		mode = "remote"
	}
	return kit.Job{
		ExternalID:  p.ID,
		URL:         employmentHeroJobURL + p.Slug + "/",
		Title:       strings.TrimSpace(p.Title),
		Company:     strings.TrimSpace(p.Company),
		Description: kit.SanitizeHTML(p.Description),
		Location:    p.Location,
		Countries:   kit.CountryFromCode(p.Country),
		Team:        p.Team,
		PostedAt:    kit.ParseRFC3339(p.PostedAt),
		WorkMode:    mode,
		Remote:      mode == "remote",
	}
}

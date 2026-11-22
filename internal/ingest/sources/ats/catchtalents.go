package ats

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/strelov1/freehire/internal/dict/location"
	"github.com/strelov1/freehire/internal/ingest/sources/kit"
)

const (
	catchTalentsBoard = "https://jobboard.catch-talents.de/companies/"
	catchTalentsAPI   = "https://api.catch-talents.de/jobboard/"
	catchTalentsLimit = 25
)

type catchTalents struct {
	http kit.JSONGetter
}

// NewCatchTalents reads one employer's public Catch Talents listings and vacancies.
func NewCatchTalents(c kit.JSONGetter) kit.Source { return catchTalents{http: c} }

func (catchTalents) Provider() string { return "catchtalents" }

func (catchTalents) FullBoardListing() {}

// An unknown tenant can return the same empty listing as a quiet one, so this
// adapter deliberately does not advertise EmptyBoardListing.

func (catchTalents) ValidateBoard(board string) error {
	id, err := strconv.ParseUint(board, 10, 64)
	if err != nil || id == 0 || len(board) > 18 || strconv.FormatUint(id, 10) != board {
		return fmt.Errorf("catchtalents: invalid company ID %q", board)
	}
	return nil
}

type catchListing struct {
	ID        uint64 `json:"id"`
	CompanyID uint64 `json:"companyID"`
	Title     string `json:"jobTitle"`
	Company   string `json:"companyTitle"`
}

type catchPage struct {
	Results []catchListing `json:"results"`
	Total   *int           `json:"total"`
}

type catchDetail struct {
	catchListing
	Open          *bool  `json:"open"`
	Description   string `json:"description"`
	PostingTime   string `json:"postingTime"`
	WorkingModel  string `json:"workingModel"`
	WorkLocations []struct {
		City        string `json:"city"`
		CountryCode string `json:"countryCode"`
	} `json:"workLocations"`
}

func (s catchTalents) Fetch(ctx context.Context, entry kit.CompanyEntry) ([]kit.Job, error) {
	return s.FetchNew(ctx, entry, func(string) bool { return false })
}

func (s catchTalents) FetchNew(ctx context.Context, entry kit.CompanyEntry, seen func(string) bool) ([]kit.Job, error) {
	if err := s.ValidateBoard(entry.Board); err != nil {
		return nil, err
	}
	listed, err := s.list(ctx, entry.Board)
	if err != nil || len(listed) == 0 {
		return nil, err
	}
	jobs := kit.FetchDetails(listed, kit.DefaultDetailWorkers, func(listing catchListing) (kit.Job, bool) {
		id := strconv.FormatUint(listing.ID, 10)
		job := kit.Job{
			ExternalID: id,
			URL:        catchTalentsBoard + entry.Board + "/jobs/" + id,
			Title:      listing.Title,
			Company:    listing.Company,
		}
		if seen(id) {
			job.SeenRefresh = true
			return job, true
		}
		var detail catchDetail
		if err := s.http.GetJSON(ctx, catchTalentsAPI+"jobs/"+id, &detail); err != nil {
			job.Unreadable = kit.DetailUnreadable(err)
			return job, job.Unreadable
		}
		if detail.ID != listing.ID || detail.CompanyID != listing.CompanyID || detail.Open == nil {
			job.Unreadable = true
			return job, true
		}
		if !*detail.Open {
			return kit.Job{}, false
		}
		job.Title = strings.TrimSpace(detail.Title)
		job.Company = strings.TrimSpace(detail.Company)
		job.Description = kit.SanitizeHTML(detail.Description)
		if job.Title == "" || job.Company == "" || job.Description == "" {
			job.Unreadable = true
			return job, true
		}
		job.PostedAt = kit.ParseRFC3339(detail.PostingTime)
		switch detail.WorkingModel {
		case "on-site":
			job.WorkMode = "onsite"
		case "hybrid", "remote":
			job.WorkMode = detail.WorkingModel
		}
		job.Remote = job.WorkMode == "remote"
		var cities []string
		for _, workLocation := range detail.WorkLocations {
			cities = append(cities, workLocation.City)
			if country := location.NormalizeCountry(workLocation.CountryCode); country != "" {
				job.Countries = append(job.Countries, country)
			}
		}
		job.Location = kit.JoinNonEmpty(cities...)
		return job, true
	})
	if len(jobs) > 0 {
		for _, job := range jobs {
			if !job.Unreadable {
				return jobs, nil
			}
		}
		return nil, fmt.Errorf("catchtalents: no readable postings for company %s", entry.Board)
	}
	return jobs, nil
}

func (s catchTalents) list(ctx context.Context, board string) ([]catchListing, error) {
	companyID, _ := strconv.ParseUint(board, 10, 64)
	seen := make(map[uint64]bool)
	var listed []catchListing
	var total int
	for {
		var page catchPage
		u := fmt.Sprintf("%scompanies/%s/jobs?offset=%d&limit=%d", catchTalentsAPI, board, len(listed), catchTalentsLimit)
		if err := s.http.GetJSON(ctx, u, &page); err != nil {
			return nil, fmt.Errorf("catchtalents: list company %s: %w", board, err)
		}
		count := len(listed) + len(page.Results)
		if page.Results == nil || page.Total == nil || count > *page.Total ||
			(len(page.Results) == 0 && count != *page.Total) ||
			(len(listed) > 0 && *page.Total != total) {
			return nil, fmt.Errorf("catchtalents: incomplete listing for company %s", board)
		}
		total = *page.Total
		for _, item := range page.Results {
			if item.ID == 0 || item.CompanyID != companyID || seen[item.ID] {
				return nil, fmt.Errorf("catchtalents: invalid or repeated listing for company %s", board)
			}
			seen[item.ID] = true
			listed = append(listed, item)
		}
		if count == total {
			return listed, nil
		}
	}
}

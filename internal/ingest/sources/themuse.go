package sources

import (
	"context"
	"fmt"
	"strconv"
)

// themuse adapts themuse.com, a well-known-brands job aggregator (US-heavy). Boardless (one
// public API, no per-tenant board) and multi-company, so it stays in the source facet and
// takes each posting's company from the feed. The /api/public/jobs endpoint pages by page
// number and ships each posting's full description inline (no detail call).
//
// The response's page_count/total is NOT a walkable bound: the API rejects any page beyond ~99 with
// a 400 ("Value `page` is too high"), far below the reported count (the same surprise as WhatJobs'
// feed, see sources/AGENTS.md). Nothing is special-cased: per the paginated-walk convention (first
// page failing is a board error, a later page failing ends the walk with what was gathered), that
// 400 just ends the crawl with the partial catalogue.
type themuse struct {
	http JSONGetter
}

const themuseListURL = "https://www.themuse.com/api/public/jobs?page=%d"

// NewTheMuse builds the The Muse adapter over the given HTTP client.
func NewTheMuse(c JSONGetter) Source { return themuse{http: c} }

func (themuse) Provider() string { return "themuse" }

func (themuse) boardless() {}

func (themuse) aggregator() {}

// themuseResponse is one page: postings plus the (unreliable, see above) catalogue-wide page
// count used only as a defensive upper bound.
type themuseResponse struct {
	PageCount int              `json:"page_count"`
	Results   []themusePosting `json:"results"`
}

// themusePosting is one posting, body inline (no detail call). Levels carries the platform's
// own seniority taxonomy; a posting can list more than one, so the first is taken as primary.
type themusePosting struct {
	ID              int64  `json:"id"`
	Name            string `json:"name"`
	Contents        string `json:"contents"`
	PublicationDate string `json:"publication_date"`
	Locations       []struct {
		Name string `json:"name"`
	} `json:"locations"`
	Levels []struct {
		ShortName string `json:"short_name"`
	} `json:"levels"`
	Refs struct {
		LandingPage string `json:"landing_page"`
	} `json:"refs"`
	Company struct {
		Name string `json:"name"`
	} `json:"company"`
}

func (s themuse) Fetch(ctx context.Context, _ CompanyEntry) ([]Job, error) {
	var jobs []Job
	for page := 1; ; page++ {
		var resp themuseResponse
		url := fmt.Sprintf(themuseListURL, page)
		if err := s.http.GetJSON(ctx, url, &resp); err != nil {
			if page == 1 {
				return nil, fmt.Errorf("themuse: list page %d: %w", page, err)
			}
			return jobs, nil
		}
		for _, p := range resp.Results {
			if job, ok := p.toJob(); ok {
				jobs = append(jobs, job)
			}
		}
		if len(resp.Results) == 0 || (resp.PageCount > 0 && page >= resp.PageCount) {
			break
		}
	}
	return jobs, nil
}

// toJob maps an inline posting to a Job, returning ok=false for an unusable posting (no
// native id, or no company which would break the slug).
func (p themusePosting) toJob() (Job, bool) {
	if p.ID == 0 || p.Company.Name == "" {
		return Job{}, false
	}
	var location string
	if len(p.Locations) > 0 {
		location = p.Locations[0].Name
	}
	var seniority string
	if len(p.Levels) > 0 {
		seniority = museSeniority(p.Levels[0].ShortName)
	}
	return Job{
		ExternalID:  strconv.FormatInt(p.ID, 10),
		URL:         p.Refs.LandingPage,
		Title:       p.Name,
		Company:     p.Company.Name,
		Location:    location,
		Description: sanitizeHTML(p.Contents),
		Seniority:   seniority,
		PostedAt:    parseRFC3339(p.PublicationDate),
	}, true
}

// museSeniority maps The Muse's own level taxonomy ("entry", "mid", "senior") to freehire's
// controlled vocabulary. An unrecognized value maps to "" rather than a guess
// (vocab.SeniorityValues is dict-only).
func museSeniority(shortName string) string {
	switch shortName {
	case "entry":
		return "junior"
	case "mid":
		return "middle"
	case "senior":
		return "senior"
	default:
		return ""
	}
}

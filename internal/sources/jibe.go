package sources

import (
	"context"
	"fmt"
	"strings"
)

// jibe adapts Jibe career sites (the career-site product fronting iCIMS, e.g.
// github.careers). The board is the site's public host (e.g. "www.github.careers"),
// forming the JSON listing API "https://<board>/api/jobs". That list endpoint already
// carries the full posting HTML in "description", so there is no per-job detail fetch —
// the separate responsibilities/qualifications fields are redundant re-extractions of
// the same body (verified: every posting's description already contains them) and are
// intentionally not appended.

// jibePageLimit is the listing page size. The API reports the catalogue total, so the
// crawl pages until every posting is collected.
const jibePageLimit = 100

// jibeDateLayout is Jibe's posted_date format: RFC3339 with a numeric, colon-less zone
// offset ("2026-06-16T20:41:00+0000"), which time.RFC3339 cannot parse.
const jibeDateLayout = "2006-01-02T15:04:05-0700"

type jibe struct {
	http JSONGetter
}

// NewJibe builds the Jibe adapter over the given HTTP client.
func NewJibe(c JSONGetter) Source { return jibe{http: c} }

func (jibe) Provider() string { return "jibe" }

// jibeListing is one /api/jobs page: the postings plus the catalogue total used as the
// stop condition.
type jibeListing struct {
	Jobs []struct {
		Data jibePosting `json:"data"`
	} `json:"jobs"`
	TotalCount int `json:"totalCount"`
}

// jibePosting is one posting's "data" object; only the fields the Job shape needs are
// decoded.
type jibePosting struct {
	Slug               string `json:"slug"`
	ReqID              string `json:"req_id"`
	Title              string `json:"title"`
	Description        string `json:"description"`
	LocationName       string `json:"location_name"`
	FullLocation       string `json:"full_location"`
	LocationType       string `json:"location_type"`
	HiringOrganization string `json:"hiring_organization"`
	PostedDate         string `json:"posted_date"`
}

func (s jibe) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	var jobs []Job
	for page := 1; ; page++ {
		url := fmt.Sprintf("https://%s/api/jobs?page=%d&limit=%d", e.Board, page, jibePageLimit)
		var listing jibeListing
		if err := s.http.GetJSON(ctx, url, &listing); err != nil {
			return nil, fmt.Errorf("jibe: list board %s: %w", e.Board, err)
		}
		if len(listing.Jobs) == 0 {
			break
		}
		for _, item := range listing.Jobs {
			if job, ok := s.toJob(e, item.Data); ok {
				jobs = append(jobs, job)
			}
		}
		// Stop once the catalogue total is reached; the empty-page check above is the
		// fallback if totalCount is ever absent or wrong.
		if len(jobs) >= listing.TotalCount {
			break
		}
	}
	return jobs, nil
}

// toJob maps one Jibe posting to the normalized Job shape. The id is the posting slug
// (the public job-page path segment), falling back to req_id. A posting with neither has
// no dedup key — it would collide on a bare ".../jobs/" URL — so it is dropped (ok=false).
func (s jibe) toJob(e CompanyEntry, p jibePosting) (Job, bool) {
	id := firstNonEmpty(p.Slug, p.ReqID)
	if id == "" {
		return Job{}, false
	}
	location := firstNonEmpty(p.LocationName, p.FullLocation)
	// location_type is the structured work-arrangement signal (REMOTE/HYBRID/ONSITE);
	// "ANY" and unknown values yield no mode, leaving the pipeline to fall back to the
	// location string.
	workMode := workplaceTypeMode(p.LocationType)
	return Job{
		ExternalID:  id,
		URL:         fmt.Sprintf("https://%s/careers-home/jobs/%s", e.Board, id),
		Title:       strings.TrimSpace(p.Title),
		Company:     firstNonEmpty(p.HiringOrganization, e.Company),
		Location:    location,
		Description: sanitizeHTML(p.Description),
		Remote:      workMode == "remote" || isRemote(location),
		WorkMode:    workMode,
		PostedAt:    parseLayout(jibeDateLayout, p.PostedDate),
	}, true
}

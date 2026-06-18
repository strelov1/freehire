package sources

import (
	"context"
	"errors"
	"fmt"
	"html"
	"strings"
)

// usajobs adapts the USAJobs Search API (data.usajobs.gov), the U.S. federal government's
// official job board. Like the other aggregators it is boardless (one public API, no
// per-tenant board) yet lists many employers, so it stays in the source facet and takes
// each posting's hiring agency as the company. The list API carries every posting's body
// inline (UserArea.Details) and paginates by page number, so there is no per-posting
// detail request.
//
// It is the one KEYED source: the API is gated behind an Authorization-Key header, so the
// adapter is registered only when USAJOBS_API_KEY is configured (see All). The key is a
// secret and lives in the environment, never in a board file.
type usajobs struct {
	http   HeaderJSONGetter
	apiKey string
}

const (
	usajobsSearchURL = "https://data.usajobs.gov/api/search"
	// usajobsPageSize is the API's maximum page size; usajobsMaxPages bounds pagination
	// well above the API's ~10k open-results cap so a misbehaving feed cannot loop forever.
	usajobsPageSize = 500
	usajobsMaxPages = 50
	// usajobsDateLayout is the API's zoneless, fixed-fraction timestamp (e.g.
	// "2026-06-05T00:00:00.0000"); read as UTC, which is fine for an approximate posted_at.
	usajobsDateLayout = "2006-01-02T15:04:05.0000"
)

// NewUSAJobs builds the USAJobs adapter over the given HTTP client and API key.
func NewUSAJobs(c HeaderJSONGetter, apiKey string) Source {
	return usajobs{http: c, apiKey: apiKey}
}

func (usajobs) Provider() string { return "usajobs" }

// usajobs needs no board id (one API), so its config carries no board.
func (usajobs) boardless() {}

// usajobs aggregates postings from many federal agencies, so it stays in the source facet.
func (usajobs) aggregator() {}

// usajobsResponse is the envelope of the search API; only the items are read.
type usajobsResponse struct {
	SearchResult struct {
		SearchResultItems []usajobsItem `json:"SearchResultItems"`
	} `json:"SearchResult"`
}

// usajobsItem is one search hit: a stable id plus the descriptor holding the posting.
type usajobsItem struct {
	MatchedObjectID string            `json:"MatchedObjectId"`
	Descriptor      usajobsDescriptor `json:"MatchedObjectDescriptor"`
}

type usajobsDescriptor struct {
	PositionTitle           string `json:"PositionTitle"`
	PositionURI             string `json:"PositionURI"`
	PositionLocationDisplay string `json:"PositionLocationDisplay"`
	PositionLocation        []struct {
		LocationName string `json:"LocationName"`
	} `json:"PositionLocation"`
	OrganizationName  string `json:"OrganizationName"`
	DepartmentName    string `json:"DepartmentName"`
	PositionStartDate string `json:"PositionStartDate"`
	UserArea          struct {
		Details struct {
			JobSummary       string   `json:"JobSummary"`
			MajorDuties      []string `json:"MajorDuties"`
			Requirements     string   `json:"Requirements"`
			TeleworkEligible bool     `json:"TeleworkEligible"`
			RemoteIndicator  bool     `json:"RemoteIndicator"`
		} `json:"Details"`
	} `json:"UserArea"`
}

func (u usajobs) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	if u.apiKey == "" {
		return nil, errors.New("usajobs: missing API key (set USAJOBS_API_KEY)")
	}
	headers := map[string]string{"Authorization-Key": u.apiKey}

	var jobs []Job
	for page := 1; page <= usajobsMaxPages; page++ {
		url := fmt.Sprintf("%s?ResultsPerPage=%d&Page=%d", usajobsSearchURL, usajobsPageSize, page)
		var resp usajobsResponse
		if err := u.http.GetJSONWithHeaders(ctx, url, headers, &resp); err != nil {
			return nil, fmt.Errorf("usajobs: page %d: %w", page, err)
		}
		items := resp.SearchResult.SearchResultItems
		if len(items) == 0 {
			break
		}
		for _, it := range items {
			if job, ok := it.toJob(e); ok {
				jobs = append(jobs, job)
			}
		}
	}
	return jobs, nil
}

// toJob maps a search hit to a Job, returning ok=false for an unusable posting (no native
// id, which would collide on the dedup key, or no agency, which would break the company
// slug). The structured RemoteIndicator/TeleworkEligible flags set the work mode.
func (it usajobsItem) toJob(e CompanyEntry) (Job, bool) {
	d := it.Descriptor
	company := firstNonEmpty(d.OrganizationName, d.DepartmentName, e.Company)
	if it.MatchedObjectID == "" || company == "" {
		return Job{}, false
	}

	location := d.PositionLocationDisplay
	if len(d.PositionLocation) > 0 {
		// Prefer the first concrete location: a "Multiple Locations" display is unparseable
		// by the location dictionary, while "Anchorage, Alaska" resolves to a country/region.
		location = firstNonEmpty(d.PositionLocation[0].LocationName, location)
	}

	remote := d.UserArea.Details.RemoteIndicator
	return Job{
		ExternalID:  it.MatchedObjectID,
		URL:         cleanUSAJobsURL(d.PositionURI),
		Title:       d.PositionTitle,
		Company:     company,
		Location:    location,
		Description: usajobsDescription(d),
		Remote:      remote,
		WorkMode:    usajobsWorkMode(remote, d.UserArea.Details.TeleworkEligible),
		PostedAt:    parseLayout(usajobsDateLayout, d.PositionStartDate),
	}, true
}

// usajobsWorkMode maps the API's structured remote signals to our work-mode vocabulary:
// a full-remote posting is "remote"; a telework-eligible (part-remote) one is "hybrid";
// otherwise "onsite". Both flags are explicit booleans, so this is structured signal only.
func usajobsWorkMode(remote, telework bool) string {
	switch {
	case remote:
		return "remote"
	case telework:
		return "hybrid"
	default:
		return "onsite"
	}
}

// usajobsDescription assembles the plain-text JobSummary, major duties, and requirements
// into HTML paragraphs (escaping the source text), then sanitizes the result like every
// other adapter's body.
func usajobsDescription(d usajobsDescriptor) string {
	var b strings.Builder
	det := d.UserArea.Details
	writePara := func(s string) {
		if strings.TrimSpace(s) != "" {
			b.WriteString("<p>" + html.EscapeString(s) + "</p>")
		}
	}
	writePara(det.JobSummary)
	for _, duty := range det.MajorDuties {
		writePara(duty)
	}
	writePara(det.Requirements)
	return sanitizeHTML(b.String())
}

// cleanUSAJobsURL drops the explicit :443 default-port noise the API emits in PositionURI
// (https://www.usajobs.gov:443/job/<id> → https://www.usajobs.gov/job/<id>).
func cleanUSAJobsURL(u string) string {
	return strings.Replace(u, ":443/", "/", 1)
}

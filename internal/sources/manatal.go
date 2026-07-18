package sources

import (
	"context"
	"fmt"
	"net/url"
)

// manatal adapts Manatal career pages (the recruitment ATS behind careers-page.com). The
// board is the tenant's career-page slug (e.g. "davidjoseph-co"), which keys the public,
// keyless list endpoint https://open.api.manatal.com/open/v3/career-page/<slug>/jobs/. That
// endpoint carries every posting's full HTML description and its structured location,
// remote flag, and contract type inline, so there is no per-posting detail request; it
// paginates DRF-style via a top-level "next" link, walked until it is null.
//
// This is the JSON path for the same platform careers-page.com serves as server-rendered
// HTML: the careerspage adapter crawls a tenant's <slug>.careers-page.com pages and fetches
// each posting's ld+json detail, whereas this reads the whole board from one paginated JSON
// feed with the description and facets already inline — so a Manatal tenant is best onboarded
// here rather than under careerspage.
type manatal struct {
	http JSONGetter
}

// NewManatal builds the Manatal adapter over the given HTTP client.
func NewManatal(c JSONGetter) Source { return manatal{http: c} }

func (manatal) Provider() string { return "manatal" }

const (
	// manatalListURL is the public career-page jobs list, taking the tenant slug. It returns
	// a DRF page {count,next,previous,results}; the next link carries ?page=N.
	manatalListURL = "https://open.api.manatal.com/open/v3/career-page/%s/jobs/"
	// manatalJobURL is the public human-facing posting page, keyed by the tenant slug and the
	// posting hash (the id that appears in the career-page URL).
	manatalJobURL = "https://www.careers-page.com/%s/job/%s"
	// manatalMaxPages bounds the next-link walk so a feed that never stops handing out a next
	// page cannot loop forever (the largest boards seen are ~10 pages; this is ample headroom).
	manatalMaxPages = 200
)

// manatalPosting is one job from the career-page list, with its body and facets inline.
type manatalPosting struct {
	Hash            string `json:"hash"`
	PositionName    string `json:"position_name"`
	Description     string `json:"description"`
	Country         string `json:"country"`
	State           string `json:"state"`
	City            string `json:"city"`
	LocationDisplay string `json:"location_display"`
	// IsRemote is the platform's structured remote flag; it is null for postings that leave
	// it unset, which decodes to false — indistinguishable from an explicit "not remote", and
	// treated the same (no structured remote signal).
	IsRemote        bool   `json:"is_remote"`
	ContractDetails string `json:"contract_details"`
}

func (m manatal) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	next := fmt.Sprintf(manatalListURL, url.PathEscape(e.Board))
	var jobs []Job
	for page := 0; page < manatalMaxPages && next != ""; page++ {
		var resp struct {
			Next    string           `json:"next"`
			Results []manatalPosting `json:"results"`
		}
		if err := m.http.GetJSON(ctx, next, &resp); err != nil {
			return nil, fmt.Errorf("manatal: board %s page %d: %w", e.Board, page, err)
		}
		for _, p := range resp.Results {
			if job, ok := p.toJob(e); ok {
				jobs = append(jobs, job)
			}
		}
		next = resp.Next
	}
	return jobs, nil
}

// toJob maps a posting to a Job, returning ok=false when it has no hash — the hash is the
// posting's public id and the dedup key, so a hashless posting (which would collide on the
// (source, external_id) key) is dropped. The configured company is canonical: organization_name
// is the tenant's internal department, not the employer, so it never labels the job.
func (p manatalPosting) toJob(e CompanyEntry) (Job, bool) {
	if p.Hash == "" {
		return Job{}, false
	}
	location := firstNonEmpty(p.LocationDisplay, joinNonEmpty(p.City, p.State, p.Country))
	return Job{
		ExternalID:     p.Hash,
		URL:            fmt.Sprintf(manatalJobURL, url.PathEscape(e.Board), p.Hash),
		Title:          p.PositionName,
		Company:        e.Company,
		Location:       location,
		Description:    sanitizeHTML(p.Description),
		Remote:         p.IsRemote || isRemote(location),
		WorkMode:       workModeFromRemote(p.IsRemote),
		EmploymentType: manatalEmploymentType(p.ContractDetails),
		// The API carries no publish date, so posted_at is left unset (nullable).
	}, true
}

// manatalEmploymentType maps Manatal's contract_details onto the freehire employment-type
// vocabulary, returning "" for an unset/unknown value so the description parser decides.
func manatalEmploymentType(t string) string {
	switch t {
	case "full_time":
		return "full_time"
	case "part_time":
		return "part_time"
	case "contract", "temporary", "freelance":
		return "contract"
	case "internship":
		return "internship"
	default:
		return ""
	}
}

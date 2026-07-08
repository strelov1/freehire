package sources

import (
	"context"
	"fmt"
)

// jobnet adapts Jobnet.dk — Denmark's official public job portal, operated by STAR
// (Styrelsen for Arbejdsmarked og Rekruttering). Its BFF search API (/bff/FindJob/Search,
// gated behind a non-secret "x-csrf: 1" header) returns a paginated catalogue where every
// ad carries its own employer and the full HTML description inline, so one paged crawl
// assembles every Job with no per-ad detail fetch. Like the other national feeds it is
// boardless (one national API, no per-tenant board) and an aggregator (many employers; the
// company comes from the ad, not the placeholder entry). It covers every sector, not just IT
// — the same as jobtech — so the downstream dictionaries and the enrich non-tech gate decide
// relevance, not the adapter.
type jobnet struct {
	http HeaderJSONGetter
}

const (
	// jobnetSearchURL pages the BFF search freshest-first. Keyless; the only gate is the
	// x-csrf header (see jobnetHeaders). An empty SearchString returns the whole catalogue.
	jobnetSearchURL = "https://jobnet.dk/bff/FindJob/Search?PageNumber=%d&ResultsPerPage=%d&OrderType=PublicationDate"
	// jobnetJobURL is the canonical posting page, built from the ad id when the ad carries no
	// destination URL of its own. The page sits behind the STAR login, but it is the ad's
	// stable public location; an ad with its own jobAdUrl (external postings) prefers that.
	jobnetJobURL = "https://job.jobnet.dk/CV/FindWork/Details/%s"
	// jobnetPageSize is the ResultsPerPage the API honours (100 confirmed against the live API).
	jobnetPageSize = 100
	// jobnetMaxPages bounds pagination so a wrong or missing totalJobAdCount cannot loop.
	// The empty page the API returns past the end is the real terminator; this is only the
	// runaway backstop, sized well above the live catalogue (~20k) so catalogue growth does
	// not silently truncate the crawl (a freshest-first crawl truncated at the tail would let
	// the unseen sweep close the oldest still-open ads).
	jobnetMaxPages = 600
)

// jobnetHeaders is the non-secret header the BFF requires; without it the API 400s.
var jobnetHeaders = map[string]string{"x-csrf": "1"}

// NewJobnet builds the Jobnet.dk adapter over the given header-capable JSON client.
func NewJobnet(c HeaderJSONGetter) Source { return jobnet{http: c} }

func (jobnet) Provider() string { return "jobnet" }

// jobnet is a national portal with one global feed, so its config entry carries no board.
func (jobnet) boardless() {}

// jobnet aggregates postings from many employers, so it stays in the source facet.
func (jobnet) aggregator() {}

// jobnetSearchResponse is one BFF search page: TotalJobAdCount is the catalogue size used to
// stop pagination; JobAds is the page. Only the fields the catalogue needs are decoded.
type jobnetSearchResponse struct {
	TotalJobAdCount int        `json:"totalJobAdCount"`
	JobAds          []jobnetAd `json:"jobAds"`
}

// jobnetAd is one posting. HiringOrgName is the employer (the portal lists many); Description
// is the full HTML body served inline on the list; JobAdURL is set only for external postings
// and, when present, is the real destination link.
type jobnetAd struct {
	JobAdID            string `json:"jobAdId"`
	Title              string `json:"title"`
	HiringOrgName      string `json:"hiringOrgName"`
	Description        string `json:"description"`
	PostalDistrictName string `json:"postalDistrictName"`
	Country            string `json:"country"`
	PublicationDate    string `json:"publicationDate"`
	JobAdURL           string `json:"jobAdUrl"`
}

// Fetch pages the BFF search freshest-first, stopping when the catalogue is exhausted (an
// empty page or the running count reaching totalJobAdCount). The first page failing is a
// board error; a later page failing ends enumeration with the jobs gathered so far.
func (s jobnet) Fetch(ctx context.Context, _ CompanyEntry) ([]Job, error) {
	var jobs []Job
	seen := 0
	for page := 1; page <= jobnetMaxPages; page++ {
		var resp jobnetSearchResponse
		url := fmt.Sprintf(jobnetSearchURL, page, jobnetPageSize)
		if err := s.http.GetJSONWithHeaders(ctx, url, jobnetHeaders, &resp); err != nil {
			if page == 1 {
				return nil, fmt.Errorf("jobnet: search page %d: %w", page, err)
			}
			break // a later page failing ends enumeration with the jobs gathered so far
		}
		if len(resp.JobAds) == 0 {
			break
		}
		seen += len(resp.JobAds)
		for _, a := range resp.JobAds {
			if j, ok := a.toJob(); ok {
				jobs = append(jobs, j)
			}
		}
		// Stop once the ads actually returned reach the reported catalogue size. Counting
		// returned ads (not page*pageSize) stays correct even if the API caps a page below
		// ResultsPerPage; the empty-page break above is the primary terminator, this only
		// saves the trailing empty request.
		if resp.TotalJobAdCount > 0 && seen >= resp.TotalJobAdCount {
			break
		}
	}
	return jobs, nil
}

// toJob maps an ad to a Job, returning ok=false for an ad with no id to key on or no employer
// name (which would break the company slug). Remote/WorkMode are left unset: the search
// response states no structured work arrangement, so the deterministic location dictionary
// derives it downstream from the location string rather than the adapter guessing.
func (a jobnetAd) toJob() (Job, bool) {
	if a.JobAdID == "" || a.HiringOrgName == "" {
		return Job{}, false
	}
	return Job{
		ExternalID: a.JobAdID,
		// External postings carry their own destination; jobnet-hosted ones fall back to the
		// canonical id-based posting page.
		URL:         firstNonEmpty(a.JobAdURL, fmt.Sprintf(jobnetJobURL, a.JobAdID)),
		Title:       a.Title,
		Company:     a.HiringOrgName,
		Description: sanitizeHTML(a.Description),
		Location:    joinNonEmpty(a.PostalDistrictName, a.Country),
		PostedAt:    parseRFC3339(a.PublicationDate),
	}, true
}

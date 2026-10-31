package sources

import (
	"context"
	"fmt"
	"net/url"
	"slices"
	"strconv"
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
//
// The search serves only the first jobnetWindow results of any query, and the catalogue is
// larger, so the crawl walks it in occupation slices that each fit the window (see Fetch). The
// walk reads the whole catalogue or fails, so jobnet is swept by source (fullCatalog).
type jobnet struct {
	http HeaderJSONGetter
}

const (
	// jobnetSearchURL is the BFF search endpoint. Keyless; the only gate is the x-csrf header
	// (see jobnetHeaders). With no filter it searches the whole catalogue.
	jobnetSearchURL = "https://jobnet.dk/bff/FindJob/Search"
	// jobnetJobURL is the canonical posting page, built from the ad id when the ad carries no
	// destination URL of its own. The page sits behind the STAR login, but it is the ad's
	// stable public location; an ad with its own jobAdUrl (external postings) prefers that.
	jobnetJobURL = "https://job.jobnet.dk/CV/FindWork/Details/%s"
	// jobnetPageSize is the most ResultsPerPage the API honours: 100 is served, 500 and 1000
	// are refused (measured 2026-10-31).
	jobnetPageSize = 100
	// jobnetWindow is how many results the search serves for any one query: with 23,169 ads
	// stated, page 100 of 100 was full and page 101 empty (measured 2026-10-31). The unfiltered
	// walk this replaced took that empty page for the catalogue's end, so it never listed the
	// oldest ~13k ads, and ~17k stored ones went unseen for a week while still shown.
	jobnetWindow = 10000
	// jobnetSliceAds bounds how many ads (by the facet's counts) one occupation slice may hold,
	// below the window so the ads published while a slice is walked still fit in it.
	jobnetSliceAds = 9000
	// jobnetSliceOccupations bounds how many occupation ids one slice names, keeping its URL a
	// few KB (an id is a 36-character GUID; 880 occupations were listed on 2026-10-31).
	jobnetSliceOccupations = 100
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

// fullCatalog: Fetch reads every occupation slice to its end and fails on any request failing,
// on a facet that does not account for the stated total, and on a slice that came back short, so
// a finished crawl has listed every ad the portal serves. See the fullCatalog interface
// (source.go) for the bar.
func (jobnet) fullCatalog() {}

// jobnetSearchResponse is one BFF search page: TotalJobAdCount is the size of what the query
// matched; JobAds is the page; the occupations facet counts the catalogue by occupation. Only
// the fields the crawl needs are decoded.
type jobnetSearchResponse struct {
	TotalJobAdCount int        `json:"totalJobAdCount"`
	JobAds          []jobnetAd `json:"jobAds"`
	SearchFacets    struct {
		Occupations []jobnetFacet `json:"occupations"`
	} `json:"searchFacets"`
}

// jobnetFacet is one occupation and how many ads carry it.
type jobnetFacet struct {
	Identifier string `json:"identifier"`
	JobAdCount int    `json:"jobAdCount"`
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

// Fetch lists the whole catalogue. The search serves only jobnetWindow results of a query, so
// the catalogue is read in slices: one request for the occupations facet (every ad carries
// exactly one occupation — the facet's counts summed to the stated total, 23,169, on
// 2026-10-31), the occupations packed into slices that each fit the window, and each slice
// walked freshest-first to its natural end, the first empty page. An ad that shifted across a
// page boundary while a slice was walked is kept once.
//
// jobnet is swept by source, so a partial list would close every ad the walk missed, and
// anything that leaves the read unproven fails the crawl: any request failing, an empty
// catalogue, a facet that does not account for the stated total, and a slice whose walk ended a
// whole page or more short of what its first page stated (the window cut it, or the listing
// stopped serving). Less than a page of shortfall is the drift of ads removed while the slice
// was walked, which the sweep's window absorbs.
func (s jobnet) Fetch(ctx context.Context, _ CompanyEntry) ([]Job, error) {
	var head jobnetSearchResponse
	if err := s.search(ctx, 1, 1, nil, &head); err != nil {
		return nil, err
	}
	if head.TotalJobAdCount <= 0 {
		return nil, fmt.Errorf("jobnet: the search states no ads")
	}
	counted := 0
	for _, o := range head.SearchFacets.Occupations {
		counted += o.JobAdCount
	}
	if counted < head.TotalJobAdCount {
		return nil, fmt.Errorf("jobnet: the occupations facet counts %d ads, not the %d the search states", counted, head.TotalJobAdCount)
	}

	seen := make(map[string]bool, head.TotalJobAdCount)
	var jobs []Job
	for _, occupations := range jobnetSlices(head.SearchFacets.Occupations, jobnetSliceAds, jobnetSliceOccupations) {
		read, stated := 0, 0
		for page := 1; ; page++ {
			var resp jobnetSearchResponse
			if err := s.search(ctx, page, jobnetPageSize, occupations, &resp); err != nil {
				return nil, err
			}
			if page == 1 {
				stated = resp.TotalJobAdCount
			}
			if len(resp.JobAds) == 0 {
				break
			}
			read += len(resp.JobAds)
			for _, a := range resp.JobAds {
				if seen[a.JobAdID] {
					continue
				}
				seen[a.JobAdID] = true
				if j, ok := a.toJob(); ok {
					jobs = append(jobs, j)
				}
			}
		}
		if read+jobnetPageSize <= stated {
			return nil, fmt.Errorf("jobnet: a slice of %d occupations listed %d of the %d ads it states", len(occupations), read, stated)
		}
	}
	return jobs, nil
}

// search requests one page of the search, filtered to the given occupations (none: the whole
// catalogue). The API reads a repeated occupations parameter as OR.
func (s jobnet) search(ctx context.Context, page, size int, occupations []string, v *jobnetSearchResponse) error {
	q := url.Values{
		"PageNumber":     {strconv.Itoa(page)},
		"ResultsPerPage": {strconv.Itoa(size)},
		"OrderType":      {"PublicationDate"},
	}
	if len(occupations) > 0 {
		q["occupations"] = occupations
	}
	if err := s.http.GetJSONWithHeaders(ctx, jobnetSearchURL+"?"+q.Encode(), jobnetHeaders, v); err != nil {
		return fmt.Errorf("jobnet: search page %d: %w", page, err)
	}
	return nil
}

// jobnetSlices packs the occupations, largest first, into slices of at most maxAds ads (by the
// facet's counts) and maxIDs occupations. An occupation larger than maxAds gets a slice of its
// own; if it outgrows the window, its walk comes back short and the crawl fails.
func jobnetSlices(facet []jobnetFacet, maxAds, maxIDs int) [][]string {
	sorted := slices.Clone(facet)
	slices.SortStableFunc(sorted, func(a, b jobnetFacet) int { return b.JobAdCount - a.JobAdCount })
	var out [][]string
	var cur []string
	ads := 0
	for _, o := range sorted {
		if len(cur) > 0 && (ads+o.JobAdCount > maxAds || len(cur) >= maxIDs) {
			out = append(out, cur)
			cur, ads = nil, 0
		}
		cur = append(cur, o.Identifier)
		ads += o.JobAdCount
	}
	if len(cur) > 0 {
		out = append(out, cur)
	}
	return out
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

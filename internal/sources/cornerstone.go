package sources

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// cornerstone adapts Cornerstone OnDemand (CSOD) career sites. The board is the tenant
// subdomain (e.g. "nintendoeurope" → nintendoeurope.csod.com). The careersite home shell
// embeds a JSON config blob carrying the regional API base and a JWT read token; with the
// token as a Bearer credential, the rec-job-search/external/jobs endpoint returns every
// requisition with its description inline, so no per-posting detail fetch is needed.
type cornerstoneHTTP interface {
	TextGetter
	HeaderJSONPoster
}

type cornerstone struct {
	http cornerstoneHTTP
}

// NewCornerstone builds the Cornerstone adapter over the given HTTP client.
func NewCornerstone(c cornerstoneHTTP) Source { return cornerstone{http: c} }

func (cornerstone) Provider() string { return "cornerstone" }

// cornerstonePageSize is the search page size; the loop paginates until totalCount is reached.
const cornerstonePageSize = 100

// cornerstoneLocation is one of a requisition's locations.
type cornerstoneLocation struct {
	City    string `json:"city"`
	Country string `json:"country"`
}

type cornerstoneRequisition struct {
	RequisitionID        int                   `json:"requisitionId"`
	DisplayJobTitle      string                `json:"displayJobTitle"`
	PostingEffectiveDate string                `json:"postingEffectiveDate"`
	Locations            []cornerstoneLocation `json:"locations"`
	ExternalDescription  string                `json:"externalDescription"`
}

type cornerstoneResponse struct {
	Data struct {
		TotalCount   int                      `json:"totalCount"`
		Requisitions []cornerstoneRequisition `json:"requisitions"`
	} `json:"data"`
}

func (s cornerstone) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	homeURL := fmt.Sprintf("https://%s.csod.com/ux/ats/careersite/1/home?c=%s", e.Board, e.Board)
	home, err := s.http.GetText(ctx, homeURL)
	if err != nil {
		return nil, fmt.Errorf("cornerstone: home %s: %w", e.Board, err)
	}
	cloud := firstSubmatch(cornerstoneCloudPattern, home)
	token := firstSubmatch(cornerstoneTokenPattern, home)
	if cloud == "" || token == "" {
		return nil, fmt.Errorf("cornerstone: no token/cloud endpoint on %s home", e.Board)
	}
	cultureID, _ := strconv.Atoi(firstSubmatch(cornerstoneCultureIDPattern, home))
	if cultureID == 0 {
		cultureID = 1
	}
	cultureName := firstSubmatch(cornerstoneCultureNamePattern, home)
	if cultureName == "" {
		cultureName = "en-US"
	}

	apiURL := strings.TrimRight(cloud, "/") + "/rec-job-search/external/jobs"
	headers := map[string]string{"Authorization": "Bearer " + token}

	var reqs []cornerstoneRequisition
	for page := 1; page <= 100; page++ {
		body := map[string]any{
			"careerSiteId":            1,
			"careerSitePageId":        1,
			"pageNumber":              page,
			"pageSize":                cornerstonePageSize,
			"cultureId":               cultureID,
			"cultureName":             cultureName,
			"searchText":              "",
			"states":                  []any{},
			"countryCodes":            []any{},
			"cities":                  []any{},
			"customFieldCheckboxKeys": []any{},
			"customFieldDropdowns":    []any{},
			"customFieldRadios":       []any{},
		}
		var resp cornerstoneResponse
		if err := s.http.PostJSONWithHeaders(ctx, apiURL, headers, body, &resp); err != nil {
			if page == 1 {
				return nil, fmt.Errorf("cornerstone: search %s: %w", e.Board, err)
			}
			break // a later page failing ends enumeration with what we have
		}
		if len(resp.Data.Requisitions) == 0 {
			break
		}
		reqs = append(reqs, resp.Data.Requisitions...)
		if len(reqs) >= resp.Data.TotalCount {
			break
		}
	}

	jobs := make([]Job, 0, len(reqs))
	for _, r := range reqs {
		if j, ok := s.toJob(r, e); ok {
			jobs = append(jobs, j)
		}
	}
	return jobs, nil
}

// toJob maps a requisition to a Job, returning ok=false when it has no id (which would
// collide on the dedup key). Company comes from config; the description is served inline.
func (cornerstone) toJob(r cornerstoneRequisition, e CompanyEntry) (Job, bool) {
	if r.RequisitionID == 0 {
		return Job{}, false
	}
	return Job{
		ExternalID: strconv.Itoa(r.RequisitionID),
		URL: fmt.Sprintf("https://%s.csod.com/ux/ats/careersite/1/home/requisition/%d?c=%s",
			e.Board, r.RequisitionID, e.Board),
		Title:       r.DisplayJobTitle,
		Company:     e.Company,
		Location:    cornerstonePrimaryLocation(r.Locations),
		Description: sanitizeHTML(r.ExternalDescription),
		PostedAt:    parseLayout("1/2/2006", r.PostingEffectiveDate),
	}, true
}

// cornerstonePrimaryLocation renders the first location as "City, Country", falling back to
// whichever of the two is present, or "" when there are none.
func cornerstonePrimaryLocation(locs []cornerstoneLocation) string {
	if len(locs) == 0 {
		return ""
	}
	var parts []string
	for _, p := range []string{locs[0].City, locs[0].Country} {
		if p = strings.TrimSpace(p); p != "" {
			parts = append(parts, p)
		}
	}
	return strings.Join(parts, ", ")
}

var (
	// cloud is read from inside the "endpoints" object and must be an absolute URL, so a
	// stray "cloud" key in an unrelated inline script can't be mistaken for the API base.
	cornerstoneCloudPattern       = regexp.MustCompile(`"endpoints"\s*:\s*\{[^}]*"cloud"\s*:\s*"(https?://[^"]+)"`)
	cornerstoneTokenPattern       = regexp.MustCompile(`"token"\s*:\s*"(eyJ[^"]+)"`)
	cornerstoneCultureIDPattern   = regexp.MustCompile(`(?i)"cultureID"\s*:\s*(\d+)`)
	cornerstoneCultureNamePattern = regexp.MustCompile(`(?i)"cultureName"\s*:\s*"([^"]+)"`)
)

// firstSubmatch returns the first capture group of pattern in s, or "".
func firstSubmatch(pattern *regexp.Regexp, s string) string {
	if m := pattern.FindStringSubmatch(s); m != nil {
		return m[1]
	}
	return ""
}

package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"net/url"
	"regexp"
	"strings"
)

// oraclePageLimit is the requisition-list page size (the API caps a page well above it).
const oraclePageLimit = 100

// oracleOffsetCap is the deepest offset findReqs serves: from offset 10000 on it answers an
// empty page whatever TotalJobsCount says (measured on AutoZone's tenant, 2026-11). It also
// ignores POSTING_DATES_ASC, so the tail cannot be read from the other end.
const oracleOffsetCap = 10000

// oracle adapts Oracle Recruiting Cloud (a.k.a. Oracle Fusion / ORC) career sites. The
// board id is the public host and site number, e.g.
// "fa-evmr-saasfaprod1.fa.ocs.oraclecloud.com/CX_1". Both endpoints are public GET JSON:
// the requisition list (which must expand requisitionList or it comes back empty) carries
// no description, so each requisition's detail is fetched to assemble it.
type oracle struct {
	http JSONGetter
}

// NewOracle builds the Oracle Recruiting Cloud adapter over the given HTTP client.
func NewOracle(c JSONGetter) Source { return oracle{http: c} }

var _ HydratingSource = oracle{}

func (oracle) Provider() string { return "oracle" }

// oracleBoard is a configured board split into the host and site number the endpoints need.
type oracleBoard struct {
	host, site string
}

// parseOracleBoard splits "host/site" (e.g. "acme.fa.ocs.oraclecloud.com/CX_1").
func parseOracleBoard(board string) (oracleBoard, error) {
	host, site, ok := strings.Cut(board, "/")
	if !ok || host == "" || site == "" {
		return oracleBoard{}, fmt.Errorf("oracle: board %q must be \"host/site\"", board)
	}
	return oracleBoard{host: host, site: site}, nil
}

// oracleRequisition is one item from the requisition list (no description here).
type oracleRequisition struct {
	ID                string `json:"Id"`
	Title             string `json:"Title"`
	PostedDate        string `json:"PostedDate"`
	PrimaryLocation   string `json:"PrimaryLocation"`
	WorkplaceTypeCode string `json:"WorkplaceTypeCode"`
}

// oracleFacetValue is one value of a findReqs facet with the number of requisitions under it.
// Id is a number for some facets and a string for others, so it is kept raw.
type oracleFacetValue struct {
	ID         json.RawMessage `json:"Id"`
	TotalCount int             `json:"TotalCount"`
}

// oraclePage is items[0] of a list response (items itself is a single search-result envelope).
// The facets are present only when the request asked for them (facetsList).
type oraclePage struct {
	TotalJobsCount  int                 `json:"TotalJobsCount"`
	RequisitionList []oracleRequisition `json:"requisitionList"`
	Categories      []oracleFacetValue  `json:"categoriesFacet"`
	Titles          []oracleFacetValue  `json:"titlesFacet"`
	Locations       []oracleFacetValue  `json:"locationsFacet"`
}

type oracleListResponse struct {
	Items []oraclePage `json:"items"`
}

func (s oracle) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return s.FetchNew(ctx, e, func(string) bool { return false })
}

// FetchNew lists the whole board but fetches a requisition's detail only when the catalogue
// does not hold it yet; a stored one is re-emitted as a liveness refresh. The detail request per
// requisition is what made a crawl of the big tenants outlast its shard's window.
func (s oracle) FetchNew(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	b, err := parseOracleBoard(e.Board)
	if err != nil {
		return nil, err
	}

	reqs, err := s.listRequisitions(ctx, b)
	if err != nil {
		return nil, err
	}

	return fetchDetails(reqs, defaultDetailWorkers, func(r oracleRequisition) (Job, bool) {
		if seen(r.ID) {
			return Job{ExternalID: r.ID, URL: oracleJobURL(b, r.ID), Company: e.Company, SeenRefresh: true}, true
		}
		return s.detail(ctx, e, b, r)
	}), nil
}

func oracleJobURL(b oracleBoard, id string) string {
	return fmt.Sprintf("https://%s/hcmUI/CandidateExperience/en/sites/%s/job/%s", b.host, b.site, id)
}

// listRequisitions reads the board newest-first and, when that yields fewer distinct
// requisitions than TotalJobsCount, completes it from facet slices (one filtered listing per
// category, then title, then location value), each under the offset cap. findReqs serves
// nothing past oracleOffsetCap, and requisitions sharing a posting date can shift between pages;
// the slices overlap each other, so every requisition is kept once.
// shortcut: Oracle names only the ten largest values of a facet, so a board whose remainder
// hides outside them (Dollar General, ~91K) stays short; that needs nested slicing.
func (s oracle) listRequisitions(ctx context.Context, b oracleBoard) ([]oracleRequisition, error) {
	first, all, err := s.listSlice(ctx, b, "")
	if err != nil {
		return nil, err
	}
	var reqs []oracleRequisition
	have := map[string]bool{}
	merge := func(page []oracleRequisition) {
		for _, r := range page {
			if !have[r.ID] {
				have[r.ID] = true
				reqs = append(reqs, r)
			}
		}
	}
	merge(all)
	facets := []struct {
		param  string
		values []oracleFacetValue
	}{
		{"selectedCategoriesFacet", first.Categories},
		{"selectedTitlesFacet", first.Titles},
		{"selectedLocationsFacet", first.Locations},
	}
	for _, f := range facets {
		for _, v := range f.values {
			if len(reqs) >= first.TotalJobsCount {
				return reqs, nil
			}
			if v.TotalCount > oracleOffsetCap {
				continue
			}
			id := strings.Trim(string(v.ID), `"`)
			_, slice, err := s.listSlice(ctx, b, ","+f.param+"="+url.PathEscape(id))
			if err != nil {
				return nil, err
			}
			merge(slice)
		}
	}
	return reqs, nil
}

// listSlice pages one listing (the whole board, or the facet filter appended to the finder)
// newest-first up to the offset cap, stopping early on an empty page or at TotalJobsCount. It
// returns the first page too, which carries the facets. The expand keeps requisitionList
// populated (without it the API returns an empty list under a non-zero total).
func (s oracle) listSlice(ctx context.Context, b oracleBoard, filter string) (oraclePage, []oracleRequisition, error) {
	var first oraclePage
	var reqs []oracleRequisition
	for offset := 0; offset < oracleOffsetCap; {
		facets := ""
		if offset == 0 {
			facets = ",facetsList=CATEGORIES%3BTITLES%3BLOCATIONS"
		}
		// offset must sit INSIDE the finder clause: Oracle ignores a top-level &offset=, which
		// would re-fetch the first page forever.
		u := fmt.Sprintf(
			"https://%s/hcmRestApi/resources/latest/recruitingCEJobRequisitions"+
				"?onlyData=true&expand=requisitionList.secondaryLocations,requisitionList.workLocation"+
				"&finder=findReqs;siteNumber=%s%s%s,limit=%d,offset=%d,sortBy=POSTING_DATES_DESC",
			b.host, b.site, facets, filter, oraclePageLimit, offset)
		var resp oracleListResponse
		if err := s.http.GetJSON(ctx, u, &resp); err != nil {
			return oraclePage{}, nil, fmt.Errorf("oracle: list site %s: %w", b.site, err)
		}
		if len(resp.Items) == 0 {
			break
		}
		page := resp.Items[0]
		if offset == 0 {
			first = page
		}
		if len(page.RequisitionList) == 0 {
			break
		}
		reqs = append(reqs, page.RequisitionList...)
		offset += len(page.RequisitionList)
		if offset >= page.TotalJobsCount {
			break
		}
	}
	return first, reqs, nil
}

// detail fetches one requisition's detail and maps it to a Job. A failed request yields an
// Unreadable marker, so the sweep does not read it as a removal; a 404/410 is a removal and is
// dropped.
func (s oracle) detail(ctx context.Context, e CompanyEntry, b oracleBoard, r oracleRequisition) (Job, bool) {
	url := fmt.Sprintf(
		"https://%s/hcmRestApi/resources/latest/recruitingCEJobRequisitionDetails"+
			"?onlyData=true&expand=all&finder=ById;Id=%%22%s%%22,siteNumber=%s",
		b.host, r.ID, b.site)

	var resp struct {
		Items []struct {
			ExternalDescriptionStr      string `json:"ExternalDescriptionStr"`
			ExternalResponsibilitiesStr string `json:"ExternalResponsibilitiesStr"`
			ExternalQualificationsStr   string `json:"ExternalQualificationsStr"`
			CorporateDescriptionStr     string `json:"CorporateDescriptionStr"`
			OrganizationDescriptionStr  string `json:"OrganizationDescriptionStr"`
			JobSchedule                 string `json:"JobSchedule"`
		} `json:"items"`
	}
	err := s.http.GetJSON(ctx, url, &resp)
	if err != nil && !detailUnreadable(err) {
		return Job{}, false
	}
	if err != nil || len(resp.Items) == 0 {
		return unreadableDetail(r.ID, oracleJobURL(b, r.ID), e.Company), true
	}
	d := resp.Items[0]
	// The three external fields are block-level fragments, so they concatenate directly.
	external := d.ExternalDescriptionStr + d.ExternalResponsibilitiesStr + d.ExternalQualificationsStr

	var description string
	if oracleHasVisibleText(external) {
		description = sanitizeHTML(external)
	} else {
		// Some requisition templates leave the External* fields empty ("<br>", "") and carry
		// the real body in these two.
		description = sanitizeHTML(d.CorporateDescriptionStr + d.OrganizationDescriptionStr)
	}

	workMode := oracleWorkMode(r.WorkplaceTypeCode)
	return Job{
		ExternalID:  r.ID,
		URL:         oracleJobURL(b, r.ID),
		Title:       r.Title,
		Company:     e.Company,
		Location:    r.PrimaryLocation,
		Description: description,
		Remote:      workMode == "remote" || isRemote(r.PrimaryLocation),
		WorkMode:    workMode,
		PostedAt:    parseDate(r.PostedDate),
		// JobSchedule is a structured enum, preferred over the free-text parse.
		EmploymentType: oracleEmploymentType(d.JobSchedule),
	}, true
}

// oracleTagPattern matches an HTML tag, to tell real External* content from bare markup.
var oracleTagPattern = regexp.MustCompile(`<[^>]*>`)

// oracleHasVisibleText reports whether s renders any text once its tags are stripped; failing
// it triggers the Corporate/Organization description fallback in detail.
func oracleHasVisibleText(s string) bool {
	return strings.TrimSpace(html.UnescapeString(oracleTagPattern.ReplaceAllString(s, ""))) != ""
}

// oracleEmploymentType maps Oracle's JobSchedule ("Full time" / "Part time"); anything else is
// "" so the description parser decides.
func oracleEmploymentType(jobSchedule string) string {
	switch strings.TrimSpace(strings.ToLower(jobSchedule)) {
	case "full time":
		return "full_time"
	case "part time":
		return "part_time"
	default:
		return ""
	}
}

// oracleWorkMode maps an ORC workplace-type code to our work-mode vocabulary; an empty or
// unrecognized code yields "".
func oracleWorkMode(code string) string {
	switch code {
	case "ORA_REMOTE":
		return "remote"
	case "ORA_HYBRID":
		return "hybrid"
	case "ORA_ON_SITE":
		return "onsite"
	default:
		return ""
	}
}

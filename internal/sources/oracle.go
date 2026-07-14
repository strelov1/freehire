package sources

import (
	"context"
	"fmt"
	"strings"
)

// oraclePageLimit is the requisition-list page size. The API caps a page well above this;
// a modest page keeps each request cheap and the pagination loop simple.
const oraclePageLimit = 100

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

// oracleListResponse wraps the page. The requisitions and the total both nest under
// items[0] (items itself is a single search-result envelope).
type oracleListResponse struct {
	Items []struct {
		TotalJobsCount  int                 `json:"TotalJobsCount"`
		RequisitionList []oracleRequisition `json:"requisitionList"`
	} `json:"items"`
}

func (s oracle) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	b, err := parseOracleBoard(e.Board)
	if err != nil {
		return nil, err
	}

	reqs, err := s.listRequisitions(ctx, b)
	if err != nil {
		return nil, err
	}

	return fetchDetails(reqs, defaultDetailWorkers, func(r oracleRequisition) (Job, bool) {
		return s.detail(ctx, e, b, r)
	}), nil
}

// listRequisitions pages through the board's requisitions, stopping when a page is empty
// or the collected count reaches TotalJobsCount. The expand keeps requisitionList populated
// (without it the API returns an empty list under a non-zero total).
func (s oracle) listRequisitions(ctx context.Context, b oracleBoard) ([]oracleRequisition, error) {
	var reqs []oracleRequisition
	for offset := 0; ; {
		// offset must sit INSIDE the finder clause (next to limit). Oracle ignores a
		// top-level &offset= query param, so a misplaced offset silently re-fetches the
		// first page every iteration and only the newest page of jobs is ever collected.
		url := fmt.Sprintf(
			"https://%s/hcmRestApi/resources/latest/recruitingCEJobRequisitions"+
				"?onlyData=true&expand=requisitionList.secondaryLocations,requisitionList.workLocation"+
				"&finder=findReqs;siteNumber=%s,limit=%d,offset=%d,sortBy=POSTING_DATES_DESC",
			b.host, b.site, oraclePageLimit, offset)
		var resp oracleListResponse
		if err := s.http.GetJSON(ctx, url, &resp); err != nil {
			return nil, fmt.Errorf("oracle: list site %s: %w", b.site, err)
		}
		if len(resp.Items) == 0 {
			break
		}
		page := resp.Items[0]
		if len(page.RequisitionList) == 0 {
			break
		}
		reqs = append(reqs, page.RequisitionList...)
		offset += len(page.RequisitionList)
		if offset >= page.TotalJobsCount {
			break
		}
	}
	return reqs, nil
}

// detail fetches one requisition's detail and maps it to a Job, returning ok=false when
// the detail request fails so the caller can skip just that requisition.
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
			JobSchedule                 string `json:"JobSchedule"`
		} `json:"items"`
	}
	if err := s.http.GetJSON(ctx, url, &resp); err != nil || len(resp.Items) == 0 {
		return Job{}, false
	}
	d := resp.Items[0]
	// The three external fields are block-level HTML fragments, so they concatenate
	// directly (an empty field contributes nothing) before sanitizing.
	description := sanitizeHTML(
		d.ExternalDescriptionStr + d.ExternalResponsibilitiesStr + d.ExternalQualificationsStr)

	workMode := oracleWorkMode(r.WorkplaceTypeCode)
	return Job{
		ExternalID: r.ID,
		URL: fmt.Sprintf("https://%s/hcmUI/CandidateExperience/en/sites/%s/job/%s",
			b.host, b.site, r.ID),
		Title:       r.Title,
		Company:     e.Company,
		Location:    r.PrimaryLocation,
		Description: description,
		Remote:      workMode == "remote" || isRemote(r.PrimaryLocation),
		WorkMode:    workMode,
		PostedAt:    parseDate(r.PostedDate),
		// JobSchedule is Oracle's structured full/part-time enum; preferred over the
		// free-text employment-type parse.
		EmploymentType: oracleEmploymentType(d.JobSchedule),
	}, true
}

// oracleEmploymentType maps Oracle's JobSchedule ("Full time" / "Part time") onto the
// freehire employment-type vocabulary, returning "" for any other/absent value so the
// description parser decides — structured signal only, never a guess.
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

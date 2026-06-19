package sources

import (
	"context"
	"fmt"
	"strings"

	"golang.org/x/net/html"
)

// adp adapts ADP Workforce Now public career boards (workforcenow.adp.com). A board is
// identified by the tenant's cid + ccId pair (both carried in the recruitment.html URL),
// stored as "cid:ccId". The public staffing API is keyless JSON: a paged requisition list
// (id/title/date/location, no description) plus a per-requisition detail carrying the full
// HTML description — the same list-then-detail shape as aviasales.
type adp struct {
	http JSONGetter
}

// NewADP builds the ADP Workforce Now adapter over the given HTTP client.
func NewADP(c JSONGetter) Source { return adp{http: c} }

func (adp) Provider() string { return "adp" }

const (
	adpBaseURL  = "https://workforcenow.adp.com/mascsr/default/careercenter/public/events/staffing/v1/job-requisitions"
	adpPageSize = 50
	adpMaxPages = 100 // bound the paging so a tenant that ignores $skip can't loop
)

// adpReq is one requisition. The list response omits requisitionDescription (it comes from
// the per-requisition detail); the other fields are present in both.
type adpReq struct {
	ItemID                 string `json:"itemID"`
	RequisitionTitle       string `json:"requisitionTitle"`
	PostDate               string `json:"postDate"`
	RequisitionDescription string `json:"requisitionDescription"`
	RequisitionLocations   []struct {
		NameCode struct {
			ShortName string `json:"shortName"`
		} `json:"nameCode"`
	} `json:"requisitionLocations"`
}

type adpListResp struct {
	JobRequisitions []adpReq `json:"jobRequisitions"`
	Meta            struct {
		TotalNumber   int `json:"totalNumber"`
		StartSequence int `json:"startSequence"`
	} `json:"meta"`
}

func (a adp) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	cid, ccID, ok := strings.Cut(e.Board, ":")
	if !ok || cid == "" || ccID == "" {
		return nil, fmt.Errorf("adp: board %q must be \"cid:ccId\"", e.Board)
	}

	var reqs []adpReq
	for page := 0; page < adpMaxPages; page++ {
		skip := page * adpPageSize
		listURL := fmt.Sprintf("%s?cid=%s&ccId=%s&lang=en_US&locale=en_US&%%24top=%d&%%24skip=%d",
			adpBaseURL, cid, ccID, adpPageSize, skip)
		var resp adpListResp
		if err := a.http.GetJSON(ctx, listURL, &resp); err != nil {
			if page == 0 {
				return nil, fmt.Errorf("adp: list %s: %w", e.Board, err)
			}
			break // a later page failing ends enumeration with what we have
		}
		if len(resp.JobRequisitions) == 0 {
			break
		}
		reqs = append(reqs, resp.JobRequisitions...)
		if skip+len(resp.JobRequisitions) >= resp.Meta.TotalNumber {
			break
		}
	}

	return fetchDetails(reqs, defaultDetailWorkers, func(r adpReq) (Job, bool) {
		return a.detail(ctx, e, cid, ccID, r)
	}), nil
}

// detail fetches one requisition's full record (the list omits the description) and maps it
// to a Job, returning ok=false when the id is empty or the fetch fails.
func (a adp) detail(ctx context.Context, e CompanyEntry, cid, ccID string, r adpReq) (Job, bool) {
	if r.ItemID == "" {
		return Job{}, false
	}
	detailURL := fmt.Sprintf("%s/%s?cid=%s&ccId=%s&lang=en_US&locale=en_US", adpBaseURL, r.ItemID, cid, ccID)
	var d adpReq
	if err := a.http.GetJSON(ctx, detailURL, &d); err != nil {
		return Job{}, false
	}

	location := adpLocation(d)
	return Job{
		ExternalID:  r.ItemID,
		URL:         fmt.Sprintf("https://workforcenow.adp.com/mascsr/default/mdf/recruitment/recruitment.html?cid=%s&ccId=%s&jobId=%s&lang=en_US", cid, ccID, r.ItemID),
		Title:       d.RequisitionTitle,
		Company:     e.Company,
		Location:    location,
		Description: sanitizeHTML(html.UnescapeString(d.RequisitionDescription)),
		Remote:      isRemote(location),
		PostedAt:    parseRFC3339(d.PostDate),
	}, true
}

// adpLocation returns the first location's display name (ADP's shortName, e.g.
// "Remote, EST, US"), trimmed of the leading space ADP prefixes.
func adpLocation(r adpReq) string {
	if len(r.RequisitionLocations) == 0 {
		return ""
	}
	return strings.TrimSpace(r.RequisitionLocations[0].NameCode.ShortName)
}

package main

import (
	"context"
	"fmt"
	"strings"
)

// adpProber probes an ADP Workforce Now board with ONE request: the staffing API's paged
// requisition list, asked for a single row, whose meta.totalNumber is the board's open count.
//
// Without it a candidate is probed by running the adp ADAPTER, which is a whole crawl per
// board — every listing page plus one detail request per posting. Against a 6,050-candidate
// harvest that is roughly a quarter of a million requests, hours past the unit's timeout, and
// harvest-boards persists only at the end: the run would be killed having written nothing. The
// cheap probe answers the only question a harvest asks — does this board carry jobs — and
// leaves the crawl to the crawler.
//
// It publishes no employer name. The list response carries requisitions and a count and no
// account record, so the name gate stands down and liveness is the whole of the evidence —
// which is the same position every board harvested from an id-only seed is in.
type adpProber struct{}

// adpProbePageSize is the page the probe asks for, and it is 50 rather than the 1 a liveness
// check wants because ADP does not serve a page of 1. Measured against a board carrying 329 open
// postings: "$top=1" answers 200 with an empty array and no meta at all — indistinguishable from
// a dead board — while "$top=50" answers with rows and meta.totalNumber=329. It is not even a
// page size the platform honours (that request returned 19 rows), which is fine: what the probe
// reads is the count beside them.
//
// It matches the adapter's own page size deliberately. The one size we know ADP serves is the
// one the crawl already uses, and a probe that disagreed with the crawl about what the platform
// accepts would report boards dead that the crawl reads fine.
const adpProbePageSize = 50

func adpProbeURL(cid, ccID string) string {
	return fmt.Sprintf(
		"https://workforcenow.adp.com/mascsr/default/careercenter/public/events/staffing/v1/job-requisitions"+
			"?cid=%s&ccId=%s&lang=en_US&locale=en_US&%%24top=%d&%%24skip=0", cid, ccID, adpProbePageSize)
}

func (adpProber) probe(ctx context.Context, c httpClient, board string) (string, int, error) {
	cid, ccID, ok := strings.Cut(board, ":")
	if !ok || cid == "" || ccID == "" {
		// Not a board this platform can be asked about. Declining is not an error: a seed may
		// legitimately propose a shape the platform does not use.
		return "", 0, nil
	}
	var resp struct {
		JobRequisitions []struct {
			ItemID string `json:"itemID"`
		} `json:"jobRequisitions"`
		Meta struct {
			TotalNumber int `json:"totalNumber"`
		} `json:"meta"`
	}
	if err := c.GetJSON(ctx, adpProbeURL(cid, ccID), &resp); err != nil {
		return "", 0, nil
	}
	// totalNumber is the platform's own count and the reason one row is enough. A board that
	// answers with rows but no count still reads as live off what it did return, rather than
	// being discarded over a field the tenant's configuration may omit.
	if n := resp.Meta.TotalNumber; n > 0 {
		return "", n, nil
	}
	return "", len(resp.JobRequisitions), nil
}

// dedupKey folds an ADP board to lower case. The cid half is a uuid the platform serves in
// either case and the catalogue holds lower-case, so without folding a seed spelling it
// differently dedups as a distinct board.
func (adpProber) dedupKey(board string) string { return strings.ToLower(board) }

// adpMyJobsProber probes an ADP MyJobs career site with two requests: its public record, which
// publishes both the employer name and the orgoid the listing is authorised by, and then one
// row of that listing.
//
// The name is worth the first request on its own. MyJobs is the one ADP product that states an
// employer for a board, so unlike adpProber above this one can feed the corroboration gate
// rather than standing it down.
type adpMyJobsProber struct{}

func (adpMyJobsProber) probe(ctx context.Context, c httpClient, slug string) (string, int, error) {
	if slug == "" {
		return "", 0, nil
	}
	var site struct {
		Orgoid     string `json:"orgoid"`
		ClientName string `json:"clientName"`
		Name       string `json:"name"`
	}
	siteURL := fmt.Sprintf("https://myjobs.adp.com/public/staffing/v1/career-site/%s", slug)
	if err := c.GetJSONWithHeaders(ctx, siteURL, nil, &site); err != nil {
		return "", 0, nil
	}
	if site.Orgoid == "" {
		// No orgoid means the listing cannot be authorised, so the board cannot be crawled even
		// if postings exist behind it. Reporting it dead is the honest answer.
		return "", 0, nil
	}
	var resp struct {
		Count           int `json:"count"`
		JobRequisitions []struct {
			ReqID string `json:"reqId"`
		} `json:"jobRequisitions"`
	}
	listURL := "https://my.adp.com/myadp_prefix/mycareer/public/staffing/v1/job-requisitions?%24top=1&%24skip=0"
	if err := c.GetJSONWithHeaders(ctx, listURL, map[string]string{"orgoid": site.Orgoid}, &resp); err != nil {
		return "", 0, nil
	}
	name := strings.TrimSpace(site.ClientName)
	if name == "" {
		name = strings.TrimSpace(site.Name)
	}
	if resp.Count > 0 {
		return name, resp.Count, nil
	}
	return name, len(resp.JobRequisitions), nil
}

// dedupKey folds a MyJobs slug to lower case: the career-site API resolves either spelling to
// the same site, so an unfolded seed would dedup as a distinct board.
func (adpMyJobsProber) dedupKey(slug string) string { return strings.ToLower(slug) }

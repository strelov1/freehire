package sources

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"

	"golang.org/x/net/html"
)

// adpMyJobs adapts ADP MyJobs career sites (myjobs.adp.com), a different ADP product from the
// Workforce Now one `adp` crawls: MyJobs addresses a board by the career site's slug, not
// cid+ccId, and neither id derives from the other. The staffing API underneath is shared
// ($top/$skip paging, a list without descriptions, a per-requisition detail).
//
// It is its own adapter because the listing is authorised by an `orgoid` REQUEST HEADER, not query
// parameters, and the slug does not contain it, so every crawl first resolves it from the career
// site's public record.
type adpMyJobs struct {
	http HeaderJSONGetter
}

// NewADPMyJobs builds the ADP MyJobs adapter over the given HTTP client.
func NewADPMyJobs(c HeaderJSONGetter) Source { return adpMyJobs{http: c} }

func (adpMyJobs) Provider() string { return "adpmyjobs" }

const (
	// adpMyJobsSiteURL is the career site's own public record, and the only place the orgoid the
	// listing needs is published. Keyless.
	adpMyJobsSiteURL = "https://myjobs.adp.com/public/staffing/v1/career-site/%s"
	// adpMyJobsListURL is served from ADP's my.adp.com host rather than from the career site's,
	// and "myadp_prefix" is a literal path segment there, not a placeholder.
	adpMyJobsListURL  = "https://my.adp.com/myadp_prefix/mycareer/public/staffing/v1/job-requisitions"
	adpMyJobsPageSize = 50
	adpMyJobsMaxPages = 100 // bound the paging so a site that ignores $skip can't loop
)

// adpMyJobsSite is the career site's public record. The orgoid authorises the listing; the
// client name is the employer the platform publishes for the board.
type adpMyJobsSite struct {
	Orgoid     string `json:"orgoid"`
	ClientName string `json:"clientName"`
	Name       string `json:"name"`
}

// adpMyJobsReq is one requisition. The list omits jobDescription (it comes from the
// per-requisition detail); the rest are present in both.
type adpMyJobsReq struct {
	ReqID          string `json:"reqId"`
	JobTitle       string `json:"jobTitle"`
	JobDescription string `json:"jobDescription"`
	PostingDate    string `json:"postingDate"`
	Locations      []struct {
		Address struct {
			CityName string `json:"cityName"`
			Country  struct {
				LongName string `json:"longName"`
			} `json:"country"`
			CountrySubdivisionLevel1 struct {
				LongName string `json:"longName"`
			} `json:"countrySubdivisionLevel1"`
		} `json:"address"`
	} `json:"requisitionLocations"`
}

type adpMyJobsResp struct {
	Count           int            `json:"count"`
	JobRequisitions []adpMyJobsReq `json:"jobRequisitions"`
}

func (a adpMyJobs) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return a.FetchNew(ctx, e, func(string) bool { return false })
}

func (a adpMyJobs) FetchNew(ctx context.Context, e CompanyEntry, seen func(string) bool) ([]Job, error) {
	board := strings.TrimSpace(e.Board)
	if board == "" {
		return nil, fmt.Errorf("adpmyjobs: board is empty")
	}

	var site adpMyJobsSite
	if err := a.http.GetJSONWithHeaders(ctx, fmt.Sprintf(adpMyJobsSiteURL, board), nil, &site); err != nil {
		return nil, fmt.Errorf("adpmyjobs: career site %s: %w", board, err)
	}
	if site.Orgoid == "" {
		// The site answered without the orgoid. Fail here: an unauthorised listing returns 400,
		// which would read as a board that has gone away.
		return nil, fmt.Errorf("adpmyjobs: career site %s published no orgoid", board)
	}
	headers := map[string]string{"orgoid": site.Orgoid}

	var reqs []adpMyJobsReq
	for page := 0; page < adpMyJobsMaxPages; page++ {
		skip := page * adpMyJobsPageSize
		listURL := fmt.Sprintf("%s?%%24top=%d&%%24skip=%d", adpMyJobsListURL, adpMyJobsPageSize, skip)
		var resp adpMyJobsResp
		if err := a.http.GetJSONWithHeaders(ctx, listURL, headers, &resp); err != nil {
			if page == 0 {
				return nil, fmt.Errorf("adpmyjobs: list %s: %w", board, err)
			}
			break // a later page failing ends enumeration with what we have
		}
		if len(resp.JobRequisitions) == 0 {
			break
		}
		reqs = append(reqs, resp.JobRequisitions...)
		if skip+len(resp.JobRequisitions) >= resp.Count {
			break
		}
	}

	company := e.Company
	if company == "" {
		company = adpMyJobsCompany(site)
	}
	var attempted, readable atomic.Int64
	jobs := fetchDetails(reqs, defaultDetailWorkers, func(r adpMyJobsReq) (Job, bool) {
		if r.ReqID != "" && seen(r.ReqID) {
			return Job{ExternalID: r.ReqID, Title: r.JobTitle, URL: fmt.Sprintf("https://myjobs.adp.com/%s/cx/job/%s", board, r.ReqID), Company: company, SeenRefresh: true}, true
		}
		attempted.Add(1)
		job, ok := a.detail(ctx, board, company, headers, r)
		if ok {
			readable.Add(1)
		}
		return job, ok
	})
	if attempted.Load() > 0 && readable.Load() == 0 {
		return nil, fmt.Errorf("adpmyjobs: none of %d new postings could be read", attempted.Load())
	}
	return jobs, nil
}

// detail fetches one requisition's full record (the list omits the description) and maps it to
// a Job, returning ok=false when the id is empty or the fetch fails.
func (a adpMyJobs) detail(ctx context.Context, board, company string, headers map[string]string, r adpMyJobsReq) (Job, bool) {
	if r.ReqID == "" {
		return Job{}, false
	}
	var resp adpMyJobsResp
	if err := a.http.GetJSONWithHeaders(ctx, adpMyJobsListURL+"/"+r.ReqID, headers, &resp); err != nil {
		return Job{}, false
	}
	// The detail answers in the same envelope as the list, one requisition long.
	if len(resp.JobRequisitions) == 0 {
		return Job{}, false
	}
	d := resp.JobRequisitions[0]

	location := adpMyJobsLocation(d)
	return Job{
		ExternalID:  r.ReqID,
		URL:         fmt.Sprintf("https://myjobs.adp.com/%s/cx/job/%s", board, r.ReqID),
		Title:       d.JobTitle,
		Company:     company,
		Location:    location,
		Description: sanitizeHTML(html.UnescapeString(d.JobDescription)),
		Remote:      isRemote(location),
		PostedAt:    parseRFC3339(d.PostingDate),
	}, true
}

// adpMyJobsCompany is the employer name the career site publishes, used only when the catalog row
// carries none. clientName is the employer ("Stellantis"); the site name reads as a board
// ("Stellantis External CX"), so it is the fallback.
func adpMyJobsCompany(s adpMyJobsSite) string {
	if n := strings.TrimSpace(s.ClientName); n != "" {
		return n
	}
	return strings.TrimSpace(s.Name)
}

// adpMyJobsLocation renders the first location as "City, Region, Country", dropping empty parts;
// MyJobs gives the address only in pieces.
//
// It reads requisitionLocations only on purpose: postingLocations and workLocations were empty
// wherever sampled, and some boards publish no location in any array, so an empty result is the
// employer saying nothing.
func adpMyJobsLocation(r adpMyJobsReq) string {
	if len(r.Locations) == 0 {
		return ""
	}
	a := r.Locations[0].Address
	parts := make([]string, 0, 3)
	for _, p := range []string{a.CityName, a.CountrySubdivisionLevel1.LongName, a.Country.LongName} {
		if p = strings.TrimSpace(p); p != "" {
			parts = append(parts, p)
		}
	}
	return strings.Join(parts, ", ")
}

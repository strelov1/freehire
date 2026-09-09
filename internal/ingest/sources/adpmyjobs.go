package sources

import (
	"context"
	"fmt"
	"strings"

	"golang.org/x/net/html"
)

// adpMyJobs adapts ADP MyJobs career sites (myjobs.adp.com). It is a SECOND ADP product, not a
// second front door onto the one `adp` already crawls: Workforce Now addresses a board by the
// cid+ccId pair carried in its recruitment URL, MyJobs by the career site's own slug, and
// neither id is derivable from the other. A company on MyJobs is therefore unreachable through
// the adp adapter however its board is written.
//
// What the two share is the staffing API underneath — the same $top/$skip paging, the same
// requisition list that omits the description and per-requisition detail that carries it — so
// this reads as a near-sibling of adp.go by construction rather than by copying.
//
// Two things make it its own adapter rather than a mode of that one. The board is a slug, and
// the listing is authorised by an `orgoid` REQUEST HEADER rather than by query parameters — an
// id the slug does not contain, so every crawl resolves it first from the career site's own
// public record.
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
	board := strings.TrimSpace(e.Board)
	if board == "" {
		return nil, fmt.Errorf("adpmyjobs: board is empty")
	}

	var site adpMyJobsSite
	if err := a.http.GetJSONWithHeaders(ctx, fmt.Sprintf(adpMyJobsSiteURL, board), nil, &site); err != nil {
		return nil, fmt.Errorf("adpmyjobs: career site %s: %w", board, err)
	}
	if site.Orgoid == "" {
		// The site answered without the id the listing is authorised by. Failing here rather
		// than fetching is the point: an unauthorised listing returns 400, which would read as a
		// board that has gone away rather than one we asked for wrongly.
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
	return fetchDetails(reqs, defaultDetailWorkers, func(r adpMyJobsReq) (Job, bool) {
		return a.detail(ctx, board, company, headers, r)
	}), nil
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

// adpMyJobsCompany is the employer name the career site publishes, used only when the catalog
// row carries none. clientName is the account's own name ("Stellantis"); the site name is the
// career site's ("Stellantis External CX"), which reads as a board rather than an employer, so
// it is the fallback rather than the first choice.
func adpMyJobsCompany(s adpMyJobsSite) string {
	if n := strings.TrimSpace(s.ClientName); n != "" {
		return n
	}
	return strings.TrimSpace(s.Name)
}

// adpMyJobsLocation renders the first location as "City, Region, Country", dropping the parts
// the platform left empty. Unlike Workforce Now, which publishes a ready display string, MyJobs
// only gives the address in pieces.
//
// It reads requisitionLocations and nothing else on purpose. The payload carries postingLocations
// and workLocations beside it, and both were empty for every posting sampled across three boards
// — while one of those boards (jsmcareers) published no location in ANY of the three arrays. An
// empty location here is therefore an employer saying nothing, not a field read from the wrong
// place, and the pipeline's dictionaries would rather have nothing than a guess.
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

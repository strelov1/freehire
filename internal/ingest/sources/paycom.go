package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

// paycomHTTP is the transport surface the Paycom adapter needs: the portal page (raw text,
// for the embedded session JWT), the authed previews search (POST), and the authed
// company-name + job detail (GET-with-headers).
type paycomHTTP interface {
	TextGetter
	HeaderJSONGetter
	HeaderJSONPoster
}

// paycom adapts Paycom's ATS career portals. A portal is reached at
// paycomonline.net/v4/ats/web.php/portal/<clientkey>/..., a JS app whose SSR shell embeds a
// per-portal session JWT (configsFromHost.sessionJWT) and the regional "Mantle" API host.
// With that JWT as the Authorization header, the Mantle API lists postings
// (POST .../api/ats/job-posting-previews/search, paginated by skip/take) and serves each
// posting's detail (.../api/ats/job-postings/<id>); the employer name comes from
// .../api/ats/company-name. The board is the 32-hex client key; no API key or browser is
// needed — the JWT is read from the page.
type paycom struct {
	http paycomHTTP
}

// NewPaycom builds the Paycom adapter over the given HTTP client.
func NewPaycom(c paycomHTTP) Source { return paycom{http: c} }

func (paycom) Provider() string { return "paycom" }

const (
	paycomPortalBase = "https://www.paycomonline.net/v4/ats/web.php/portal"
	paycomPageSize   = 50
	// paycomMaxPages bounds pagination so a miscounted total can't loop forever. Reaching it
	// before the stated total fails the crawl (fullBoardListing): the largest board measured
	// 2026-10-29 stated 2,705 postings, 55 pages.
	paycomMaxPages = 200
	// paycomNoSuchBoard is the portal's own answer for a client key it no longer serves,
	// delivered as a short page under HTTP 200. Matched on the leading clause alone, without
	// the surrounding markup or the rest of the sentence: the notice read "Job board does not
	// exist." until 2026-10 and "Job board does not exist or is unavailable at this time." since,
	// and the trailing period of the old constant silently stopped matching the new one.
	paycomNoSuchBoard = "Job board does not exist"
)

// fullBoardListing: listJobIDs pages until the distinct ids read reach the total the search
// states (jobPostingPreviewsCount), and fails the crawl on a page error, on a page that comes
// back empty short of that total, and on reaching paycomMaxPages first. A posting whose detail
// the platform answers 404 is dropped (its own "Active job posting not found"); any other
// failed detail is an unreadableDetail marker. See the fullBoardListing interface for the bar.
func (paycom) fullBoardListing() {}

func (s paycom) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	// The SSR portal shell embeds the session JWT + the regional Mantle host; any job-id path
	// segment serves the same shell, so "1" bootstraps without knowing a real posting.
	page, err := s.http.GetText(ctx, fmt.Sprintf("%s/%s/jobs/1", paycomPortalBase, e.Board))
	if err != nil {
		return nil, fmt.Errorf("paycom: bootstrap %s: %w", e.Board, err)
	}
	// A retired client key still answers 200, with a one-line notice in place of the portal
	// shell. Checked before the JWT: without it the crawl fails on the missing token, which
	// reads as a parse bug against a portal that was never served.
	if strings.Contains(page, paycomNoSuchBoard) {
		return nil, fmt.Errorf("paycom: board %q: %w", e.Board, ErrBoardGone)
	}
	jwt := paycomSessionJWT(page)
	mantle := paycomMantleHost(page)
	if jwt == "" || mantle == "" {
		return nil, fmt.Errorf("paycom: board %q: no session token / API host in portal page", e.Board)
	}
	auth := map[string]string{"Authorization": jwt, "Locale": "en-US"}

	company := e.Company
	var cn struct {
		CompanyName string `json:"companyName"`
	}
	if err := s.http.GetJSONWithHeaders(ctx, mantle+"/api/ats/company-name", auth, &cn); err == nil && cn.CompanyName != "" {
		company = cn.CompanyName
	}

	ids, err := s.listJobIDs(ctx, mantle, auth)
	if err != nil {
		return nil, fmt.Errorf("paycom: board %q: %w", e.Board, err)
	}
	return fetchDetails(ids, defaultDetailWorkers, func(id string) (Job, bool) {
		return s.detail(ctx, e.Board, mantle, auth, company, id)
	}), nil
}

// listJobIDs pages the previews search (skip/take) and returns every posting id, first-seen
// order, once the distinct ids read reach the total the search states. Anything short of that
// is an error rather than a partial listing: a failed page, a page that comes back empty before
// the total, or the page cap. Until 2026-10-29 each of those ended enumeration with the ids
// gathered so far, which is why paycom could not earn fullBoardListing.
func (s paycom) listJobIDs(ctx context.Context, mantle string, auth map[string]string) ([]string, error) {
	var ids []string
	seen := make(map[string]bool)
	for page := 0; page < paycomMaxPages; page++ {
		body := paycomSearchBody(page*paycomPageSize, paycomPageSize)
		var resp struct {
			// A pointer, so an answer that omits the total cannot read as a stated total of 0
			// and end the walk on its first page as a "complete" listing.
			Count    *int `json:"jobPostingPreviewsCount"`
			Previews []struct {
				JobID json.Number `json:"jobId"`
			} `json:"jobPostingPreviews"`
		}
		if err := s.http.PostJSONWithHeaders(ctx, mantle+"/api/ats/job-posting-previews/search", auth, body, &resp); err != nil {
			return nil, fmt.Errorf("previews page %d: %w", page, err)
		}
		for _, p := range resp.Previews {
			if id := p.JobID.String(); id != "" && !seen[id] {
				seen[id] = true
				ids = append(ids, id)
			}
		}
		if resp.Count == nil {
			return nil, fmt.Errorf("previews page %d states no jobPostingPreviewsCount", page)
		}
		if len(ids) >= *resp.Count {
			return ids, nil
		}
		if len(resp.Previews) == 0 {
			return nil, fmt.Errorf("previews page %d came back empty with %d of %d postings read", page, len(ids), *resp.Count)
		}
	}
	return nil, fmt.Errorf("previews reached the %d-page cap with %d postings read", paycomMaxPages, len(ids))
}

// detail fetches one posting and maps it to a Job. A posting the platform answers 404/410 for
// is gone and dropped (ok=false); any other failure, or a 200 that carries no posting, comes
// back as an unreadableDetail marker — the detail is this adapter's only source for the posting,
// and a dropped one would read as a posting taken down.
func (s paycom) detail(ctx context.Context, board, mantle string, auth map[string]string, company, id string) (Job, bool) {
	url := fmt.Sprintf("%s/%s/jobs/%s", paycomPortalBase, board, id)
	var resp struct {
		JobPosting struct {
			JobID       json.Number `json:"jobId"`
			JobTitle    string      `json:"jobTitle"`
			Location    string      `json:"location"`
			RemoteType  string      `json:"remoteType"`
			Description string      `json:"description"`
			StartDate   string      `json:"startDate"`
		} `json:"jobPosting"`
	}
	if err := s.http.GetJSONWithHeaders(ctx, fmt.Sprintf("%s/api/ats/job-postings/%s", mantle, id), auth, &resp); err != nil {
		if detailUnreadable(err) {
			return unreadableDetail(id, url, company), true
		}
		return Job{}, false
	}
	p := resp.JobPosting
	if p.JobID.String() != id {
		return unreadableDetail(id, url, company), true
	}
	return Job{
		ExternalID:  id,
		URL:         url,
		Title:       p.JobTitle,
		Company:     company,
		Location:    p.Location,
		Description: sanitizeHTML(html.UnescapeString(p.Description)),
		Remote:      strings.EqualFold(p.RemoteType, "remote") || isRemote(p.Location),
		WorkMode:    workModeFromRemote(strings.EqualFold(p.RemoteType, "remote")),
		PostedAt:    parseRFC3339(p.StartDate),
	}, true
}

// paycomSessionJWTPattern captures the per-portal session JWT the SSR page embeds in
// configsFromHost; it is the Authorization bearer the Mantle API expects (not a secret key).
var paycomSessionJWTPattern = regexp.MustCompile(`"sessionJWT":"([^"]+)"`)

func paycomSessionJWT(page string) string {
	return firstSubmatch(paycomSessionJWTPattern, page)
}

// paycomMantleHostPattern captures the regional Mantle API host from the portal page; the
// region (us-cent, us-east, …) varies per tenant, so it is read rather than hard-coded.
var paycomMantleHostPattern = regexp.MustCompile(`portal-applicant-tracking\.[a-z0-9-]+\.paycomonline\.net`)

func paycomMantleHost(page string) string {
	if m := paycomMantleHostPattern.FindString(page); m != "" {
		return "https://" + m
	}
	return ""
}

// paycomSearchBody builds the previews-search request. The filters nest under
// filtersForQuery (a top-level filter object silently returns zero results).
func paycomSearchBody(skip, take int) map[string]any {
	return map[string]any{
		"skip": skip,
		"take": take,
		"filtersForQuery": map[string]any{
			"distanceFrom":      0,
			"workEnvironments":  []any{},
			"positionTypes":     []any{},
			"educationLevels":   []any{},
			"categories":        []any{},
			"travelTypes":       []any{},
			"shiftTypes":        []any{},
			"otherFilters":      []any{},
			"keywordSearchText": "",
			"location":          "",
			"sortOption":        "",
		},
	}
}

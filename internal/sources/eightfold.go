package sources

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// eightfoldPageLimit is the position-list page size. The Eightfold server caps a page at 10
// regardless of the requested num, so this is fixed and start is the only pagination lever.
const eightfoldPageLimit = 10

// eightfoldDetailWorkers bounds the per-board detail fan-out. Eightfold rate-limits an IP to
// ~290 requests per window (returning 403), so the detail crawl uses a low concurrency — far
// below the shared defaultDetailWorkers — to keep the burst gentle and lean on backoff retry
// (getJSONRetrying) for the rest, rather than slamming the cap and dropping postings.
const eightfoldDetailWorkers = 2

// eightfoldMaxRetries bounds how many times a rate-limited (403/429) request is retried before
// giving up. eightfoldRetryBase is the first backoff delay (doubled each attempt, capped); it
// is a var so tests can set it to 0 and not sleep.
const eightfoldMaxRetries = 6

var eightfoldRetryBase = time.Second

// eightfold adapts Eightfold AI career sites (e.g. apply.careers.microsoft.com). Both
// endpoints are public GET JSON. Two list-API generations exist and a tenant supports exactly
// one (the other returns 403): the newer /api/pcsx/search and the legacy /api/apply/v2/jobs.
// The list carries no description, so each position's detail /api/apply/v2/jobs/<id> (shared by
// both generations) is fetched to assemble the description and canonical URL.
type eightfold struct {
	http JSONGetter
}

// NewEightfold builds the Eightfold adapter over the given HTTP client.
func NewEightfold(c JSONGetter) Source { return eightfold{http: c} }

func (eightfold) Provider() string { return "eightfold" }

// eightfoldBoard is a configured board split into the host and tenant domain the endpoints need.
type eightfoldBoard struct {
	host, domain string
}

// parseEightfoldBoard splits "host/domain" (e.g.
// "apply.careers.microsoft.com/microsoft.com"). The host paths the requests; the domain is
// the Eightfold tenant key the endpoints require as a query parameter.
func parseEightfoldBoard(board string) (eightfoldBoard, error) {
	host, domain, ok := strings.Cut(board, "/")
	if !ok || host == "" || domain == "" {
		return eightfoldBoard{}, fmt.Errorf("eightfold: board %q must be \"host/domain\"", board)
	}
	return eightfoldBoard{host: host, domain: domain}, nil
}

// eightfoldPosition is one list item, decoding both generations' field names: the newer pcsx
// list uses postedTs/workLocationOption and only locations[]; the legacy v2 list uses
// t_create/work_location_option and a single-string location (plus a canonical URL). Unused
// fields stay zero, so one struct decodes either shape.
type eightfoldPosition struct {
	ID                   int64    `json:"id"`
	Name                 string   `json:"name"`
	Location             string   `json:"location"`  // legacy v2 single-string
	Locations            []string `json:"locations"` // both generations
	PostedTs             int64    `json:"postedTs"`  // pcsx
	TCreate              int64    `json:"t_create"`  // legacy v2
	WorkLocationOption   string   `json:"workLocationOption"`
	WorkLocationOptionV2 string   `json:"work_location_option"`
	CanonicalPositionURL string   `json:"canonicalPositionUrl"` // present in the legacy v2 list
}

// pcsxListResponse is one /api/pcsx/search page (positions nest under data).
type pcsxListResponse struct {
	Data struct {
		Positions []eightfoldPosition `json:"positions"`
		Count     int                 `json:"count"`
	} `json:"data"`
}

// v2ListResponse is one legacy /api/apply/v2/jobs page (positions and count are top-level).
type v2ListResponse struct {
	Positions []eightfoldPosition `json:"positions"`
	Count     int                 `json:"count"`
}

// eightfoldDetail is the part of /api/apply/v2/jobs/<id> the Job shape needs.
type eightfoldDetail struct {
	JobDescription       string `json:"job_description"`
	CanonicalPositionURL string `json:"canonicalPositionUrl"`
}

func (s eightfold) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	b, err := parseEightfoldBoard(e.Board)
	if err != nil {
		return nil, err
	}

	positions, err := s.listPositions(ctx, b)
	if err != nil {
		return nil, err
	}

	return fetchDetails(positions, eightfoldDetailWorkers, func(p eightfoldPosition) (Job, bool) {
		return s.detail(ctx, e, b, p)
	}), nil
}

// getJSONRetrying GETs url, retrying rate-limit responses (403/429) with exponential backoff
// so the crawl rides out Eightfold's per-IP request cap (~290/window) instead of dropping the
// postings beyond it. The shared client already retries 429/5xx/network; this adds 403, which
// Eightfold returns for rate-limiting (not auth) here, and longer waits to clear a full window.
// A non-rate-limit error (e.g. 404) returns immediately so retry stays scoped.
func (s eightfold) getJSONRetrying(ctx context.Context, url string, v any) error {
	delay := eightfoldRetryBase
	var err error
	for attempt := 0; attempt <= eightfoldMaxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}
			if delay < 30*time.Second {
				delay *= 2
			}
		}
		if err = s.http.GetJSON(ctx, url, v); err == nil || !isRateLimited(err) {
			return err
		}
	}
	return err
}

// isRateLimited reports whether err is an Eightfold rate-limit response (HTTP 403 or 429). The
// shared client formats a status failure as "... status <code>", so the code is matched in the
// error text — the client does not surface the status any other way.
func isRateLimited(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "status 403") || strings.Contains(msg, "status 429")
}

// listPositions returns the board's positions, auto-detecting the list-API generation: it
// tries the newer pcsx list and, if that fails (a legacy tenant returns 403), falls back to
// the v2 list. A tenant supports exactly one, so whichever paginates successfully is
// authoritative; the fallback restarts from the first page.
func (s eightfold) listPositions(ctx context.Context, b eightfoldBoard) ([]eightfoldPosition, error) {
	positions, err := s.pageList(ctx, b, s.pcsxPage)
	if err != nil {
		positions, err = s.pageList(ctx, b, s.v2Page)
	}
	if err != nil {
		return nil, fmt.Errorf("eightfold: list domain %s: %w", b.domain, err)
	}
	return positions, nil
}

// pageList walks one list generation, fetching pages via fetchPage and advancing start by the
// page size the server returns, stopping when a page is empty or the collected count reaches
// the catalogue total. The empty-page check is the backstop if count is ever wrong.
func (s eightfold) pageList(
	ctx context.Context, b eightfoldBoard,
	fetchPage func(ctx context.Context, b eightfoldBoard, start int) ([]eightfoldPosition, int, error),
) ([]eightfoldPosition, error) {
	var positions []eightfoldPosition
	for start := 0; ; {
		page, count, err := fetchPage(ctx, b, start)
		if err != nil {
			return nil, err
		}
		if len(page) == 0 {
			break
		}
		positions = append(positions, page...)
		start += len(page)
		if start >= count {
			break
		}
	}
	return positions, nil
}

// pcsxPage fetches one newer /api/pcsx/search page.
func (s eightfold) pcsxPage(ctx context.Context, b eightfoldBoard, start int) ([]eightfoldPosition, int, error) {
	url := fmt.Sprintf(
		"https://%s/api/pcsx/search?domain=%s&query=&start=%d&num=%d&sort_by=relevance",
		b.host, b.domain, start, eightfoldPageLimit)
	var resp pcsxListResponse
	if err := s.getJSONRetrying(ctx, url, &resp); err != nil {
		return nil, 0, err
	}
	return resp.Data.Positions, resp.Data.Count, nil
}

// v2Page fetches one legacy /api/apply/v2/jobs page.
func (s eightfold) v2Page(ctx context.Context, b eightfoldBoard, start int) ([]eightfoldPosition, int, error) {
	url := fmt.Sprintf(
		"https://%s/api/apply/v2/jobs?domain=%s&query=&start=%d&num=%d&sort_by=relevance",
		b.host, b.domain, start, eightfoldPageLimit)
	var resp v2ListResponse
	if err := s.getJSONRetrying(ctx, url, &resp); err != nil {
		return nil, 0, err
	}
	return resp.Positions, resp.Count, nil
}

// detail fetches one position's detail and maps it to a Job, returning ok=false when the
// detail request fails so the caller can skip just that position. Metadata comes from the
// list position; the description and canonical URL come from the detail (with the list
// position's canonical URL and a host fallback for the URL).
func (s eightfold) detail(ctx context.Context, e CompanyEntry, b eightfoldBoard, p eightfoldPosition) (Job, bool) {
	id := strconv.FormatInt(p.ID, 10)
	url := fmt.Sprintf("https://%s/api/apply/v2/jobs/%s?domain=%s", b.host, id, b.domain)
	var d eightfoldDetail
	if err := s.getJSONRetrying(ctx, url, &d); err != nil {
		return Job{}, false
	}

	location := firstNonEmpty(p.Location, firstNonEmpty(p.Locations...))
	workMode := workplaceTypeMode(firstNonEmpty(p.WorkLocationOption, p.WorkLocationOptionV2))
	postedTs := p.PostedTs
	if postedTs == 0 {
		postedTs = p.TCreate
	}
	pageURL := firstNonEmpty(
		d.CanonicalPositionURL, p.CanonicalPositionURL,
		fmt.Sprintf("https://%s/careers/job/%s", b.host, id))
	return Job{
		ExternalID:  id,
		URL:         pageURL,
		Title:       strings.TrimSpace(p.Name),
		Company:     e.Company,
		Location:    location,
		Description: sanitizeHTML(d.JobDescription),
		Remote:      workMode == "remote" || isRemote(location),
		WorkMode:    workMode,
		PostedAt:    parseEpochSeconds(postedTs),
	}, true
}

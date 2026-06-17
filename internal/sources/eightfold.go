package sources

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// eightfoldPageLimit is the position-list page size. The Eightfold server caps a page at 10
// regardless of the requested num, so this is fixed and start is the only pagination lever.
const eightfoldPageLimit = 10

// eightfold adapts Eightfold AI career sites (e.g. apply.careers.microsoft.com). Both
// endpoints are public GET JSON: the position list /api/pcsx/search (which requires the tenant
// domain and carries no description) and each position's detail /api/apply/v2/jobs/<id>, fetched
// to assemble the description and canonical URL.
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

// eightfoldPosition is one item from the /api/pcsx/search list (no description here).
type eightfoldPosition struct {
	ID                 int64    `json:"id"`
	Name               string   `json:"name"`
	Locations          []string `json:"locations"`
	PostedTs           int64    `json:"postedTs"`
	WorkLocationOption string   `json:"workLocationOption"`
}

// eightfoldListResponse wraps a search page: the positions plus the catalogue total used as
// the stop condition.
type eightfoldListResponse struct {
	Data struct {
		Positions []eightfoldPosition `json:"positions"`
		Count     int                 `json:"count"`
	} `json:"data"`
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

	return fetchDetails(positions, defaultDetailWorkers, func(p eightfoldPosition) (Job, bool) {
		return s.detail(ctx, e, b, p)
	}), nil
}

// listPositions pages through the board's positions, advancing start by the page size the
// server returns and stopping when a page is empty or the collected count reaches the
// catalogue total. The empty-page check is the backstop if count is ever wrong.
func (s eightfold) listPositions(ctx context.Context, b eightfoldBoard) ([]eightfoldPosition, error) {
	var positions []eightfoldPosition
	for start := 0; ; {
		url := fmt.Sprintf(
			"https://%s/api/pcsx/search?domain=%s&query=&start=%d&num=%d&sort_by=relevance",
			b.host, b.domain, start, eightfoldPageLimit)
		var resp eightfoldListResponse
		if err := s.http.GetJSON(ctx, url, &resp); err != nil {
			return nil, fmt.Errorf("eightfold: list domain %s: %w", b.domain, err)
		}
		if len(resp.Data.Positions) == 0 {
			break
		}
		positions = append(positions, resp.Data.Positions...)
		start += len(resp.Data.Positions)
		if start >= resp.Data.Count {
			break
		}
	}
	return positions, nil
}

// detail fetches one position's detail and maps it to a Job, returning ok=false when the
// detail request fails so the caller can skip just that position. Metadata comes from the
// list position; only the description and canonical URL come from the detail.
func (s eightfold) detail(ctx context.Context, e CompanyEntry, b eightfoldBoard, p eightfoldPosition) (Job, bool) {
	id := strconv.FormatInt(p.ID, 10)
	url := fmt.Sprintf("https://%s/api/apply/v2/jobs/%s?domain=%s", b.host, id, b.domain)
	var d eightfoldDetail
	if err := s.http.GetJSON(ctx, url, &d); err != nil {
		return Job{}, false
	}

	location := firstNonEmpty(p.Locations...)
	workMode := workplaceTypeMode(p.WorkLocationOption)
	return Job{
		ExternalID:  id,
		URL:         firstNonEmpty(d.CanonicalPositionURL, fmt.Sprintf("https://%s/careers/job/%s", b.host, id)),
		Title:       strings.TrimSpace(p.Name),
		Company:     e.Company,
		Location:    location,
		Description: sanitizeHTML(d.JobDescription),
		Remote:      workMode == "remote" || isRemote(location),
		WorkMode:    workMode,
		PostedAt:    parseEpochSeconds(p.PostedTs),
	}, true
}

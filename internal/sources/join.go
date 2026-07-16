package sources

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/yuin/goldmark"
)

// joinPageSize is the list-API page size. Join's public API caps pageSize at 5; any larger
// value is rejected with HTTP 422. Pagination is driven by pagination.pageCount, so a smaller
// page just means more list pages — the per-job detail fetches (the bulk of the work) are unaffected.
const (
	joinPageSize = 5
)

// join adapts Join.com career pages over its public JSON API. The board is the numeric Join
// company id. The list endpoint carries no description, so each posting's body comes from its
// own detail request (bounded-concurrency), like the SmartRecruiters and Gem adapters.
type join struct {
	http JSONGetter
}

// NewJoin builds the Join.com adapter over the given HTTP client.
func NewJoin(c JSONGetter) Source { return join{http: c} }

func (join) Provider() string { return "join" }

// joinJob is one item of the list response. The description is absent here (detail only).
type joinJob struct {
	ID            int64  `json:"id"`
	IDParam       string `json:"idParam"`
	Title         string `json:"title"`
	CreatedAt     string `json:"createdAt"`
	WorkplaceType string `json:"workplaceType"`
	City          *struct {
		CityName    string `json:"cityName"`
		CountryName string `json:"countryName"`
	} `json:"city"`
}

// joinListResp is the public list endpoint's envelope. pageCount drives pagination.
type joinListResp struct {
	Items      []joinJob `json:"items"`
	Pagination struct {
		PageCount int `json:"pageCount"`
	} `json:"pagination"`
}

// joinDetail is the per-job endpoint: the Markdown body and the company slug (for the URL).
type joinDetail struct {
	Description string `json:"description"`
	Company     struct {
		Domain string `json:"domain"`
	} `json:"company"`
}

func (j join) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	var items []joinJob
	for page := 1; ; page++ {
		var resp joinListResp
		url := fmt.Sprintf("https://join.com/api/public/companies/%s/jobs?page=%d&pageSize=%d",
			e.Board, page, joinPageSize)
		if err := j.http.GetJSON(ctx, url, &resp); err != nil {
			return nil, fmt.Errorf("join: list company %s page %d: %w", e.Board, page, err)
		}
		items = append(items, resp.Items...)
		if page >= resp.Pagination.PageCount {
			break // pageCount is authoritative; 0 (empty board) stops after the first page
		}
	}

	// Each job's description comes from its own detail request, fanned out under a bounded pool.
	return fetchDetails(items, defaultDetailWorkers, func(it joinJob) (Job, bool) {
		return j.detail(ctx, e, it)
	}), nil
}

// detail fetches one job's body and maps it to a Job, returning ok=false when the detail
// request fails so the caller can skip just that posting.
func (j join) detail(ctx context.Context, e CompanyEntry, it joinJob) (Job, bool) {
	var d joinDetail
	url := fmt.Sprintf("https://join.com/api/public/jobs/%d", it.ID)
	if err := j.http.GetJSON(ctx, url, &d); err != nil {
		return Job{}, false
	}

	var city, country string
	if it.City != nil {
		city, country = it.City.CityName, it.City.CountryName
	}
	location := joinNonEmpty(city, country)

	return Job{
		ExternalID:  strconv.FormatInt(it.ID, 10),
		URL:         fmt.Sprintf("https://join.com/companies/%s/%s", d.Company.Domain, it.IDParam),
		Title:       it.Title,
		Company:     e.Company,
		Location:    location,
		Description: sanitizeHTML(markdownToHTML(d.Description)),
		// workplaceType is the authoritative remote signal; isRemote(location) is only a
		// fallback (never the title, which false-positives on "Remote …" role names).
		Remote:   it.WorkplaceType == "REMOTE" || isRemote(location),
		PostedAt: parseRFC3339(it.CreatedAt),
	}, true
}

// markdownToHTML renders a Markdown body to HTML. Join.com job descriptions are Markdown;
// the rendered HTML is then sanitized so the stored description matches the "descriptions
// are sanitized HTML" convention. A render error (or empty input) yields "".
func markdownToHTML(md string) string {
	if strings.TrimSpace(md) == "" {
		return ""
	}
	var b strings.Builder
	if err := goldmark.Convert([]byte(md), &b); err != nil {
		return ""
	}
	return b.String()
}

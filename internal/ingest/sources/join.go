package sources

import (
	"context"
	"errors"
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

// Listing pages are validated through the declared end before detail reads begin.
func (join) fullBoardListing()  {}
func (join) emptyBoardListing() {}

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
		PageCount *int `json:"pageCount"`
		RowCount  *int `json:"rowCount"`
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
	total, pages := -1, -1
	seen := make(map[int64]bool)
	for page := 1; ; page++ {
		var resp joinListResp
		url := fmt.Sprintf("https://join.com/api/public/companies/%s/jobs?page=%d&pageSize=%d",
			e.Board, page, joinPageSize)
		if err := j.http.GetJSON(ctx, url, &resp); err != nil {
			return nil, fmt.Errorf("join: list company %s page %d: %w", e.Board, page, err)
		}
		if resp.Items == nil || resp.Pagination.PageCount == nil || resp.Pagination.RowCount == nil {
			return nil, errListingAbsent("join", e.Board, "items/pagination")
		}
		count, rows := *resp.Pagination.PageCount, *resp.Pagination.RowCount
		if page == 1 {
			total, pages = rows, count
		}
		if rows < 0 || count < 0 || rows != total || count != pages || (rows == 0 && (count != 0 || len(resp.Items) != 0)) || (rows > 0 && (count == 0 || len(resp.Items) == 0)) {
			return nil, fmt.Errorf("join: board %s: inconsistent pagination on page %d", e.Board, page)
		}
		for _, item := range resp.Items {
			if item.ID <= 0 || strings.TrimSpace(item.Title) == "" || item.IDParam == "" || seen[item.ID] {
				return nil, fmt.Errorf("join: board %s: invalid or repeated posting on page %d", e.Board, page)
			}
			seen[item.ID] = true
		}
		items = append(items, resp.Items...)
		if len(items) > total {
			return nil, fmt.Errorf("join: board %s: listing exceeds row count", e.Board)
		}
		if page >= count {
			break // pageCount is authoritative; 0 (empty board) stops after the first page
		}
	}

	if len(items) != total {
		return nil, fmt.Errorf("join: board %s: incomplete listing", e.Board)
	}

	// Each job's description comes from its own detail request, fanned out under a bounded pool.
	return fetchDetails(items, defaultDetailWorkers, func(it joinJob) (Job, bool) {
		return j.detail(ctx, e, it)
	}), nil
}

// detail fetches one job's body and maps it to a Job, returning ok=false when the detail
// request proves it gone. Other failures preserve the listed identity as unreadable.
func (j join) detail(ctx context.Context, e CompanyEntry, it joinJob) (Job, bool) {
	var d joinDetail
	url := fmt.Sprintf("https://join.com/api/public/jobs/%d", it.ID)
	if err := j.http.GetJSON(ctx, url, &d); err != nil {
		var status *StatusError
		if errors.As(err, &status) && (status.Code == 404 || status.Code == 410) {
			return Job{}, false
		}
		return unreadableDetail(strconv.FormatInt(it.ID, 10), url, e.Company), true
	}
	if strings.TrimSpace(d.Description) == "" || d.Company.Domain == "" {
		return unreadableDetail(strconv.FormatInt(it.ID, 10), url, e.Company), true
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

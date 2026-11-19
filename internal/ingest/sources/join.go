package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/yuin/goldmark"
	"golang.org/x/time/rate"
)

// joinPageSize retains the public careers UI page size. pageInfo drives pagination.
const (
	joinPageSize = 5
)

// join adapts Join.com career pages over its public JSON API. The board is the numeric Join
// company id. The list endpoint carries no description, so each posting's body comes from its
// own detail request (bounded-concurrency), like the SmartRecruiters and Gem adapters.
type join struct {
	http JSONPoster
}

// NewJoin builds the Join.com adapter over the given HTTP client.
func NewJoin(c JSONPoster) Source {
	return join{http: rateLimitedJSONPoster{inner: c, limiter: rate.NewLimiter(rate.Every(joinRequestInterval), joinRequestBurst)}}
}

func (join) Provider() string { return "join" }

// Listing pages are validated through the declared end before detail reads begin.
func (join) FullBoardListing() {}

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
	} `json:"pageInfo"`
}

// joinDetail holds the public posting's HTML body and company slug for its URL.
type joinDetail struct {
	Description string `json:"descriptionHtml"`
	Company     struct {
		Domain string `json:"domain"`
	} `json:"company"`
}

func (j join) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	companyID, err := strconv.ParseInt(e.Board, 10, 64)
	if err != nil || companyID <= 0 {
		return nil, fmt.Errorf("join: invalid company id %q", e.Board)
	}
	var items []joinJob
	total, pages := -1, -1
	seen := make(map[int64]bool)
	for page := 1; ; page++ {
		var data struct {
			PublicJobs joinListResp `json:"publicJobs"`
		}
		if err := j.query(ctx, joinListQuery, map[string]any{"companyId": companyID, "paginationPage": map[string]int{"page": page, "pageSize": joinPageSize}}, &data); err != nil {
			return nil, fmt.Errorf("join: list company %s page %d: %w", e.Board, page, err)
		}
		resp := data.PublicJobs
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

// detail fetches one job's HTML body. Failed or missing details preserve the listed
// identity as unreadable; GraphQL null does not establish removal.
func (j join) detail(ctx context.Context, e CompanyEntry, it joinJob) (Job, bool) {
	var data struct {
		PublicJob *joinDetail `json:"publicJob"`
	}
	url := fmt.Sprintf("https://join.com/companies/%s", it.IDParam)
	if err := j.query(ctx, joinDetailQuery, map[string]any{"id": it.ID}, &data); err != nil || data.PublicJob == nil {
		return unreadableDetail(strconv.FormatInt(it.ID, 10), url, e.Company), true
	}
	d := data.PublicJob
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
		Description: sanitizeHTML(d.Description),
		// workplaceType is the authoritative remote signal; isRemote(location) is only a
		// fallback (never the title, which false-positives on "Remote …" role names).
		Remote:   it.WorkplaceType == "REMOTE" || isRemote(location),
		PostedAt: parseRFC3339(it.CreatedAt),
	}, true
}

// markdownToHTML renders a Markdown body to HTML for providers publishing Markdown;
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

const joinGraphQLURL = "https://join.com/candidate-api/graphql"
const joinListQuery = `query PublicJobsList($input: PublicJobsQueryInput!) {
 publicJobs(input: $input) {
 items { id idParam title createdAt workplaceType city { cityName countryName } }
 pageInfo { pageCount rowCount }
 }
}`
const joinDetailQuery = `query PublicJob($input: PublicJobInput!) {
 publicJob(input: $input) { descriptionHtml company { domain } }
}`

func (j join) query(ctx context.Context, query string, input any, result any) error {
	var response struct {
		Data   json.RawMessage `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	err := j.http.PostJSON(ctx, joinGraphQLURL, map[string]any{"query": query, "variables": map[string]any{"input": input}}, &response)
	if err != nil {
		return err
	}
	if len(response.Errors) != 0 {
		return fmt.Errorf("join: GraphQL: %s", response.Errors[0].Message)
	}
	if len(response.Data) == 0 || string(response.Data) == "null" {
		return fmt.Errorf("join: GraphQL response has no data")
	}
	return json.Unmarshal(response.Data, result)
}

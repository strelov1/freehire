package sources

import (
	"context"
	"fmt"
	"strconv"
	"time"
)

// vk adapts VK's public careers API (team.vk.company), a single-company source with no
// per-tenant board id (boardless). The list API paginates by a next URL and carries no
// description, so each vacancy's body is scraped from its server-rendered job page (which
// exposes schema.org JobPosting microdata, like SuccessFactors).
//
// VK's edge (kittenx) rate-limit-challenges bursts by IP: once tripped it 302s to a JS
// "429" challenge page that carries no microdata, so the scraped description comes back
// empty. A concurrent fan-out trips this almost immediately. The adapter therefore fetches
// details serially with a pause between requests (interval) to stay under the limit. A
// posting that is still challenged keeps its list fields with an empty description rather
// than being dropped, so the worst case is identical to no pacing — never a regression.
// vkHTTP is the transport vk needs: a JSON listing plus HTML detail pages.
type vkHTTP interface {
	JSONGetter
	HTMLGetter
}

type vk struct {
	http     vkHTTP
	interval time.Duration
}

const (
	vkListURL    = "https://team.vk.company/career/api/v2/vacancies/?limit=50&offset=0"
	vkVacancyURL = "https://team.vk.company/vacancy/%d/"
	// vkDetailInterval paces the per-vacancy detail fetches under VK's IP rate limit.
	vkDetailInterval = 5 * time.Second
)

// NewVK builds the VK adapter over the given HTTP client.
func NewVK(c vkHTTP) Source { return newVK(c, vkDetailInterval) }

// newVK builds the adapter with an explicit detail-fetch pace, so tests can disable it.
func newVK(c vkHTTP, interval time.Duration) vk { return vk{http: c, interval: interval} }

func (vk) Provider() string { return "vk" }

// vk is single-company, so its config entries carry no board.
func (vk) Boardless() {}

// vkResult is one vacancy from the list response (no description here). Remote is a flag the
// API sets directly, so no location heuristic is needed.
type vkResult struct {
	ID    int    `json:"id"`
	Title string `json:"title"`
	Town  struct {
		Name string `json:"name"`
	} `json:"town"`
	Remote bool `json:"remote"`
}

func (a vk) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	results, err := a.list(ctx)
	if err != nil {
		return nil, err
	}

	jobs := make([]Job, 0, len(results))
	for i, r := range results {
		// Pause between detail fetches (not before the first) to stay under VK's rate
		// limit, honoring cancellation so a stopped crawl returns promptly.
		if i > 0 && a.interval > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(a.interval):
			}
		}
		if j, ok := a.detail(ctx, e, r); ok {
			jobs = append(jobs, j)
		}
	}
	return jobs, nil
}

// list pages through every vacancy by following the response's next URL until it is null.
func (a vk) list(ctx context.Context) ([]vkResult, error) {
	var results []vkResult
	for url := vkListURL; url != ""; {
		var resp struct {
			Next    string     `json:"next"`
			Results []vkResult `json:"results"`
		}
		if err := a.http.GetJSON(ctx, url, &resp); err != nil {
			return nil, fmt.Errorf("vk: list: %w", err)
		}
		results = append(results, resp.Results...)
		url = resp.Next
	}
	return results, nil
}

// detail scrapes one vacancy's description from its job page and maps it to a Job, returning
// ok=false when the page fetch fails so the caller skips just that posting.
func (a vk) detail(ctx context.Context, e CompanyEntry, r vkResult) (Job, bool) {
	url := fmt.Sprintf(vkVacancyURL, r.ID)
	root, err := a.http.GetHTML(ctx, url)
	if err != nil {
		return Job{}, false
	}

	return Job{
		ExternalID:  strconv.Itoa(r.ID),
		URL:         url,
		Title:       r.Title,
		Company:     e.Company,
		Location:    r.Town.Name,
		Description: sanitizeHTML(itempropHTML(root, "description")),
		Remote:      r.Remote,
		PostedAt:    nil,
	}, true
}

package sources

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"sync/atomic"

	"golang.org/x/net/html"
)

// selectel adapts Selectel's careers site (selectel.ru/careers), a single-company source with no
// per-tenant board id (boardless). It is a server-rendered Nuxt app, and both pages it needs carry
// their data in the page's own __NUXT_DATA__ payload (devalue-encoded, the format huntflow's
// unflattenPayload already resolves), so nothing is scraped from the markup:
//
//   - the listing, /careers/all/, holds every vacancy on one page (items_per_page 1000) with
//     everything but the body — title, city, publication time, remote flag;
//   - the vacancy page, /careers/all/vacancy/<id>/, adds short_desc, detailed_desc and
//     conditions, the body.
//
// The body costs a request per posting, so the adapter is a HydratingSource: a stored posting is
// re-listed as a SeenRefresh with no detail request.
type selectel struct {
	http HTMLGetter
}

const (
	selectelListURL    = "https://selectel.ru/careers/all/"
	selectelVacancyURL = "https://selectel.ru/careers/all/vacancy/%d/"
)

// NewSelectel builds the Selectel adapter over the given HTTP client.
func NewSelectel(c HTMLGetter) Source { return selectel{http: c} }

func (selectel) Provider() string { return "selectel" }

// selectel is single-company, so its config entries carry no board.
func (selectel) Boardless() {}

// selectelItem is one vacancy as the listing's payload carries it. The tag (the site's own
// rubric) and timetable_mode (a schedule: flexible / shift / fixed) are deliberately not read:
// the rubric is too coarse to be a category ("AI Solution Architect" and "Team lead IAM" both sit
// under backend), so the title dictionary decides, and no Job field holds a work schedule.
type selectelItem struct {
	ID   int64 `json:"id"`
	City struct {
		Name string `json:"name"`
	} `json:"city"`
	Title             string `json:"title"`
	PublishedAt       string `json:"published_at"`
	IsRemoteAvailable bool   `json:"is_remote_available"`
}

// selectelDetail is the vacancy page's payload: the listing fields plus the three body parts.
type selectelDetail struct {
	selectelItem
	ShortDesc    string `json:"short_desc"`
	DetailedDesc string `json:"detailed_desc"`
	Conditions   string `json:"conditions"`
}

func (s selectel) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return s.FetchNew(ctx, e, func(string) bool { return false })
}

// FetchNew fetches the vacancy page only for a posting the catalogue does not already have. A new
// posting whose page cannot be read is dropped rather than stored body-less: it stays unseen, so
// the next crawl retries it, while a stored body-less row would never be hydrated again once it
// aged past the pipeline's retry window.
func (s selectel) FetchNew(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	items, err := s.list(ctx)
	if err != nil {
		return nil, err
	}
	var wanted, read atomic.Int64
	jobs := fetchDetails(items, defaultDetailWorkers, func(it selectelItem) (Job, bool) {
		id := strconv.FormatInt(it.ID, 10)
		url := fmt.Sprintf(selectelVacancyURL, it.ID)
		if seen(id) {
			return Job{ExternalID: id, URL: url, Title: it.Title, Company: e.Company, SeenRefresh: true}, true
		}
		wanted.Add(1)
		job, ok := s.detail(ctx, e, it)
		if ok {
			read.Add(1)
		}
		return job, ok
	})
	// A crawl that listed new postings and read none of them is broken, not quiet.
	if wanted.Load() > 0 && read.Load() == 0 {
		return nil, fmt.Errorf("selectel: listed %d new postings and read none of their pages", wanted.Load())
	}
	return jobs, nil
}

// list reads the listing page's payload. The site serves every vacancy on page one; a payload
// that reports more pages, or fewer items than its own item_count, fails the crawl rather than
// ingesting a silently truncated catalogue.
func (s selectel) list(ctx context.Context) ([]selectelItem, error) {
	root, err := s.http.GetHTML(ctx, selectelListURL)
	if err != nil {
		return nil, fmt.Errorf("selectel: listing: %w", err)
	}
	node, err := selectelPayload(root)
	if err != nil {
		return nil, fmt.Errorf("selectel: listing: %w", err)
	}
	store := findNuxtMap(node, func(m map[string]any) bool {
		_, count := m["item_count"]
		_, items := m["items"]
		return count && items
	})
	if store == nil {
		return nil, errors.New("selectel: listing: no vacancy list in the payload")
	}
	var page struct {
		ItemCount int            `json:"item_count"`
		PageCount int            `json:"page_count"`
		Items     []selectelItem `json:"items"`
	}
	if err := remarshal(store, &page); err != nil {
		return nil, fmt.Errorf("selectel: listing: %w", err)
	}
	if page.PageCount > 1 || len(page.Items) != page.ItemCount {
		return nil, fmt.Errorf("selectel: listing: %d of %d items over %d pages, want all on one",
			len(page.Items), page.ItemCount, page.PageCount)
	}
	return page.Items, nil
}

// detail reads one vacancy page's payload and maps it to a Job, returning ok=false when the page
// cannot be read or carries no vacancy, so the caller skips just that posting.
func (s selectel) detail(ctx context.Context, e CompanyEntry, it selectelItem) (Job, bool) {
	url := fmt.Sprintf(selectelVacancyURL, it.ID)
	root, err := s.http.GetHTML(ctx, url)
	if err != nil {
		return Job{}, false
	}
	node, err := selectelPayload(root)
	if err != nil {
		return Job{}, false
	}
	vac := findNuxtMap(node, func(m map[string]any) bool {
		_, ok := m["detailed_desc"]
		return ok
	})
	if vac == nil {
		return Job{}, false
	}
	var d selectelDetail
	if err := remarshal(vac, &d); err != nil || d.ID != it.ID {
		return Job{}, false
	}

	location := firstNonEmpty(d.City.Name, it.City.Name)
	remote := d.IsRemoteAvailable || it.IsRemoteAvailable
	return Job{
		ExternalID:  strconv.FormatInt(it.ID, 10),
		URL:         url,
		Title:       firstNonEmpty(d.Title, it.Title),
		Company:     e.Company,
		Location:    location,
		Description: sanitizeHTML(d.ShortDesc + d.DetailedDesc + d.Conditions),
		Remote:      remote || isRemote(location),
		// is_remote_available is a structured flag, and a lone boolean: it says remote work is
		// on offer, never whether the alternative is onsite or hybrid.
		WorkMode: workModeFromRemote(remote),
		PostedAt: parseRFC3339(firstNonEmpty(d.PublishedAt, it.PublishedAt)),
	}, true
}

// selectelPayload decodes a page's __NUXT_DATA__ script into a plain nested value.
func selectelPayload(root *html.Node) (any, error) {
	var raw []json.RawMessage
	if err := json.Unmarshal([]byte(scriptTextByID(root, "__NUXT_DATA__")), &raw); err != nil {
		return nil, fmt.Errorf("__NUXT_DATA__: %w", err)
	}
	return unflattenPayload(raw)
}

// findNuxtMap walks a decoded Nuxt payload depth-first for the first object match accepts.
// Walking rather than assuming a key path keeps the adapter robust to Nuxt's surrounding
// store and route-state nesting.
func findNuxtMap(node any, match func(map[string]any) bool) map[string]any {
	switch n := node.(type) {
	case map[string]any:
		if match(n) {
			return n
		}
		for _, v := range n {
			if r := findNuxtMap(v, match); r != nil {
				return r
			}
		}
	case []any:
		for _, v := range n {
			if r := findNuxtMap(v, match); r != nil {
				return r
			}
		}
	}
	return nil
}

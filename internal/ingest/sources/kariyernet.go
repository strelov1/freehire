package sources

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/time/rate"
)

// kariyernet adapts Kariyer.net (kariyer.net), Turkey's largest job board, whose postings each
// name their own employer. Boardless aggregator: one crawl reads the whole catalogue (24,968 open
// postings on 2026-11-17, across every profession). The search stops at page 100 — 20,000 rows
// at 200 a page, fewer than the catalogue — so it is read as one slice per "work area" (the
// site's own department filter, 97 values that together state 24,965 of the 24,968 postings, the
// largest 4,015). Every slice is walked to its natural end; the IT areas come first so the
// detail budget reaches the technical postings first.
//
// The listing is the JSON search the site's own app calls (POST /search on the search gateway),
// 200 rows a page, newest first. It carries no body, so this is a HydratingSource that reads a
// posting's JSON from the same gateway (GET /job) only when the catalogue lacks it, up to
// kariyernetDetailBudget a run. The body is read from the posting's own page: its server-rendered
// Nuxt state carries the posting as the gateway's /job would answer it (kariyernet_page.go), and
// /job refuses the crawl hosts. The page sits behind a bot wall and is read through the Scrapling
// tier over its proxy (scraplingtier.go), while the search answers a plain client. See
// providers/kariyernet.md.
type kariyernet struct {
	api kariyernetAPI
	// detailBudget bounds one run's detail reads; non-positive takes kariyernetDetailBudget.
	detailBudget int
	// retryBackoff is the first wait before asking a refused page again; zero takes
	// kariyernetRetryBackoff.
	retryBackoff time.Duration
}

// kariyernetAPI is the two transports one crawl needs: the JSON search and the posting pages.
type kariyernetAPI interface {
	JSONPoster
	TextGetter
}

// kariyernetSplit sends the search and the pages over different clients.
type kariyernetSplit struct {
	JSONPoster
	TextGetter
}

const (
	kariyernetBase      = "https://www.kariyer.net"
	kariyernetGateway   = "https://candidatesearchapigateway.kariyer.net"
	kariyernetSearchURL = kariyernetGateway + "/search"

	// kariyernetPageSize is the largest page the search served (200 rows came back for size=200).
	kariyernetPageSize = 200
	// kariyernetMaxPages is the search's own deepest page: any currentPage above 100 is answered
	// with page 100 again. A walk that reaches it has not seen the end of the slice and fails.
	kariyernetMaxPages = 100
	// kariyernetDetailBudget bounds one run's detail reads: 1,500 at one a second is ~25 minutes,
	// beside the ~200 listing requests (~3.5 minutes) inside the scheduler's 2,400 s crawl budget.
	// The catalogue holds ~24.6k postings and a cold start reads them over ~17 runs, newest and
	// technical first; a day's new postings are a few thousand at most.
	kariyernetDetailBudget = 1500
	// kariyernetPageRetries asks a refused page again this many times: over the proxy each
	// request leaves from a different exit, and on 2026-11-17 a page was refused or served by the
	// exit it drew. The first wait is kariyernetRetryBackoff, doubling.
	kariyernetPageRetries    = 3
	kariyernetRetryBackoff   = 2 * time.Second
	kariyernetDetailWorkers  = 2
	kariyernetRequestPeriod  = time.Second
	kariyernetRequestBurst   = 1
	kariyernetCountryTurkiye = "Türkiye"
)

// kariyernetITAreas are the site's "work area" (departments) filter values that mean
// information technology: "Bilgi Teknolojileri / IT" (78), "Bilgi İşlem" (5), "Teknoloji" (55),
// "Sistem" (60) and "Program" (54). They are walked first, so the detail budget reaches them
// before the rest. The areas are the employer's own pick (a receptionist is filed under them),
// so no posting is hinted technical.
var kariyernetITAreas = []string{"78", "5", "55", "60", "54"}

// NewKariyerNet builds the Kariyer.net adapter over api, which must reach the search gateway and
// the posting pages.
func NewKariyerNet(api kariyernetAPI) Source {
	return kariyernet{api: api}
}

// pacedKariyerNet holds the search and the page reads of a run to one limiter: search over the
// search client, pages over the page client.
func pacedKariyerNet(search JSONPoster, pages TextGetter) Source {
	return NewKariyerNet(kariyernetPacer{
		inner:   kariyernetSplit{JSONPoster: search, TextGetter: pages},
		limiter: rate.NewLimiter(rate.Every(kariyernetRequestPeriod), kariyernetRequestBurst),
	})
}

// kariyernetPacer shares one limiter between the POST search and the page reads.
type kariyernetPacer struct {
	inner   kariyernetAPI
	limiter waiter
}

func (p kariyernetPacer) GetText(ctx context.Context, url string) (string, error) {
	if err := p.limiter.Wait(ctx); err != nil {
		return "", err
	}
	return p.inner.GetText(ctx, url)
}

func (p kariyernetPacer) PostJSON(ctx context.Context, url string, body, v any) error {
	if err := p.limiter.Wait(ctx); err != nil {
		return err
	}
	return p.inner.PostJSON(ctx, url, body, v)
}

func (kariyernet) Provider() string { return "kariyernet" }

// boardless: one crawl covers the whole slice, there is no per-tenant board id.
func (kariyernet) Boardless() {}

// Every posting names its own employer, so Kariyer.net stays in the source facet and in the
// cross-source duplicate suppression set.
func (kariyernet) Aggregator() {}

// kariyernetItem is one search row, reduced to what the adapter reads.
type kariyernetItem struct {
	ID           int64  `json:"id"`
	Title        string `json:"title"`
	CompanyName  string `json:"companyName"`
	Confidential bool   `json:"confidential"`
	JobURL       string `json:"jobUrl"`
}

type kariyernetSearchResponse struct {
	Data struct {
		TotalJobCount int `json:"totalJobCount"`
		Filters       struct {
			WorkAreas struct {
				Items []kariyernetFilterItem `json:"items"`
			} `json:"workAreas"`
		} `json:"filters"`
		Jobs struct {
			Items []kariyernetItem `json:"items"`
		} `json:"jobs"`
	} `json:"data"`
}

func (s kariyernet) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return s.FetchNew(ctx, e, func(string) bool { return false })
}

func (s kariyernet) FetchNew(ctx context.Context, _ CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	items, err := s.list(ctx)
	if err != nil {
		return nil, err
	}
	return s.hydrate(ctx, items, seen)
}

// list reads the whole catalogue as one slice per work area, the IT areas first. An area holds
// at most ~4,000 postings, far under the search's 20,000-row reach, and the areas overlap by
// nothing but a few sponsored rows, so the union is deduplicated by id.
func (s kariyernet) list(ctx context.Context) ([]kariyernetItem, error) {
	var head kariyernetSearchResponse
	if err := s.api.PostJSON(ctx, kariyernetSearchURL, map[string]any{"currentPage": 1, "size": 1}, &head); err != nil {
		return nil, fmt.Errorf("kariyernet: search: %w", err)
	}
	areas := kariyernetAreaOrder(head.Data.Filters.WorkAreas.Items)
	if len(areas) == 0 {
		// The filter list is how the catalogue is enumerated; without it nothing can be read whole.
		return nil, fmt.Errorf("kariyernet: the search names no work area")
	}
	var out []kariyernetItem
	byID := map[int64]bool{}
	for _, area := range areas {
		items, err := s.listArea(ctx, area)
		if err != nil {
			return nil, err
		}
		for _, it := range items {
			if !byID[it.ID] {
				byID[it.ID] = true
				out = append(out, it)
			}
		}
	}
	// The catalogue is never empty; a listing that read nothing is a changed response or a
	// refusal page.
	if len(out) == 0 {
		return nil, fmt.Errorf("kariyernet: the search carries no postings")
	}
	return out, nil
}

// kariyernetFilterItem is one value of a search filter.
type kariyernetFilterItem struct {
	Value string `json:"value"`
}

// kariyernetAreaOrder lists the work area values, the IT areas first and the rest in the filter's
// own order.
func kariyernetAreaOrder(items []kariyernetFilterItem) []string {
	var first, rest []string
	for _, it := range items {
		if slices.Contains(kariyernetITAreas, it.Value) {
			first = append(first, it.Value)
		} else if it.Value != "" {
			rest = append(rest, it.Value)
		}
	}
	return append(first, rest...)
}

// listArea walks one work area newest first until a page adds no new row. The search prepends
// sponsored rows to every page and states a total a row or two above what its pages carry (6 of
// 97 areas on 2026-11-17), so a page past the end is not empty but empty of NEW ids, and that is
// the end. Reaching the search's deepest page without it fails the crawl. An area holding no
// posting is fine.
func (s kariyernet) listArea(ctx context.Context, area string) ([]kariyernetItem, error) {
	var out []kariyernetItem
	byID := map[int64]bool{}
	for page := 1; page <= kariyernetMaxPages; page++ {
		var resp kariyernetSearchResponse
		body := map[string]any{
			"currentPage":   page,
			"size":          kariyernetPageSize,
			"departments":   []string{area},
			"sortType":      "SortByDate",
			"sortDirection": "Descending",
		}
		if err := s.api.PostJSON(ctx, kariyernetSearchURL, body, &resp); err != nil {
			return nil, fmt.Errorf("kariyernet: work area %s page %d: %w", area, page, err)
		}
		fresh := 0
		for _, it := range resp.Data.Jobs.Items {
			if it.ID <= 0 || byID[it.ID] {
				continue
			}
			byID[it.ID] = true
			out = append(out, it)
			fresh++
		}
		if fresh == 0 || len(out) >= resp.Data.TotalJobCount {
			return out, nil
		}
	}
	return nil, fmt.Errorf("kariyernet: work area %s still returned postings at page %d; the end was not reached", area, kariyernetMaxPages)
}

// hydrate buys the posting JSON only for a posting the catalogue lacks, up to the budget taken in
// listing (newest first) order. A confidential posting names no employer to file it under and is
// skipped before any request. A detail that could not be read defers the posting to a later run
// rather than storing it body-less.
func (s kariyernet) hydrate(ctx context.Context, items []kariyernetItem, seen func(string) bool) ([]Job, error) {
	budget := s.detailBudget
	if budget <= 0 {
		budget = kariyernetDetailBudget
	}
	var picked []kariyernetItem
	fresh, deferred := 0, 0
	for _, it := range items {
		if it.Confidential {
			continue
		}
		if !seen(strconv.FormatInt(it.ID, 10)) {
			if fresh == budget {
				deferred++
				continue
			}
			fresh++
		}
		picked = append(picked, it)
	}
	if deferred > 0 {
		log.Printf("kariyernet: hydration budget %d reached; %d new postings deferred to a later run", budget, deferred)
	}
	var wanted, read atomic.Int64
	jobs := fetchDetails(picked, kariyernetDetailWorkers, func(it kariyernetItem) (Job, bool) {
		id := strconv.FormatInt(it.ID, 10)
		if seen(id) {
			return Job{ExternalID: id, Title: strings.TrimSpace(it.Title), Company: strings.TrimSpace(it.CompanyName), SeenRefresh: true}, true
		}
		wanted.Add(1)
		d, err := s.readPage(ctx, id, it.JobURL)
		if err != nil {
			if detailUnreadable(err) {
				log.Printf("kariyernet: detail %s failed; deferring: %v", id, err)
			}
			return Job{}, false
		}
		job, ok := d.toJob(id)
		if !ok {
			log.Printf("kariyernet: detail %s carries no usable posting; deferring", id)
			return Job{}, false
		}
		read.Add(1)
		return job, true
	})
	// A crawl that listed new postings and read none of them is broken, not quiet.
	if wanted.Load() > 0 && read.Load() == 0 {
		return nil, fmt.Errorf("kariyernet: listed %d new postings and read none of them", wanted.Load())
	}
	return jobs, nil
}

// readPage reads a posting's page and the state it carries. A refused page (403/429) or a server
// failure is asked again, the Scrapling client retrying nothing itself.
func (s kariyernet) readPage(ctx context.Context, id, path string) (kariyernetDetail, error) {
	backoff := s.retryBackoff
	if backoff <= 0 {
		backoff = kariyernetRetryBackoff
	}
	var d kariyernetDetail
	err := retryWhile(ctx, kariyernetPageRetries, backoff, kariyernetRefused, func() error {
		page, err := s.api.GetText(ctx, kariyernetPostingURL(id, path))
		if err != nil {
			return err
		}
		d, err = kariyernetPageDetail(page)
		return err
	})
	return d, err
}

func kariyernetRefused(err error) bool {
	var se *StatusError
	return errors.As(err, &se) && (se.Code == http.StatusForbidden || se.Code == http.StatusTooManyRequests || se.Code >= 500)
}

// kariyernetPlace is one of a posting's locations.
type kariyernetPlace struct {
	CountryName string `json:"countryName"`
	CityName    string `json:"cityName"`
}

// kariyernetDetail is GET /job: only the mapped fields are named.
type kariyernetDetail struct {
	Data struct {
		General struct {
			ID                 int64             `json:"id"`
			Title              string            `json:"title"`
			JobURL             string            `json:"jobUrl"`
			Confidential       bool              `json:"confidential"`
			Qualifications     string            `json:"qualifications"`
			QualificationsHTML string            `json:"qualificationsWithHtml"`
			IsActive           bool              `json:"isActive"`
			PostingDate        string            `json:"postingDate"`
			ClosingDate        string            `json:"closingDateNumeric"`
			Locations          []kariyernetPlace `json:"locations"`
		} `json:"jobGeneralInformation"`
		Position struct {
			WorkType      string `json:"workType"`
			WorkModel     string `json:"workModel"`
			PositionLevel int    `json:"positionLevel"`
			IsRemoteWork  bool   `json:"isRemoteWork"`
		} `json:"jobPositionInformation"`
		Company struct {
			Name string `json:"companyName"`
		} `json:"jobCompanyInformation"`
	} `json:"data"`
}

func (d kariyernetDetail) toJob(id string) (Job, bool) {
	g := d.Data.General
	title := strings.Join(strings.Fields(g.Title), " ")
	company := strings.Join(strings.Fields(d.Data.Company.Name), " ")
	body := g.QualificationsHTML
	if strings.TrimSpace(body) == "" {
		body = g.Qualifications
	}
	description := sanitizeHTML(body)
	if strconv.FormatInt(g.ID, 10) != id || title == "" || company == "" || g.Confidential ||
		strings.TrimSpace(textFromHTML(description)) == "" {
		return Job{}, false
	}
	// A posting the site has taken down still answers 200 with isActive false: it is gone, and
	// storing it would reopen what the crawl closed.
	if !g.IsActive {
		return Job{}, false
	}
	pos := d.Data.Position
	workMode := kariyernetWorkModes[pos.WorkModel]
	if workMode == "" && pos.IsRemoteWork {
		workMode = "remote"
	}
	job := Job{
		ExternalID:     id,
		URL:            kariyernetPostingURL(id, g.JobURL),
		Title:          title,
		Company:        company,
		Location:       kariyernetLocation(g.Locations),
		Description:    description,
		PostedAt:       NotFuture(parseDate(g.PostingDate)),
		WorkMode:       workMode,
		Remote:         workMode == "remote",
		EmploymentType: kariyernetEmploymentTypes[pos.WorkType],
		Seniority:      kariyernetSeniorities[pos.PositionLevel],
		// A Turkish city the dictionary reads bare ("Kocaeli") carries no country on its own, and
		// the board is the Turkish one; the country is read only when the line states none.
		FallbackCountries: []string{"tr"},
	}
	// The deadline is in the future by nature, so it cannot go through the posted_at funnel.
	if t, err := time.Parse("02.01.2006", g.ClosingDate); err == nil {
		job.ApplicationDeadline = &t
	}
	return job, true
}

// kariyernetPostingURL is the posting's own page: the path the site states, else the bare id
// route, which the site redirects to it.
func kariyernetPostingURL(id, path string) string {
	if strings.HasPrefix(path, "/is-ilani/") {
		return kariyernetBase + path
	}
	return kariyernetBase + "/is-ilani/" + url.PathEscape(id)
}

// kariyernetLocation joins the posting's distinct cities, naming the country only when it is not
// Turkey ("Berlin, Almanya"), which is what lets the dictionary place an overseas posting.
func kariyernetLocation(locs []kariyernetPlace) string {
	return distinctJoin(locs, "; ", func(l kariyernetPlace) string {
		city, country := strings.TrimSpace(l.CityName), strings.TrimSpace(l.CountryName)
		if country == kariyernetCountryTurkiye {
			country = ""
		}
		return joinNonEmpty(city, country)
	})
}

// kariyernetWorkModes is the site's own work-model enum.
var kariyernetWorkModes = map[string]string{
	"OnSite": "onsite",
	"Hybrid": "hybrid",
	"Remote": "remote",
}

// kariyernetEmploymentTypes keys the site's work-type enum, the four values seen on the IT
// slice (185 FullTime, 15 Periodical, 2 Freelance, 1 PartTime of 203 rows). "Periodical" is the
// site's "Dönemsel / Proje bazlı" — seasonal or per-project work.
var kariyernetEmploymentTypes = map[string]string{
	"FullTime":   "full_time",
	"PartTime":   "part_time",
	"Periodical": "contract",
	"Freelance":  "contract",
}

// kariyernetSeniorities keys the two position levels that state an experience band on their
// own: 8 "Stajyer" and 5 "Yeni Başlayan". The rest ("Uzman", "Eleman", "Yönetici adayı") say
// nothing the title dictionary does not read better.
var kariyernetSeniorities = map[int]string{
	8: "intern",
	5: "junior",
}

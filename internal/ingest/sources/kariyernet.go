package sources

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/time/rate"
)

// kariyernet adapts Kariyer.net (kariyer.net), Turkey's largest job board, whose postings each
// name their own employer. Boardless aggregator: one crawl reads the IT slice of the catalogue.
// The board is general (24.6k open postings on 2026-11-17, most of them retail, logistics and
// industry), so the crawl is scoped to the site's own "work area" filter for information
// technology (kariyernetITAreas) rather than walking everything and leaving the classifier to
// discard 98%.
//
// The listing is the JSON search the site's own app calls (POST /search on the search gateway),
// 200 rows a page, newest first. It carries no body, so this is a HydratingSource that reads a
// posting's JSON from the same gateway (GET /job) only when the catalogue lacks it, up to
// kariyernetDetailBudget a run. The gateway answers a plain client an HTML 403 on /job and passes
// the Scrapling tier's impersonating fetcher (scraplingtier.go), so both requests go through it.
// See the Kariyer.net traps in AGENTS.md.
type kariyernet struct {
	api kariyernetAPI
	// detailBudget bounds one run's detail reads; non-positive takes kariyernetDetailBudget.
	detailBudget int
}

type kariyernetAPI interface {
	JSONGetter
	JSONPoster
}

const (
	kariyernetBase      = "https://www.kariyer.net"
	kariyernetGateway   = "https://candidatesearchapigateway.kariyer.net"
	kariyernetSearchURL = kariyernetGateway + "/search"
	kariyernetDetailURL = kariyernetGateway + "/job?jobId="

	// kariyernetPageSize is the largest page the search served (200 rows came back for size=200).
	kariyernetPageSize = 200
	// kariyernetMaxPages is the search's own deepest page: any currentPage above 100 is answered
	// with page 100 again. A walk that reaches it has not seen the end of the slice and fails.
	kariyernetMaxPages = 100
	// kariyernetDetailBudget bounds one run's detail reads. The IT slice held 437 postings on
	// 2026-11-17, so a cold start fits one run at the pace below (~8 minutes).
	kariyernetDetailBudget   = 500
	kariyernetDetailWorkers  = 2
	kariyernetRequestPeriod  = time.Second
	kariyernetRequestBurst   = 1
	kariyernetCountryTurkiye = "Türkiye"
)

// kariyernetITAreas are the site's "work area" (departments) filter values that mean
// information technology: "Bilgi Teknolojileri / IT" (78), "Bilgi İşlem" (5), "Teknoloji" (55),
// "Sistem" (60) and "Program" (54). The areas are the employer's own pick, so the slice also
// holds the odd receptionist (see the traps) and no posting is hinted technical.
var kariyernetITAreas = []string{"78", "5", "55", "60", "54"}

// NewKariyerNet builds the Kariyer.net adapter over api, which must reach the search gateway.
func NewKariyerNet(api kariyernetAPI) Source {
	return kariyernet{api: api}
}

// pacedKariyerNet holds the search and the detail reads of a run to one limiter.
func pacedKariyerNet(api kariyernetAPI) Source {
	return NewKariyerNet(kariyernetPacer{inner: api, limiter: rate.NewLimiter(rate.Every(kariyernetRequestPeriod), kariyernetRequestBurst)})
}

// kariyernetPacer shares one limiter between the POST search and the GET detail.
type kariyernetPacer struct {
	inner   kariyernetAPI
	limiter waiter
}

func (p kariyernetPacer) GetJSON(ctx context.Context, url string, v any) error {
	if err := p.limiter.Wait(ctx); err != nil {
		return err
	}
	return p.inner.GetJSON(ctx, url, v)
}

func (p kariyernetPacer) PostJSON(ctx context.Context, url string, body, v any) error {
	if err := p.limiter.Wait(ctx); err != nil {
		return err
	}
	return p.inner.PostJSON(ctx, url, body, v)
}

func (kariyernet) Provider() string { return "kariyernet" }

// boardless: one crawl covers the whole slice, there is no per-tenant board id.
func (kariyernet) boardless() {}

// Every posting names its own employer, so Kariyer.net stays in the source facet and in the
// cross-source duplicate suppression set.
func (kariyernet) aggregator() {}

// kariyernetItem is one search row, reduced to what the adapter reads.
type kariyernetItem struct {
	ID           int64  `json:"id"`
	Title        string `json:"title"`
	CompanyName  string `json:"companyName"`
	Confidential bool   `json:"confidential"`
}

type kariyernetSearchResponse struct {
	Data struct {
		TotalJobCount int `json:"totalJobCount"`
		Jobs          struct {
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

// list walks the slice newest first until a page adds no new row. The search prepends sponsored
// rows to every page, so a page past the end is not empty but empty of NEW ids, and a page's row
// count runs a few above its size.
func (s kariyernet) list(ctx context.Context) ([]kariyernetItem, error) {
	var out []kariyernetItem
	byID := map[int64]bool{}
	for page := 1; page <= kariyernetMaxPages; page++ {
		var resp kariyernetSearchResponse
		body := map[string]any{
			"currentPage":   page,
			"size":          kariyernetPageSize,
			"departments":   kariyernetITAreas,
			"sortType":      "SortByDate",
			"sortDirection": "Descending",
		}
		if err := s.api.PostJSON(ctx, kariyernetSearchURL, body, &resp); err != nil {
			if page == 1 {
				return nil, fmt.Errorf("kariyernet: search: %w", err)
			}
			return nil, fmt.Errorf("kariyernet: search page %d: %w", page, err)
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
		if page == 1 && fresh == 0 {
			// The slice always holds postings; a first page without a row is a changed
			// response or a refusal page, never an empty market.
			return nil, fmt.Errorf("kariyernet: search page 1 carries no postings")
		}
		if fresh == 0 || len(out) >= resp.Data.TotalJobCount {
			return out, nil
		}
	}
	return nil, fmt.Errorf("kariyernet: search still returned postings at page %d; the end of the slice was not reached", kariyernetMaxPages)
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
		var d kariyernetDetail
		if err := s.api.GetJSON(ctx, kariyernetDetailURL+id, &d); err != nil {
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

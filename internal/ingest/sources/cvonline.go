package sources

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/time/rate"
)

// cvonline adapts CV-Online (Alma Career), the Baltic job board run as cv.lv (Latvia) and cv.ee
// (Estonia): one platform and one id space served from one site per country, general, every
// trade, each posting naming its own employer. An aggregator whose board is the country site
// ("lv", "ee"), so each country is crawled, swept and health-tracked apart (the board names are
// distinct, so nothing is region-ambiguous). cv.lt is a different platform and not read here.
//
// The posting set is the site's jobs-sitemap.xml (robots.txt has no Disallow at all), which lists
// every open posting with its id. The sitemap carries only URLs, so the adapter hydrates the
// postings the catalogue lacks, newest id first, from the posting page: a Next.js render whose
// __NEXT_DATA__ holds the posting in full. The page's schema.org JobPosting is NOT used — its
// description is always empty. See providers/cvonline.md.
type cvonline struct {
	http         cvonlineHTTP
	limiter      waiter
	maxNewPerRun int
}

// cvonlineHTTP is the transport the adapter needs: the sitemap as a stream and each posting page
// as text.
type cvonlineHTTP interface {
	StreamGetter
	TextGetter
}

// cvonlineSite is one country's site.
type cvonlineSite struct {
	host string // the site's origin
	lang string // path segment of the site's own language
	iso  string // the board's country, ISO 3166-1 alpha-2, lower case
}

// cvonlineSites is the closed set of boards.
var cvonlineSites = map[string]cvonlineSite{
	"lv": {host: "https://www.cv.lv", lang: "lv", iso: "lv"},
	"ee": {host: "https://www.cv.ee", lang: "et", iso: "ee"},
}

const (
	cvonlineSitemapPath = "/jobs-sitemap.xml"
	// cvonlineMaxSitemapBytes bounds the sitemap (~330 KB for Latvia's 1.8k postings).
	cvonlineMaxSitemapBytes = 16 << 20
	// cvonlineRequestInterval paces sitemap and page reads alike at one a second, a courtesy
	// ceiling: a page is ~800 KB before compression.
	cvonlineRequestInterval = time.Second
	cvonlineDetailWorkers   = 2
	// cvonlineMaxNewPerRun bounds one run's page reads: 1,200 at a second each is 20 minutes,
	// leaving the rest of the crawl budget for the writes. Latvia lists ~1.8k postings and
	// Estonia ~1.2k, so a cold board drains in two runs and a day's intake is far below the bound.
	cvonlineMaxNewPerRun = 1200
	// cvonlineSweepGrace is the sitemap's own freshness (it is regenerated continuously) with room
	// for a few missed runs: a posting the sitemap stops naming is gone from the site.
	cvonlineSweepGrace = 48 * time.Hour
)

// NewCVOnline builds the CV-Online adapter over the given client.
func NewCVOnline(c cvonlineHTTP) Source {
	return cvonline{
		http:         c,
		limiter:      rate.NewLimiter(rate.Every(cvonlineRequestInterval), 1),
		maxNewPerRun: cvonlineMaxNewPerRun,
	}
}

func (cvonline) Provider() string { return "cvonline" }

// Every posting names its own employer, so CV-Online stays in the source facet and in the
// cross-source duplicate suppression set.
func (cvonline) Aggregator() {}

func (cvonline) SweepGrace() time.Duration { return cvonlineSweepGrace }

// ValidateBoard accepts only the country sites the adapter knows.
func (cvonline) ValidateBoard(board string) error {
	if _, ok := cvonlineSites[board]; !ok {
		return fmt.Errorf("cvonline: unknown board %q (want lv or ee)", board)
	}
	return nil
}

// cvonlinePosting is one sitemap entry.
type cvonlinePosting struct {
	id  int64
	url string
}

func (p cvonlinePosting) externalID() string { return strconv.FormatInt(p.id, 10) }

var cvonlinePostingPath = regexp.MustCompile(`^/[a-z]{2}/vacancy/(\d+)(?:/|$)`)

// cvonlineParsePosting reads a sitemap URL, or ok=false when it is not a posting URL.
func cvonlineParsePosting(raw string) (cvonlinePosting, bool) {
	u, err := url.Parse(raw)
	if err != nil {
		return cvonlinePosting{}, false
	}
	m := cvonlinePostingPath.FindStringSubmatch(u.Path)
	if m == nil {
		return cvonlinePosting{}, false
	}
	id, err := strconv.ParseInt(m[1], 10, 64)
	if err != nil || id <= 0 {
		return cvonlinePosting{}, false
	}
	return cvonlinePosting{id: id, url: raw}, true
}

// Fetch is the list-only fallback HydratingSource specifies: everything is unseen.
func (s cvonline) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return s.FetchNew(ctx, e, func(string) bool { return false })
}

// FetchNew lists every posting the country's sitemap names, refreshes the stored ones by
// identity and reads up to maxNewPerRun of the rest, newest id first. A posting whose page cannot
// be read is not yielded; a later run tries again.
func (s cvonline) FetchNew(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	site, ok := cvonlineSites[e.Board]
	if !ok {
		return nil, fmt.Errorf("cvonline: unknown board %q", e.Board)
	}
	postings, err := s.list(ctx, site)
	if err != nil {
		return nil, err
	}
	var jobs []Job
	var unseen []cvonlinePosting
	deferred := 0
	for _, p := range postings {
		if seen(p.externalID()) {
			jobs = append(jobs, Job{ExternalID: p.externalID(), URL: p.url, SeenRefresh: true})
			continue
		}
		if len(unseen) >= s.maxNewPerRun {
			deferred++
			continue
		}
		unseen = append(unseen, p)
	}
	var answered atomic.Int64
	read := fetchDetails(unseen, cvonlineDetailWorkers, func(p cvonlinePosting) (Job, bool) {
		job, ok, err := s.detail(ctx, site, p)
		if err != nil {
			log.Printf("cvonline: posting %d failed; deferring: %v", p.id, err)
			return unreadableDetail(p.externalID(), p.url, ""), true
		}
		answered.Add(1)
		return job, ok
	})
	// A crawl that read no page at all is broken, not quiet. "Answered" counts a posting the
	// site reported gone.
	if len(unseen) > 0 && answered.Load() == 0 {
		return nil, fmt.Errorf("cvonline: requested %d posting pages and none answered", len(unseen))
	}
	if deferred > 0 {
		log.Printf("cvonline: %d new postings past this run's bound of %d, deferred to later runs", deferred, s.maxNewPerRun)
	}
	return append(jobs, read...), nil
}

// list reads the country's sitemap, newest id first. A failed read fails the crawl: a short list
// would leave the postings it missed un-refreshed.
func (s cvonline) list(ctx context.Context, site cvonlineSite) ([]cvonlinePosting, error) {
	if err := s.limiter.Wait(ctx); err != nil {
		return nil, err
	}
	var doc sitemapDoc
	err := s.http.GetStream(ctx, site.host+cvonlineSitemapPath, "application/xml", func(r io.Reader) error {
		body, err := io.ReadAll(io.LimitReader(r, cvonlineMaxSitemapBytes+1))
		if err != nil {
			return err
		}
		if len(body) > cvonlineMaxSitemapBytes {
			return fmt.Errorf("sitemap exceeds %d bytes", cvonlineMaxSitemapBytes)
		}
		doc, err = decodeSitemapBody(body)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("cvonline: sitemap: %w", err)
	}
	ids := make(map[int64]bool)
	var out []cvonlinePosting
	for _, u := range doc.URLs {
		loc := strings.TrimSpace(u.Loc)
		if !strings.HasPrefix(loc, site.host+"/") {
			continue
		}
		p, ok := cvonlineParsePosting(loc)
		if !ok || ids[p.id] {
			continue
		}
		ids[p.id] = true
		out = append(out, p)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("cvonline: the sitemap lists no posting")
	}
	sort.Slice(out, func(i, j int) bool { return out[i].id > out[j].id })
	return out, nil
}

// detail reads one posting page. ok=false with a nil error is the site's answer that the posting
// is gone (404/410) or no longer published; an error is the absence of an answer.
func (s cvonline) detail(ctx context.Context, site cvonlineSite, p cvonlinePosting) (Job, bool, error) {
	if err := s.limiter.Wait(ctx); err != nil {
		return Job{}, false, err
	}
	page, err := s.http.GetText(ctx, p.url)
	if err != nil {
		var se *StatusError
		if errors.As(err, &se) && (se.Code == http.StatusNotFound || se.Code == http.StatusGone) {
			return Job{}, false, nil
		}
		return Job{}, false, err
	}
	state, err := cvonlineNextData(page)
	if err != nil {
		return Job{}, false, err
	}
	v, ok := state.vacancy(p.id)
	if !ok {
		return Job{}, false, fmt.Errorf("page carries no posting %d", p.id)
	}
	if v.Status != "PUBLISHED" {
		return Job{}, false, nil
	}
	job, ok := cvonlineJob(v, state.Props.PageProps.Locations, site, p)
	if !ok {
		return Job{}, false, fmt.Errorf("published posting %d has no title or employer", p.id)
	}
	return job, true, nil
}

// cvonlineState is the part of the page's __NEXT_DATA__ the adapter reads.
type cvonlineState struct {
	Props struct {
		PageProps struct {
			Locations cvonlineLocations `json:"locations"`
			State     struct {
				Public map[string]cvonlineVacancy `json:"publicVacancies"`
			} `json:"initialReduxState"`
		} `json:"pageProps"`
	} `json:"props"`
}

func (st cvonlineState) vacancy(id int64) (cvonlineVacancy, bool) {
	v, ok := st.Props.PageProps.State.Public[strconv.FormatInt(id, 10)]
	return v, ok
}

// cvonlineLocations is the page's place dictionary for its own country; towns and counties are
// named by id in a posting.
type cvonlineLocations struct {
	Countries map[string]struct {
		ISO string `json:"iso"`
	} `json:"countries"`
	Counties map[string]struct {
		Name string `json:"name"`
	} `json:"counties"`
	Towns []struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	} `json:"towns"`
}

type cvonlineVacancy struct {
	Position         string `json:"position"`
	Status           string `json:"status"`
	FirstPublishDate string `json:"firstPublishDate"`
	DateCreated      string `json:"dateCreated"`
	EmployerName     string `json:"employerName"`
	Details          struct {
		Standard []struct {
			Title   *string `json:"title"`
			Content *string `json:"content"`
		} `json:"standardDetails"`
	} `json:"details"`
	Highlights struct {
		RemoteWorkType string `json:"remoteWorkType"`
		Location       struct {
			TownID    *int64 `json:"townId"`
			CountyID  *int64 `json:"countyId"`
			CountryID *int64 `json:"countryId"`
		} `json:"location"`
		SalaryFrom *float64 `json:"salaryFrom"`
		SalaryTo   *float64 `json:"salaryTo"`
		RatePer    string   `json:"ratePer"`
		WorkTimes  []string `json:"workTimes"`
	} `json:"highlights"`
	Settings struct {
		Categories []string `json:"categories"`
	} `json:"settings"`
}

const cvonlineDataOpen = `<script id="__NEXT_DATA__" type="application/json">`

// cvonlineNextData decodes the page's embedded Next.js state.
func cvonlineNextData(page string) (cvonlineState, error) {
	var st cvonlineState
	start := strings.Index(page, cvonlineDataOpen)
	if start < 0 {
		return st, fmt.Errorf("page carries no __NEXT_DATA__")
	}
	start += len(cvonlineDataOpen)
	end := strings.Index(page[start:], "</script>")
	if end < 0 {
		return st, fmt.Errorf("__NEXT_DATA__ is not closed")
	}
	if err := json.Unmarshal([]byte(page[start:start+end]), &st); err != nil {
		return st, fmt.Errorf("decode __NEXT_DATA__: %w", err)
	}
	return st, nil
}

// cvonlineJob maps a posting to a Job. ok is false for one with no title or employer. A posting
// whose text is an attached file or an external link has no body here; it is kept with an empty
// description rather than dropped, so it is not asked for again on every run.
func cvonlineJob(v cvonlineVacancy, loc cvonlineLocations, site cvonlineSite, p cvonlinePosting) (Job, bool) {
	title := collapseSpace(v.Position)
	company := collapseSpace(v.EmployerName)
	if title == "" || company == "" {
		return Job{}, false
	}
	var body strings.Builder
	for _, d := range v.Details.Standard {
		if d.Content == nil || strings.TrimSpace(*d.Content) == "" {
			continue
		}
		if d.Title != nil && strings.TrimSpace(*d.Title) != "" {
			body.WriteString("<h3>" + html.EscapeString(strings.TrimSpace(*d.Title)) + "</h3>")
		}
		body.WriteString(*d.Content)
	}
	hl := v.Highlights
	country := site.iso
	if hl.Location.CountryID != nil {
		if c, ok := loc.Countries[strconv.FormatInt(*hl.Location.CountryID, 10)]; ok && c.ISO != "" {
			country = strings.ToLower(c.ISO)
		}
	}
	j := Job{
		ExternalID:        p.externalID(),
		URL:               p.url,
		Title:             title,
		Company:           company,
		Description:       sanitizeHTML(body.String()),
		Location:          cvonlinePlace(hl.Location.TownID, hl.Location.CountyID, loc),
		FallbackCountries: []string{country},
		PostedAt:          parseRFC3339OrDate(firstNonEmpty(v.FirstPublishDate, v.DateCreated)),
	}
	switch strings.ToUpper(hl.RemoteWorkType) {
	case "REMOTE":
		j.WorkMode, j.Remote = "remote", true
	case "HYBRID":
		j.WorkMode = "hybrid"
	case "ON_SITE":
		j.WorkMode = "onsite"
	}
	full, part := false, false
	for _, w := range hl.WorkTimes {
		switch w = strings.ToUpper(w); {
		case strings.HasPrefix(w, "FULL_TIME"): // FULL_TIME and FULL_TIME_WITH_SHIFTS
			full = true
		case strings.HasPrefix(w, "PART_TIME"):
			part = true
		}
	}
	switch {
	case full && !part:
		j.EmploymentType = "full_time"
	case part && !full:
		j.EmploymentType = "part_time"
	}
	// A monthly figure under 100 is an hourly rate typed into the wrong field (seen: 13 a month
	// on a shift fitter), not pay.
	period := cvonlineSalaryPeriod(hl.RatePer)
	if period != "" && hl.SalaryFrom != nil && *hl.SalaryFrom > 0 && (period != "month" || *hl.SalaryFrom >= 100) {
		lo := int(*hl.SalaryFrom)
		hi := lo
		if hl.SalaryTo != nil && *hl.SalaryTo >= *hl.SalaryFrom {
			hi = int(*hl.SalaryTo)
		}
		j.SalaryMin, j.SalaryMax = &lo, &hi
		j.SalaryCurrency, j.SalaryPeriod = "EUR", period
	}
	for _, c := range v.Settings.Categories {
		if c == "INFORMATION_TECHNOLOGY" {
			j.IsTechHint = true
		}
	}
	return j, true
}

func cvonlineSalaryPeriod(ratePer string) string {
	switch strings.ToUpper(ratePer) {
	case "MONTHLY":
		return "month"
	case "HOURLY":
		return "hour"
	case "YEARLY", "ANNUALLY":
		return "year"
	}
	return ""
}

// cvonlinePlace names the posting's town, else its county.
func cvonlinePlace(town, county *int64, loc cvonlineLocations) string {
	if town != nil {
		for _, t := range loc.Towns {
			if t.ID == *town {
				return collapseSpace(t.Name)
			}
		}
	}
	if county != nil {
		if c, ok := loc.Counties[strconv.FormatInt(*county, 10)]; ok {
			return collapseSpace(c.Name)
		}
	}
	return ""
}

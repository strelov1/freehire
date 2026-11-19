package sources

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

	"github.com/strelov1/freehire/internal/platform/flexjson"
	"golang.org/x/time/rate"
)

// foundit adapts foundit.in (formerly Monster India), a general Indian job board, every trade,
// whose postings each name their own employer. It is boardless and an aggregator: one crawl
// covers the whole India catalogue (~211k postings on 2026-11-17), the employer read per posting.
//
// The posting set comes from the site's sitemaps, because robots.txt disallows /middleware/,
// the JSON API the web app itself calls: the nine active-jobs files list every active posting
// and todays-jobs-sitemap the newest ones not yet in them. They carry nothing but each posting's
// URL, so every fact is bought with the posting page's schema.org JobPosting: the adapter
// hydrates only postings the catalogue lacks, newest id first, and refreshes the rest by
// identity. Akamai refuses a plain client, so production reads sitemaps and pages through the
// Scrapling tier (scraplingtier.go). See providers/foundit.md.
type foundit struct {
	http         founditHTTP
	limiter      waiter
	maxNewPerRun int
}

// founditHTTP is the transport the adapter needs: the sitemaps as a raw stream (the .gz files
// are served as bytes the adapter inflates itself) and each posting page.
type founditHTTP interface {
	StreamGetter
	HTMLGetter
}

const (
	founditBase         = "https://www.foundit.in"
	founditSitemapIndex = founditBase + "/xmlsitemap/sitemap-index.xml"
	// The index names ~60 files of search landing pages beside the posting ones; these two
	// needles select the posting files (active-jobs-sitemap0..8.xml.gz and todays-jobs-sitemap.xml).
	founditActiveNeedle = "/active-jobs-sitemap"
	founditTodayNeedle  = "/todays-jobs-sitemap"
	// founditMaxSitemapBytes bounds one sitemap file once inflated (a 25k-URL file is ~5 MB).
	founditMaxSitemapBytes = 64 << 20

	// founditSweepGrace absorbs the sitemaps' lag: the active files are regenerated every few
	// days (the copy read on 2026-11-17 was generated on the 15th), so a posting can leave
	// todays-jobs-sitemap before the active files name it and read as unseen for a while.
	founditSweepGrace = 7 * 24 * time.Hour

	// founditRequestInterval paces every request of a run, sitemap and posting page alike, at one
	// a second — a courtesy ceiling toward a large site, not a measured limit.
	founditRequestInterval = time.Second
	// founditDetailWorkers keeps enough pages in flight that the limiter, not latency, sets the pace.
	founditDetailWorkers = 2
	// founditMaxNewPerRun bounds how many unseen postings one run reads: 1,500 pages at one a
	// second is 25 minutes of the scheduler's default 2,400 s crawl budget, leaving the rest for
	// the ~10 sitemap files and the writes after the fetch (a run that meets its deadline stores
	// nothing). The site adds ~5,300 postings a day, so at the default two-hour cadence the 18,000
	// a day it takes outruns the intake and drains the ~207k cold backlog in about 12 days,
	// newest first.
	founditMaxNewPerRun = 1500
)

// NewFoundit builds the foundit adapter over the given client.
func NewFoundit(c founditHTTP) Source {
	return foundit{
		http:         c,
		limiter:      rate.NewLimiter(rate.Every(founditRequestInterval), 1),
		maxNewPerRun: founditMaxNewPerRun,
	}
}

func (foundit) Provider() string { return "foundit" }

// boardless: one crawl covers the whole national catalogue.
func (foundit) Boardless() {}

// Every posting names its own employer, so foundit stays in the source facet and in the
// cross-source duplicate suppression set.
func (foundit) Aggregator() {}

func (foundit) SweepGrace() time.Duration { return founditSweepGrace }

// founditPosting is one sitemap entry: the posting's id and URL. The URL's slug spells the
// title, employer and place run together with nothing to split them on, so none is read from it.
type founditPosting struct {
	id  int64
	url string
}

func (p founditPosting) externalID() string { return strconv.FormatInt(p.id, 10) }

// founditPostingPath matches a posting URL, /job/<slug>-<id>. The slug is cosmetic.
var founditPostingPath = regexp.MustCompile(`^/job/[^/]+-(\d+)/?$`)

// founditParsePosting reads a sitemap URL, or ok=false when it is not a posting URL.
func founditParsePosting(raw string) (founditPosting, bool) {
	u, err := url.Parse(raw)
	if err != nil {
		return founditPosting{}, false
	}
	m := founditPostingPath.FindStringSubmatch(u.Path)
	if m == nil {
		return founditPosting{}, false
	}
	id, err := strconv.ParseInt(m[1], 10, 64)
	if err != nil || id <= 0 {
		return founditPosting{}, false
	}
	return founditPosting{id: id, url: raw}, true
}

// Fetch is the list-only fallback HydratingSource specifies: everything is unseen.
func (s foundit) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return s.FetchNew(ctx, e, func(string) bool { return false })
}

// FetchNew lists every posting the sitemaps name, refreshes the ones already stored, and reads
// up to maxNewPerRun of the rest, newest id first. A posting whose page cannot be read is not
// yielded; a later run tries again.
func (s foundit) FetchNew(ctx context.Context, _ CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	postings, err := s.list(ctx)
	if err != nil {
		return nil, err
	}
	var jobs []Job
	var unseen []founditPosting
	deferred := 0
	for _, p := range postings {
		if seen(p.externalID()) {
			// The slug names no separate title or employer; the pipeline refreshes by identity.
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
	read := fetchDetails(unseen, founditDetailWorkers, func(p founditPosting) (Job, bool) {
		job, ok, err := s.detail(ctx, p)
		if err != nil {
			log.Printf("foundit: posting %d failed; deferring: %v", p.id, err)
			return Job{}, false
		}
		answered.Add(1)
		return job, ok
	})
	// A crawl that read no page at all is broken, not quiet. "Answered" counts a posting the
	// site reported gone.
	if len(unseen) > 0 && answered.Load() == 0 {
		return nil, fmt.Errorf("foundit: requested %d posting pages and none answered", len(unseen))
	}
	if deferred > 0 {
		log.Printf("foundit: %d new postings past this run's bound of %d, deferred to later runs", deferred, s.maxNewPerRun)
	}
	return append(jobs, read...), nil
}

// list reads every posting sitemap the index names and returns their postings, newest id first.
// Any file failing fails the crawl: a short list would leave the postings it missed
// un-refreshed, and the index only names files that exist.
func (s foundit) list(ctx context.Context) ([]founditPosting, error) {
	index, err := s.sitemap(ctx, founditSitemapIndex)
	if err != nil {
		return nil, fmt.Errorf("foundit: sitemap index: %w", err)
	}
	ids := make(map[int64]bool)
	var out []founditPosting
	active := 0
	for _, sm := range index.Sitemaps {
		isActive := strings.Contains(sm.Loc, founditActiveNeedle)
		if !isActive && !strings.Contains(sm.Loc, founditTodayNeedle) {
			continue
		}
		if isActive {
			active++
		}
		doc, err := s.sitemap(ctx, sm.Loc)
		if err != nil {
			return nil, fmt.Errorf("foundit: sitemap %s: %w", sm.Loc, err)
		}
		for _, u := range doc.URLs {
			p, ok := founditParsePosting(strings.TrimSpace(u.Loc))
			if !ok || ids[p.id] {
				continue
			}
			ids[p.id] = true
			out = append(out, p)
		}
	}
	if active == 0 {
		return nil, fmt.Errorf("foundit: sitemap index names no %s file", founditActiveNeedle)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("foundit: the sitemaps list no posting")
	}
	sort.Slice(out, func(i, j int) bool { return out[i].id > out[j].id })
	return out, nil
}

// sitemap fetches and decodes one sitemap file, plain or gzip. The body is read to EOF, so a
// truncated gzip file fails its checksum instead of reading as a short list.
func (s foundit) sitemap(ctx context.Context, loc string) (sitemapDoc, error) {
	if err := s.limiter.Wait(ctx); err != nil {
		return sitemapDoc{}, err
	}
	var doc sitemapDoc
	err := s.http.GetStream(ctx, loc, "application/xml, application/x-gzip", func(r io.Reader) error {
		body, err := io.ReadAll(io.LimitReader(r, founditMaxSitemapBytes+1))
		if err != nil {
			return err
		}
		if len(body) > founditMaxSitemapBytes {
			return fmt.Errorf("sitemap exceeds %d bytes", founditMaxSitemapBytes)
		}
		doc, err = decodeSitemapBody(body)
		return err
	})
	return doc, err
}

// founditLD is the part of the posting page's schema.org JobPosting the adapter reads.
// datePosted is "16-11-2026", day first. baseSalary is a MonetaryAmount whose value reads "Not
// disclosed" when the poster hid it, so its figures are read leniently.
type founditLD struct {
	Title              string `json:"title"`
	Description        string `json:"description"`
	DatePosted         string `json:"datePosted"`
	EmploymentType     string `json:"employmentType"`
	JobLocationType    string `json:"jobLocationType"`
	HiringOrganization struct {
		Name string `json:"name"`
	} `json:"hiringOrganization"`
	// JobLocation is one Place, or a list of them when a posting names several.
	JobLocation json.RawMessage `json:"jobLocation"`
	BaseSalary  struct {
		Currency string `json:"currency"`
		Value    struct {
			UnitText string         `json:"unitText"`
			MinValue flexjson.Float `json:"minValue"`
			MaxValue flexjson.Float `json:"maxValue"`
		} `json:"value"`
	} `json:"baseSalary"`
}

// detail reads one posting page. ok=false with a nil error is the site's answer that the posting
// is gone (404) or a page carrying no usable posting; an error is the absence of an answer
// (Akamai's 403 included, which says nothing about the posting).
func (s foundit) detail(ctx context.Context, p founditPosting) (Job, bool, error) {
	if err := s.limiter.Wait(ctx); err != nil {
		return Job{}, false, err
	}
	root, err := s.http.GetHTML(ctx, p.url)
	if err != nil {
		var se *StatusError
		if errors.As(err, &se) && (se.Code == http.StatusNotFound || se.Code == http.StatusGone) {
			return Job{}, false, nil
		}
		return Job{}, false, err
	}
	var ld founditLD
	if !ldJobPosting(root, &ld) {
		return Job{}, false, fmt.Errorf("page carries no JobPosting")
	}
	job, ok := founditJob(ld, p)
	return job, ok, nil
}

// founditJob maps a posting's JobPosting to a Job. ok is false for one with no title, employer
// or body.
func founditJob(ld founditLD, p founditPosting) (Job, bool) {
	title := collapseSpace(ld.Title)
	company := strings.TrimSpace(ld.HiringOrganization.Name)
	body := sanitizeHTML(ld.Description)
	if title == "" || company == "" || strings.TrimSpace(body) == "" {
		return Job{}, false
	}
	j := Job{
		ExternalID:        p.externalID(),
		URL:               p.url,
		Title:             title,
		Company:           company,
		Description:       body,
		Location:          founditPlaces(ld.JobLocation),
		FallbackCountries: []string{"in"},
		PostedAt:          parseLayout("02-01-2006", strings.TrimSpace(ld.DatePosted)),
	}
	if strings.EqualFold(strings.TrimSpace(ld.JobLocationType), "TELECOMMUTE") {
		j.WorkMode, j.Remote = "remote", true
	}
	switch strings.ToLower(strings.TrimSpace(ld.EmploymentType)) {
	case "full time", "full_time":
		j.EmploymentType = "full_time"
	case "part time", "part_time":
		j.EmploymentType = "part_time"
	case "contract", "contractor", "temporary":
		j.EmploymentType = "contract"
	case "intern", "internship":
		j.EmploymentType = "internship"
	}
	sal := ld.BaseSalary
	if cur := strings.TrimSpace(sal.Currency); cur != "" && strings.EqualFold(sal.Value.UnitText, "YEAR") &&
		sal.Value.MinValue > 0 && sal.Value.MaxValue >= sal.Value.MinValue {
		lo, hi := int(sal.Value.MinValue), int(sal.Value.MaxValue)
		j.SalaryMin, j.SalaryMax = &lo, &hi
		j.SalaryCurrency = strings.ToUpper(cur)
		j.SalaryPeriod = "year"
	}
	return j, true
}

// founditPlaces reads the addressLocality of a JobPosting's jobLocation, a Place or a list of
// them ("Bengaluru / Bangalore, Karnataka, India": city, state and country in one string), and
// joins the distinct ones.
func founditPlaces(raw json.RawMessage) string {
	type place struct {
		Address struct {
			Locality string `json:"addressLocality"`
		} `json:"address"`
	}
	var many []place
	if err := json.Unmarshal(raw, &many); err != nil {
		var one place
		if json.Unmarshal(raw, &one) != nil {
			return ""
		}
		many = []place{one}
	}
	var out []string
	seen := map[string]bool{}
	for _, pl := range many {
		if loc := collapseSpace(pl.Address.Locality); loc != "" && !seen[loc] {
			seen[loc] = true
			out = append(out, loc)
		}
	}
	return strings.Join(out, "; ")
}

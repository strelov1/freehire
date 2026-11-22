package sources

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/strelov1/freehire/internal/platform/flexjson"
	"golang.org/x/net/html"
	"golang.org/x/time/rate"
)

// hellowork adapts HelloWork (hellowork.com/fr-fr, formerly RegionsJob), a general French job
// board, every trade, whose postings each name their own employer. It is boardless and an
// aggregator.
//
// robots.txt disallows `/*?`, so every query string is off limits: the search page, its `?p=`
// pagination (the pager is a form that submits `p`) and the sitemap's host, which also answers
// Cloudflare's 403 to every client we have. What is left is the site's own landing pages with
// clean URLs: /fr-fr/emploi/domaine_<field>.html, metier_<job>.html and
// domaine_<field>-ville_<city>.html, each showing the 20 newest-ranked postings and linking a few
// dozen siblings. The crawl reads the home page's fields, then a rotating slice of the pages
// those link to, and collects the posting ids they show. So a run sees a window of the
// catalogue, never all of it: the window moves between runs (hellowork's rotation), and the sweep
// waits a posting's whole lifetime (helloworkSweepGrace) before calling an unlisted one gone.
//
// Each unseen posting's facts come from its own page's schema.org JobPosting. See
// providers/hellowork.md.
type hellowork struct {
	http         HTMLGetter
	limiter      waiter
	now          func() time.Time
	maxListPages int
	maxNewPerRun int
	retryBackoff time.Duration
}

const (
	helloworkBase = "https://www.hellowork.com"
	helloworkHome = helloworkBase + "/fr-fr/"
	// helloworkPostingURL is a posting's own page and its public URL.
	helloworkPostingURL = helloworkBase + "/fr-fr/emplois/%d.html"

	// helloworkSweepGrace is a posting's lifetime: every JobPosting states a validThrough 30
	// days after its datePosted, and a landing page lists a posting only while it ranks among
	// the newest 20 of that page.
	helloworkSweepGrace = 30 * 24 * time.Hour

	// helloworkRequestInterval paces every request of a run, listing and posting page alike, at
	// one a second, a courtesy ceiling toward a large site, not a measured limit (plain requests
	// from the crawl host were served 200 throughout the probe).
	helloworkRequestInterval = time.Second
	helloworkDetailWorkers   = 2
	// helloworkMaxListPages bounds how many landing pages a run reads, the home page's fields
	// first. Each page names ~20 postings, ~19 of them new to the run (measured over 60 pages:
	// 1,168 distinct ids).
	helloworkMaxListPages = 300
	// helloworkMaxNewPerRun bounds how many unseen postings one run reads, newest id first:
	// with the listing, ~1,500 requests at one a second is ~25 minutes, inside the scheduler's
	// 2,400 s crawl budget with room for the writes after the fetch.
	helloworkMaxNewPerRun = 1200

	helloworkRetries      = 2
	helloworkRetryBackoff = 2 * time.Second
)

// NewHelloWork builds the HelloWork adapter over the given client.
func NewHelloWork(c HTMLGetter) Source {
	return hellowork{
		http:         c,
		limiter:      rate.NewLimiter(rate.Every(helloworkRequestInterval), 1),
		now:          time.Now,
		maxListPages: helloworkMaxListPages,
		maxNewPerRun: helloworkMaxNewPerRun,
		retryBackoff: helloworkRetryBackoff,
	}
}

func (hellowork) Provider() string          { return "hellowork" }
func (hellowork) Boardless()                {}
func (hellowork) Aggregator()               {}
func (hellowork) SweepGrace() time.Duration { return helloworkSweepGrace }

var (
	// helloworkLandingPath matches a landing page the crawl may follow: a field, a job, a city or
	// a keyword, clean URL only.
	helloworkLandingPath = regexp.MustCompile(`^/fr-fr/emploi/(?:domaine|metier|mot-cle|ville)_[A-Za-z0-9_-]+\.html$`)
	helloworkPostingPath = regexp.MustCompile(`^/fr-fr/emplois/(\d+)\.html$`)
)

// Fetch is the list-only fallback HydratingSource specifies: everything is unseen.
func (s hellowork) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return s.FetchNew(ctx, e, func(string) bool { return false })
}

// FetchNew collects the posting ids the landing pages show, refreshes the ones already stored
// and reads up to maxNewPerRun of the rest, newest id first. A posting whose page cannot be read
// is not yielded; a later run tries again.
func (s hellowork) FetchNew(ctx context.Context, _ CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	ids, err := s.list(ctx)
	if err != nil {
		return nil, err
	}
	var jobs []Job
	var unseen []int64
	deferred := 0
	for _, id := range ids {
		ext := strconv.FormatInt(id, 10)
		if seen(ext) {
			jobs = append(jobs, Job{ExternalID: ext, URL: fmt.Sprintf(helloworkPostingURL, id), SeenRefresh: true})
			continue
		}
		if len(unseen) >= s.maxNewPerRun {
			deferred++
			continue
		}
		unseen = append(unseen, id)
	}
	var answered atomic.Int64
	read := fetchDetails(unseen, helloworkDetailWorkers, func(id int64) (Job, bool) {
		job, ok, err := s.detail(ctx, id)
		if err != nil {
			log.Printf("hellowork: posting %d failed; deferring: %v", id, err)
			return Job{}, false
		}
		answered.Add(1)
		return job, ok
	})
	// A crawl that read no page at all is broken, not quiet. "Answered" counts a posting the
	// site reported gone.
	if len(unseen) > 0 && answered.Load() == 0 {
		return nil, fmt.Errorf("hellowork: requested %d posting pages and none answered", len(unseen))
	}
	if deferred > 0 {
		log.Printf("hellowork: %d new postings past this run's bound of %d, deferred to later runs", deferred, s.maxNewPerRun)
	}
	return append(jobs, read...), nil
}

// list reads the home page, then the landing pages it names, then a slice of the pages those
// name, and returns every posting id shown, newest first. The home page failing fails the crawl;
// a later page failing is skipped, and a crawl in which no landing page answered fails.
func (s hellowork) list(ctx context.Context) ([]int64, error) {
	home, err := s.page(ctx, helloworkHome)
	if err != nil {
		return nil, fmt.Errorf("hellowork: home page: %w", err)
	}
	_, fields := helloworkLinks(home)
	if len(fields) == 0 {
		return nil, errors.New("hellowork: the home page names no landing page")
	}
	ids := map[int64]bool{}
	visited := map[string]bool{}
	answered := 0
	next := map[string]bool{}
	read := func(path string) error {
		visited[path] = true
		root, err := s.page(ctx, helloworkBase+path)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			log.Printf("hellowork: landing page %s failed; skipping: %v", path, err)
			return nil
		}
		answered++
		postings, links := helloworkLinks(root)
		for _, id := range postings {
			ids[id] = true
		}
		for _, l := range links {
			next[l] = true
		}
		return nil
	}
	budget := s.maxListPages
	for _, p := range fields {
		if budget == 0 {
			break
		}
		budget--
		if err := read(p); err != nil {
			return nil, err
		}
	}
	frontier := make([]string, 0, len(next))
	for l := range next {
		if !visited[l] {
			frontier = append(frontier, l)
		}
	}
	sort.Strings(frontier)
	for _, p := range helloworkRotate(frontier, s.now(), s.maxListPages) {
		if budget == 0 {
			break
		}
		if visited[p] {
			continue
		}
		budget--
		if err := read(p); err != nil {
			return nil, err
		}
	}
	if answered == 0 {
		return nil, errors.New("hellowork: no landing page answered")
	}
	out := make([]int64, 0, len(ids))
	for id := range ids {
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] > out[j] })
	return out, nil
}

// helloworkRotate starts the sorted frontier at a point that moves by one run's width every
// hour, so successive runs read different landing pages instead of the same head of the list.
func helloworkRotate(frontier []string, now time.Time, width int) []string {
	if len(frontier) == 0 || width <= 0 {
		return frontier
	}
	start := int((now.Unix() / 3600 * int64(width)) % int64(len(frontier)))
	return append(append([]string{}, frontier[start:]...), frontier[:start]...)
}

// helloworkLinks reads one page's posting ids and landing-page paths, in document order, each
// once.
func helloworkLinks(root *html.Node) (postings []int64, landings []string) {
	seenID := map[int64]bool{}
	seenPath := map[string]bool{}
	walk(root, func(n *html.Node) bool {
		if n.Type != html.ElementNode || n.Data != "a" {
			return true
		}
		href := strings.TrimSpace(Attr(n, "href"))
		if m := helloworkPostingPath.FindStringSubmatch(href); m != nil {
			if id, err := strconv.ParseInt(m[1], 10, 64); err == nil && id > 0 && !seenID[id] {
				seenID[id] = true
				postings = append(postings, id)
			}
		} else if helloworkLandingPath.MatchString(href) && !seenPath[href] {
			seenPath[href] = true
			landings = append(landings, href)
		}
		return true
	})
	return postings, landings
}

// page fetches one page under the shared pacer, asking again when the edge refuses it.
func (s hellowork) page(ctx context.Context, u string) (*html.Node, error) {
	var root *html.Node
	err := retryWhile(ctx, helloworkRetries, s.retryBackoff, helloworkRefused, func() error {
		if err := s.limiter.Wait(ctx); err != nil {
			return err
		}
		var err error
		root, err = s.http.GetHTML(ctx, u)
		return err
	})
	return root, err
}

// helloworkRefused reports an answer worth asking again: a refusal, a rate limit or a server
// failure.
func helloworkRefused(err error) bool {
	var se *StatusError
	return errors.As(err, &se) && (se.Code == http.StatusForbidden || se.Code == http.StatusTooManyRequests || se.Code >= 500)
}

// helloworkLD is the part of the posting page's schema.org JobPosting the adapter reads.
// employmentType is one string or a list; the salary is read from its numeric bounds only, since
// the page also carries the same figure as a formatted string ("38 000,00").
type helloworkLD struct {
	Title              string          `json:"title"`
	Description        string          `json:"description"`
	DatePosted         string          `json:"datePosted"`
	EmploymentType     json.RawMessage `json:"employmentType"`
	OccupationalCat    string          `json:"occupationalCategory"`
	HiringOrganization struct {
		Name string `json:"name"`
	} `json:"hiringOrganization"`
	JobLocation struct {
		Address struct {
			Locality string `json:"addressLocality"`
			Region   string `json:"addressRegion"`
		} `json:"address"`
	} `json:"jobLocation"`
	BaseSalary struct {
		Currency string `json:"currency"`
		Value    struct {
			UnitText string         `json:"unitText"`
			MinValue flexjson.Float `json:"minValue"`
			MaxValue flexjson.Float `json:"maxValue"`
		} `json:"value"`
	} `json:"baseSalary"`
}

// detail reads one posting page. ok=false with a nil error is the site's answer that the posting
// is gone (404, or 410 for an id that was never or is no longer assigned) or a page carrying no
// usable posting; an error is the absence of an answer.
func (s hellowork) detail(ctx context.Context, id int64) (Job, bool, error) {
	root, err := s.page(ctx, fmt.Sprintf(helloworkPostingURL, id))
	if err != nil {
		var se *StatusError
		if errors.As(err, &se) && (se.Code == http.StatusNotFound || se.Code == http.StatusGone) {
			return Job{}, false, nil
		}
		return Job{}, false, err
	}
	var ld helloworkLD
	if !ldJobPosting(root, &ld) {
		return Job{}, false, errors.New("page carries no JobPosting")
	}
	job, ok := helloworkJob(ld, id)
	return job, ok, nil
}

// helloworkJob maps a posting's JobPosting to a Job. ok is false for one with no title, employer
// or body.
func helloworkJob(ld helloworkLD, id int64) (Job, bool) {
	title := collapseSpace(ld.Title)
	company := strings.TrimSpace(ld.HiringOrganization.Name)
	body := sanitizeHTML(ld.Description)
	if title == "" || company == "" || strings.TrimSpace(body) == "" {
		return Job{}, false
	}
	j := Job{
		ExternalID:        strconv.FormatInt(id, 10),
		URL:               fmt.Sprintf(helloworkPostingURL, id),
		Title:             title,
		Company:           company,
		Description:       body,
		Location:          helloworkPlace(ld),
		FallbackCountries: []string{"fr"},
		PostedAt:          parseRFC3339OrDate(strings.TrimSpace(ld.DatePosted)),
		IsTechHint:        helloworkTechCategory(ld.OccupationalCat),
		EmploymentType:    helloworkEmploymentType(ld.EmploymentType),
	}
	sal := ld.BaseSalary
	period := map[string]string{"YEAR": "year", "MONTH": "month", "DAY": "day", "HOUR": "hour"}[strings.ToUpper(strings.TrimSpace(sal.Value.UnitText))]
	if cur := strings.TrimSpace(sal.Currency); cur != "" && period != "" && sal.Value.MinValue > 0 {
		lo := int(sal.Value.MinValue)
		j.SalaryMin = &lo
		if sal.Value.MaxValue >= sal.Value.MinValue {
			hi := int(sal.Value.MaxValue)
			j.SalaryMax = &hi
		}
		j.SalaryCurrency = strings.ToUpper(cur)
		j.SalaryPeriod = period
	}
	return j, true
}

// helloworkPlace joins the city and the region ("Lyon, Auvergne-Rhône-Alpes"); a posting filed
// to a whole region carries the region alone.
func helloworkPlace(ld helloworkLD) string {
	a := ld.JobLocation.Address
	var parts []string
	for _, p := range []string{a.Locality, a.Region} {
		if p = collapseSpace(p); p != "" {
			parts = append(parts, p)
		}
	}
	return strings.Join(parts, ", ")
}

// helloworkTechCategory reports HelloWork's own technical filing: the field a posting is listed
// under (occupationalCategory, the same label as the domaine_<field> landing page).
func helloworkTechCategory(cat string) bool {
	switch strings.ToLower(collapseSpace(cat)) {
	case "informatique", "data et ia":
		return true
	}
	return false
}

// helloworkEmploymentType reads employmentType, a string or a list ("TEMPORARY", "FULL_TIME"),
// to the vocabulary's value. The kind of contract wins over the hours: a fixed-term full-time
// posting is a contract.
func helloworkEmploymentType(raw json.RawMessage) string {
	var many []string
	if err := json.Unmarshal(raw, &many); err != nil {
		var one string
		if json.Unmarshal(raw, &one) != nil {
			return ""
		}
		many = []string{one}
	}
	has := func(want ...string) bool {
		for _, v := range many {
			for _, w := range want {
				if strings.EqualFold(strings.TrimSpace(v), w) {
					return true
				}
			}
		}
		return false
	}
	switch {
	case has("INTERN", "INTERNSHIP"):
		return "internship"
	case has("TEMPORARY", "CONTRACTOR", "CONTRACT"):
		return "contract"
	case has("PART_TIME"):
		return "part_time"
	case has("FULL_TIME"):
		return "full_time"
	}
	return ""
}

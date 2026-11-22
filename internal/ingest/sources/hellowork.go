package sources

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	"golang.org/x/net/html"
	"golang.org/x/time/rate"
)

// hellowork adapts HelloWork (hellowork.com/fr-fr, formerly RegionsJob), a general French job
// board, every trade, whose postings each name their own employer. It is boardless and an
// aggregator.
//
// robots.txt disallows `/*?`, so every query string is off limits: the search page, its `?p=`
// pagination (the pager is a form that submits `p`) and the sitemap, which returned a 403 from
// Cloudflare in testing. The crawl uses the site's own landing pages with
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
	http         HTMLResolvedGetter
	limiter      waiter
	now          func() time.Time
	maxListPages int
	maxNewPerRun int
	retryBackoff time.Duration
}

const (
	helloworkHost = "www.hellowork.com"
	helloworkBase = "https://" + helloworkHost
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
	// helloworkMaxNewPerRun bounds how many unseen postings one run reads. Three quarters
	// go to the newest ids and the rest to a rotating window of older unseen ids. With
	// the listing, ~1,500 requests at one a second is ~25 minutes, inside the scheduler's
	// 2,400 s crawl budget with room for the writes after the fetch.
	helloworkMaxNewPerRun = 1200

	helloworkRetries      = 2
	helloworkRetryBackoff = 2 * time.Second
)

// NewHelloWork builds the HelloWork adapter over the given client.
func NewHelloWork(c HTMLResolvedGetter) Source {
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
// and reads up to maxNewPerRun of the rest. Unreadable postings retain only their identity.
func (s hellowork) FetchNew(ctx context.Context, _ CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	ids, err := s.list(ctx)
	if err != nil {
		return nil, err
	}
	var jobs []Job
	var unseen []int64
	for _, id := range ids {
		ext := strconv.FormatInt(id, 10)
		if seen(ext) {
			jobs = append(jobs, Job{ExternalID: ext, URL: fmt.Sprintf(helloworkPostingURL, id), SeenRefresh: true})
			continue
		}
		unseen = append(unseen, id)
	}
	backlog := s.maxNewPerRun / 4
	picked := hydrationLanes(unseen, s.maxNewPerRun-backlog, backlog, s.now())
	var answered atomic.Int64
	read := fetchDetails(picked, helloworkDetailWorkers, func(id int64) (Job, bool) {
		job, ok, err := s.detail(ctx, id)
		if err != nil {
			log.Printf("hellowork: posting %d failed; deferring: %v", id, err)
			return unreadableDetail(strconv.FormatInt(id, 10), fmt.Sprintf(helloworkPostingURL, id), ""), true
		}
		answered.Add(1)
		return job, ok
	})
	// A crawl that read no page at all is broken, not quiet. "Answered" counts a posting the
	// site reported gone.
	if len(picked) > 0 && answered.Load() == 0 {
		return nil, fmt.Errorf("hellowork: requested %d posting pages and none answered", len(picked))
	}
	if deferred := len(unseen) - len(picked); deferred > 0 {
		log.Printf("hellowork: %d new postings past this run's bound of %d, deferred to later runs", deferred, s.maxNewPerRun)
	}
	return append(jobs, read...), nil
}

// list reads the home page, then the landing pages it names, then a slice of the pages those
// name, and returns every posting id shown, newest first. The home page failing fails the crawl;
// a later page failing is skipped, and a crawl in which no landing page answered fails.
func (s hellowork) list(ctx context.Context) ([]int64, error) {
	home, homeURL, err := s.page(ctx, helloworkHome)
	if err != nil {
		return nil, fmt.Errorf("hellowork: home page: %w", err)
	}
	_, fields := helloworkLinks(home, homeURL)
	if len(fields) == 0 {
		return nil, errors.New("hellowork: the home page names no landing page")
	}
	ids := map[int64]bool{}
	visited := map[string]bool{}
	answered := 0
	next := map[string]bool{}
	read := func(path string) error {
		visited[path] = true
		root, pageURL, err := s.page(ctx, helloworkBase+path)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			log.Printf("hellowork: landing page %s failed; skipping: %v", path, err)
			return nil
		}
		answered++
		postings, links := helloworkLinks(root, pageURL)
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
	for _, p := range helloworkRotate(frontier, s.now(), budget) {
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

// helloworkLinks resolves the page's links and collects each allowed posting id and landing
// path once, in document order.
func helloworkLinks(root *html.Node, pageURL string) (postings []int64, landings []string) {
	base, err := url.Parse(pageURL)
	if err != nil {
		return nil, nil
	}
	seenID := map[int64]bool{}
	seenPath := map[string]bool{}
	walk(root, func(n *html.Node) bool {
		if n.Type != html.ElementNode || n.Data != "a" {
			return true
		}
		href := strings.TrimSpace(Attr(n, "href"))
		if href == "" {
			return true
		}
		u, err := base.Parse(href)
		if err != nil || u.Scheme != "https" || !strings.EqualFold(u.Hostname(), helloworkHost) ||
			(u.Port() != "" && u.Port() != "443") || u.User != nil || u.ForceQuery || u.RawQuery != "" || u.Fragment != "" {
			return true
		}
		path := u.EscapedPath()
		if m := helloworkPostingPath.FindStringSubmatch(path); m != nil {
			if id, err := strconv.ParseInt(m[1], 10, 64); err == nil && id > 0 && !seenID[id] {
				seenID[id] = true
				postings = append(postings, id)
			}
		} else if helloworkLandingPath.MatchString(path) && !seenPath[path] {
			seenPath[path] = true
			landings = append(landings, path)
		}
		return true
	})
	return postings, landings
}

// page fetches one page under the shared pacer, asking again when the edge refuses it.
func (s hellowork) page(ctx context.Context, u string) (*html.Node, string, error) {
	var root *html.Node
	var pageURL string
	err := retryWhile(ctx, helloworkRetries, s.retryBackoff, helloworkRefused, func() error {
		if err := s.limiter.Wait(ctx); err != nil {
			return err
		}
		var err error
		root, pageURL, err = s.http.GetHTMLResolved(ctx, u)
		return err
	})
	return root, pageURL, err
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
	ValidThrough       string          `json:"validThrough"`
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
		Currency string          `json:"currency"`
		Value    json.RawMessage `json:"value"`
	} `json:"baseSalary"`
}

// detail reads one posting page. ok=false with a nil error is the site's answer that the posting
// is gone (404 or 410). An error means there was no usable answer.
func (s hellowork) detail(ctx context.Context, id int64) (Job, bool, error) {
	root, _, err := s.page(ctx, fmt.Sprintf(helloworkPostingURL, id))
	if err != nil {
		if !detailUnreadable(err) {
			return Job{}, false, nil
		}
		return Job{}, false, err
	}
	var ld helloworkLD
	if !ldJobPosting(root, &ld) {
		return Job{}, false, errors.New("page carries no JobPosting")
	}
	job, ok := helloworkJob(ld, id)
	if !ok {
		return Job{}, false, errors.New("JobPosting is missing title, employer or description")
	}
	return job, true, nil
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
		ExternalID:          strconv.FormatInt(id, 10),
		URL:                 fmt.Sprintf(helloworkPostingURL, id),
		Title:               title,
		Company:             company,
		Description:         body,
		Location:            helloworkPlace(ld),
		FallbackCountries:   []string{"fr"},
		PostedAt:            parseRFC3339OrDate(strings.TrimSpace(ld.DatePosted)),
		ApplicationDeadline: helloworkDeadline(ld.ValidThrough),
		IsTechHint:          helloworkTechCategory(ld.OccupationalCat),
		EmploymentType:      helloworkEmploymentType(ld.EmploymentType),
	}
	var bounds struct {
		UnitText string         `json:"unitText"`
		MinValue flexjson.Float `json:"minValue"`
		MaxValue flexjson.Float `json:"maxValue"`
	}
	if err := json.Unmarshal(ld.BaseSalary.Value, &bounds); err != nil {
		return j, true
	}
	period := map[string]string{"YEAR": "year", "MONTH": "month", "DAY": "day", "HOUR": "hour"}[strings.ToUpper(strings.TrimSpace(bounds.UnitText))]
	if cur := strings.TrimSpace(ld.BaseSalary.Currency); cur != "" && period != "" && bounds.MinValue > 0 {
		lo := int(bounds.MinValue)
		j.SalaryMin = &lo
		if bounds.MaxValue >= bounds.MinValue {
			hi := int(bounds.MaxValue)
			j.SalaryMax = &hi
		}
		j.SalaryCurrency = strings.ToUpper(cur)
		j.SalaryPeriod = period
	}
	return j, true
}

// helloworkDeadline preserves the stated calendar day even when the timestamp has an offset.
func helloworkDeadline(raw string) *time.Time {
	raw = strings.TrimSpace(raw)
	for _, layout := range []string{time.RFC3339Nano, time.DateOnly} {
		if t, err := time.Parse(layout, raw); err == nil {
			year, month, day := t.Date()
			deadline := time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
			return &deadline
		}
	}
	return nil
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

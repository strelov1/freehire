package sources

import (
	"context"
	"errors"
	"fmt"
	"html"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/time/rate"
)

// jobscz adapts Jobs.cz (jobs.cz), the Czech Republic's largest job board (Alma Career), whose
// postings each name their own employer. Boardless aggregator: one crawl reads the whole
// catalogue (14,971 open postings on 2026-11-22, across every field).
//
// The listing is the site's own server-rendered search, /prace/, 30 cards a page, and it stops
// after page 45: no query reaches past 1,350 rows. So the catalogue is read as one slice per
// field of work (the site's own "obor" filter), and a field holding more than that is split
// again by region (jobscz_list.go). A card names the posting, its employer, its place and its
// pay tags but no body, so this is a HydratingSource that reads the posting's page
// (/rpd/<id>/) only when the catalogue lacks it, up to jobsczDetailBudget a run.
//
// Roughly half the postings are a "partner" page: the employer runs its own career site on the
// site's platform (kaufland.jobs.cz), /rpd/<id>/ redirects there, and that page paints its body
// in the browser from an API robots.txt disallows. Such a posting has no body this adapter may
// read, so it is not yielded. See providers/jobscz.md.
type jobscz struct {
	http jobsczGetter
	// detailBudget bounds one run's detail reads; non-positive takes jobsczDetailBudget.
	detailBudget int
	// now stamps the run, which picks the rotating share of the detail budget.
	now func() time.Time
}

// jobsczGetter is the one transport a crawl needs: the pages, with the bucket cookie attached.
type jobsczGetter interface {
	GetTextWithHeaders(ctx context.Context, url string, headers map[string]string) (string, error)
}

const (
	jobsczBase = "https://www.jobs.cz"

	// jobsczPageSize is the cards a listing page carries; jobsczMaxPages is the deepest page the
	// search serves (any page above it redirects to it), so one query reaches at most
	// jobsczPageSize*jobsczMaxPages rows.
	jobsczPageSize = 30
	jobsczMaxPages = 45
	jobsczReach    = jobsczPageSize * jobsczMaxPages

	// jobsczDetailBudget bounds one run's detail reads. The listing is ~1,060 sequential pages
	// of ~240 KB (~1.3 s each from the crawl host), then at most 800 postings: ~30 minutes in all.
	// The catalogue holds ~15k postings of which about half are partner pages that cost a probe
	// each time, so a cold start reads over ~15 runs; a day's new postings are a few hundred.
	jobsczDetailBudget  = 800
	jobsczDetailWorkers = 2
	jobsczRequestPeriod = 600 * time.Millisecond
	jobsczRequestBurst  = 1
	jobsczPageRetries   = 3
	jobsczRetryBackoff  = 2 * time.Second

	// jobsczPartnerLogo marks a card whose employer runs a partner career site: its logo is served
	// from the platform's recruiting service, and none of the 14 partner cards read on 2026-11-22
	// that carried a logo was a posting with a readable page.
	jobsczPartnerLogo = "my.teamio.com/recruit/logo"
)

// jobsczHeaders pins the request to the site's control bucket. Without the cookie the edge sends
// roughly one request in three to beta.jobs.cz, a different markup this adapter does not read.
var jobsczHeaders = map[string]string{"Cookie": "beta_bucket=5%3AB"}

// jobsczFields are the site's "obor" filter values (field[0]=...): 200900001-200900045, minus
// the seven ids that match nothing. A new field is not named here until it is added.
var jobsczFields = []string{
	"200900001", "200900002", "200900003", "200900004", "200900005", "200900007", "200900008",
	"200900009", "200900011", "200900012", "200900013", "200900014", "200900015", "200900017",
	"200900018", "200900019", "200900020", "200900021", "200900022", "200900023", "200900025",
	"200900026", "200900027", "200900029", "200900030", "200900031", "200900032", "200900033",
	"200900034", "200900038", "200900039", "200900040", "200900041", "200900042", "200900043",
	"200900044", "200900045",
}

// jobsczITFields are the three IT fields, walked first so the detail budget reaches them first.
var jobsczITFields = []string{"200900011", "200900012", "200900013"}

// jobsczRegions are the path slugs of the 13 regions and the foreign slice. Together they cover
// the country; a posting with several places sits in each region it names.
var jobsczRegions = []string{
	"praha", "stredocesky-kraj", "jihocesky-kraj", "plzensky-kraj", "karlovarsky-kraj",
	"ustecky-kraj", "liberecky-kraj", "kralovehradecky-kraj", "pardubicky-kraj", "vysocina-kraj",
	"jihomoravsky-kraj", "olomoucky-kraj", "zlinsky-kraj", "moravskoslezsky-kraj", "zahranici",
}

// NewJobsCz builds the Jobs.cz adapter over g.
func NewJobsCz(g jobsczGetter) Source {
	return jobscz{http: g, now: time.Now}
}

// pacedJobsCz holds the listing and the page reads of a run to one limiter.
func pacedJobsCz(g jobsczGetter) Source {
	return NewJobsCz(jobsczPacer{
		inner:   g,
		limiter: rate.NewLimiter(rate.Every(jobsczRequestPeriod), jobsczRequestBurst),
	})
}

var errJobsCzPartner = errors.New("posting redirects to a Jobs.cz partner")

// jobsCzClient examines redirects before any request is sent to a partner host.
// The adapter paces the retries it owns, and the pinned bucket keeps listings on www.jobs.cz.
func jobsCzClient() *Client {
	c := NewClient()
	c.maxRetries = 0
	c.httpClient.CheckRedirect = jobsCzRedirect
	return c
}

func jobsCzRedirect(req *http.Request, via []*http.Request) error {
	previous := via[len(via)-1].URL
	id := strings.Trim(strings.TrimPrefix(previous.Path, "/rpd/"), "/")
	host := req.URL.Hostname()
	if previous.Hostname() == "www.jobs.cz" && strings.HasPrefix(previous.Path, "/rpd/") &&
		req.URL.Scheme == "https" && host != "www.jobs.cz" && strings.HasSuffix(host, ".jobs.cz") &&
		req.URL.Path == "/pd/"+id+"/" {
		return errJobsCzPartner
	}
	return http.ErrUseLastResponse
}

type jobsczPacer struct {
	inner   jobsczGetter
	limiter waiter
}

func (p jobsczPacer) GetTextWithHeaders(ctx context.Context, url string, headers map[string]string) (string, error) {
	if err := p.limiter.Wait(ctx); err != nil {
		return "", err
	}
	return p.inner.GetTextWithHeaders(ctx, url, headers)
}

func (jobscz) Provider() string { return "jobscz" }

// boardless: one crawl covers the whole catalogue, there is no per-tenant board id.
func (jobscz) Boardless() {}

// Every posting names its own employer, so Jobs.cz stays in the source facet and in the
// cross-source duplicate suppression set.
func (jobscz) Aggregator() {}

func (s jobscz) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return s.FetchNew(ctx, e, func(string) bool { return false })
}

func (s jobscz) FetchNew(ctx context.Context, _ CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	items, err := s.list(ctx)
	if err != nil {
		return nil, err
	}
	return s.hydrate(ctx, items, seen)
}

// get asks for a page, asking again when the edge answers a failure.
func (s jobscz) get(ctx context.Context, u string) (string, error) {
	var page string
	err := retryWhile(ctx, jobsczPageRetries, jobsczRetryBackoff, jobsczRefused, func() error {
		var err error
		page, err = s.http.GetTextWithHeaders(ctx, u, jobsczHeaders)
		return err
	})
	return page, err
}

func jobsczRefused(err error) bool {
	var se *StatusError
	return errors.As(err, &se) && (se.Code == 403 || se.Code == 429 || se.Code >= 500)
}

// jobsczItem is one listing card, reduced to what the adapter reads.
type jobsczItem struct {
	ID       string
	Title    string
	Company  string
	Location string
	// Tags are the card's neutral tags: "Možnost občasné práce z domova" and the like.
	Tags []string
	// Partner is a card whose posting lives on an employer's partner career site.
	Partner bool
}

// jobsczQuery builds a listing URL: the region path when region is set, a field filter when
// field is set, and the page.
func jobsczQuery(region, field string, page int) string {
	u := jobsczBase + "/prace/"
	q := url.Values{}
	if region != "" {
		u += region + "/"
		// A city page widens to 50 km by default; a radius of 0 keeps the page to its own region.
		q.Set("locality[radius]", "0")
	}
	if field != "" {
		q.Set("field[0]", field)
	}
	q.Set("page", strconv.Itoa(page))
	return u + "?" + jobsczEncode(q)
}

// jobsczEncode is url.Values.Encode with the brackets left bare, the form the site's own links
// use.
func jobsczEncode(q url.Values) string {
	keys := make([]string, 0, len(q))
	for k := range q {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+url.QueryEscape(q.Get(k)))
	}
	return strings.Join(parts, "&")
}

var (
	jobsczTotalRe    = regexp.MustCompile(`Našli jsme <strong>([^<]*)</strong>`)
	jobsczIDRe       = regexp.MustCompile(`data-jobad-id="(\d+)"`)
	jobsczTitleRe    = regexp.MustCompile(`data-test-ad-title="([^"]*)"`)
	jobsczCompanyRe  = regexp.MustCompile(`SearchResultCard__footerItem"\s*>\s*<span translate="no">(.*?)</span>`)
	jobsczLocalityRe = regexp.MustCompile(`(?s)data-test="serp-locality"[^>]*>\s*(.*?)\s*</li>`)
	jobsczTagRe      = regexp.MustCompile(`(?s)Tag--neutral[^>]*>\s*(.*?)\s*</span>`)
)

// jobsczTotal reads "Našli jsme 14 971 nabídek" off a listing page; ok is false when the page
// states no count (an empty search or a page that is not a listing).
func jobsczTotal(page string) (int, bool) {
	m := jobsczTotalRe.FindStringSubmatch(page)
	if m == nil {
		return 0, false
	}
	digits := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, m[1])
	n, err := strconv.Atoi(digits)
	return n, err == nil
}

// jobsczCards reads the cards of a listing page.
func jobsczCards(page string) []jobsczItem {
	var out []jobsczItem
	for _, card := range strings.Split(page, "<article")[1:] {
		card, _, _ = strings.Cut(card, "</article>")
		id := jobsczIDRe.FindStringSubmatch(card)
		if id == nil {
			continue
		}
		it := jobsczItem{
			ID:      id[1],
			Partner: strings.Contains(card, jobsczPartnerLogo),
		}
		if m := jobsczTitleRe.FindStringSubmatch(card); m != nil {
			it.Title = jobsczText(m[1])
		}
		if m := jobsczCompanyRe.FindStringSubmatch(card); m != nil {
			it.Company = jobsczText(m[1])
		}
		if m := jobsczLocalityRe.FindStringSubmatch(card); m != nil {
			it.Location = jobsczText(m[1])
		}
		for _, m := range jobsczTagRe.FindAllStringSubmatch(card, -1) {
			it.Tags = append(it.Tags, jobsczText(m[1]))
		}
		out = append(out, it)
	}
	return out
}

// jobsczText is a card's or page's inline text: tags dropped, entities decoded, whitespace
// collapsed.
func jobsczText(s string) string {
	return strings.Join(strings.Fields(html.UnescapeString(jobsczTagStripRe.ReplaceAllString(s, " "))), " ")
}

var jobsczTagStripRe = regexp.MustCompile(`<[^>]*>`)

// hydrate buys the posting page only for a posting the catalogue lacks, up to the budget. A
// partner card is skipped before any request, and a page that turns out to be a partner's is
// dropped. A page that could not be read defers the posting to a later run rather than storing
// it body-less.
func (s jobscz) hydrate(ctx context.Context, items []jobsczItem, seen func(string) bool) ([]Job, error) {
	budget := s.detailBudget
	if budget <= 0 {
		budget = jobsczDetailBudget
	}
	var picked []jobsczItem
	var fresh []jobsczItem
	for _, it := range items {
		if it.Partner {
			continue
		}
		if seen(it.ID) {
			picked = append(picked, it)
		} else {
			fresh = append(fresh, it)
		}
	}
	chosen, deferred := s.chooseFresh(fresh, budget)
	if deferred > 0 {
		log.Printf("jobscz: hydration budget %d reached; %d unseen postings deferred to a later run", budget, deferred)
	}
	picked = append(picked, chosen...)

	var answered atomic.Int64
	jobs := fetchDetails(picked, jobsczDetailWorkers, func(it jobsczItem) (Job, bool) {
		postingURL := jobsczPostingURL(it.ID)
		if seen(it.ID) {
			return Job{ExternalID: it.ID, URL: postingURL, Title: it.Title, Company: it.Company, SeenRefresh: true}, true
		}
		page, err := s.get(ctx, postingURL)
		if err != nil {
			if !detailUnreadable(err) || errors.Is(err, errJobsCzPartner) {
				answered.Add(1)
				return Job{}, false
			}
			log.Printf("jobscz: detail %s failed; deferring: %v", it.ID, err)
			return unreadableDetail(it.ID, postingURL, it.Company), true
		}
		if detail, ok := jobsczParseDetail(page); ok {
			if job, ok := detail.toJob(it); ok {
				answered.Add(1)
				return job, true
			}
		}
		log.Printf("jobscz: detail %s carries no usable posting; deferring", it.ID)
		return unreadableDetail(it.ID, postingURL, it.Company), true
	})
	if len(chosen) > 0 && answered.Load() == 0 {
		return nil, fmt.Errorf("jobscz: requested %d posting pages and none answered", len(chosen))
	}
	return jobs, nil
}

// chooseFresh takes the budget's worth of unseen postings: half newest first (ids rise with
// time), the rest a window that moves with the day through the remainder. The window is the
// point: partner pages are never stored, so they come back unseen every run, and a budget spent
// strictly newest first would stall behind the ones already probed.
func (s jobscz) chooseFresh(fresh []jobsczItem, budget int) (chosen []jobsczItem, deferred int) {
	if len(fresh) <= budget {
		return fresh, 0
	}
	slices.SortStableFunc(fresh, func(a, b jobsczItem) int {
		// Ids are fixed-width, so the string order is the numeric one.
		return strings.Compare(b.ID, a.ID)
	})
	head := budget / 2
	chosen = append(chosen, fresh[:head]...)
	rest := fresh[head:]
	window := budget - head
	now := s.now
	if now == nil {
		now = time.Now
	}
	start := (now().UTC().YearDay() * window) % len(rest)
	for i := range window {
		chosen = append(chosen, rest[(start+i)%len(rest)])
	}
	return chosen, len(fresh) - budget
}

func jobsczPostingURL(id string) string {
	return jobsczBase + "/rpd/" + url.PathEscape(id) + "/"
}

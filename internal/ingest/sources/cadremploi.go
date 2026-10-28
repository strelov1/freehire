package sources

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/time/rate"

	"github.com/strelov1/freehire/internal/dict/location"
)

// cadremploi adapts Cadremploi (www.cadremploi.fr), France's job board for managers and
// professionals ("cadres"): ~30k open postings from employers, recruitment firms and franchise
// networks, every trade, ~10% technical. It is boardless and an aggregator — the employer is read
// per posting. See the "Cadremploi traps" section of AGENTS.md for the measurements behind each
// rule.
//
// The posting set is the site's paged "online postings" sitemap, which carries only each
// posting's URL and when it was last touched, so every fact is bought with the posting page's
// schema.org JobPosting. Cloudflare refuses a plain client on every path, sitemap included; the
// impersonating fetcher of the Scrapling tier is served from the crawl host's own IP (see
// scraplingProviders), with an occasional 403 that a retry clears. The adapter hydrates only
// postings the catalogue lacks and refreshes the rest by identity.
type cadremploi struct {
	http    cadremploiHTTP
	limiter waiter
	// freshPerRun and backlogPerRun bound one run's posting pages; see cadremploiFreshPerRun.
	freshPerRun   int
	backlogPerRun int
	// deadlineMargin is how much of the run's deadline hydration leaves to the pipeline: a
	// hydrating crawl writes nothing until FetchNew returns, so one that runs into its deadline
	// stores nothing at all. Pages still unread when the margin is reached wait for a later run.
	deadlineMargin time.Duration
	// retryBackoff is the first pause before a refused request is asked again; it doubles.
	retryBackoff time.Duration
	now          func() time.Time
}

type cadremploiHTTP interface {
	XMLGetter
	HTMLGetter
}

const (
	cadremploiHost    = "www.cadremploi.fr"
	cadremploiBaseURL = "https://" + cadremploiHost
	// cadremploiSitemapURL is one page of the online-postings sitemap, numbered from 1. The
	// sitemap index names only the first page; the second exists all the same (30,149 postings
	// over two pages on 2026-10-28), and the page past the last answers 404.
	cadremploiSitemapURL = cadremploiBaseURL + "/sitemap/002_job-postings-online-%d.xml"
	// cadremploiMaxSitemapPages is a safety ceiling: the set filled two pages.
	cadremploiMaxSitemapPages = 20

	// cadremploiRequestInterval paces every request of a run, sitemap and posting page alike, at
	// one a second — a courtesy ceiling, not a measured limit.
	cadremploiRequestInterval = time.Second
	cadremploiDetailWorkers   = 2
	// cadremploiRetries bounds how often a refused (403/429) or failed (5xx) request is asked
	// again: one page in six drew a 403 at one a second, and five of six of those were served on
	// a second try two seconds later.
	cadremploiRetries      = 3
	cadremploiRetryBackoff = 3 * time.Second

	// cadremploiFreshPerRun and cadremploiBacklogPerRun split one run's 1,200 posting pages
	// (~230 KB each, ~20 minutes at one a second, leaving the rest of the scheduler's default
	// 2,400 s crawl budget to the two sitemap pages and the writes after the fetch) into
	// hydrationLanes' two lanes. The sitemap names no employer and no title, so nothing can be
	// judged before a page is bought, and a posting the pipeline discards unsaved (an employer
	// already covered first-party) stays unseen and would be bought again on every run;
	// newest-first alone would let those fill the budget for good. ~2k postings are touched a
	// day, so the first ~25 runs drain the catalogue.
	cadremploiFreshPerRun    = 900
	cadremploiBacklogPerRun  = 300
	cadremploiDeadlineMargin = 5 * time.Minute
)

// NewCadremploi builds the adapter over the given client.
func NewCadremploi(c cadremploiHTTP) Source {
	return cadremploi{
		http:           c,
		limiter:        rate.NewLimiter(rate.Every(cadremploiRequestInterval), 1),
		freshPerRun:    cadremploiFreshPerRun,
		backlogPerRun:  cadremploiBacklogPerRun,
		deadlineMargin: cadremploiDeadlineMargin,
		retryBackoff:   cadremploiRetryBackoff,
		now:            time.Now,
	}
}

func (cadremploi) Provider() string { return "cadremploi" }

// One national board, no per-tenant id.
func (cadremploi) boardless() {}

// Thousands of employers, recruitment firms and franchise networks post here, each named per
// posting.
func (cadremploi) aggregator() {}

// cadremploiPosting is one sitemap entry: the posting's id, its URL and when the site last
// touched it.
type cadremploiPosting struct {
	id      string
	url     string
	lastmod time.Time
}

// cadremploiID matches the posting id the URL's offreId parameter carries (18 digits today).
var cadremploiID = regexp.MustCompile(`^\d{6,20}$`)

// cadremploiParsePosting reads a sitemap entry, or ok=false when it is not a posting URL on the
// site's own host. The id parameter is the identity.
func cadremploiParsePosting(loc sitemapLoc) (cadremploiPosting, bool) {
	raw := strings.TrimSpace(loc.Loc)
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host != cadremploiHost || u.Path != "/emploi/detail_offre" {
		return cadremploiPosting{}, false
	}
	id := u.Query().Get("offreId")
	if !cadremploiID.MatchString(id) {
		return cadremploiPosting{}, false
	}
	p := cadremploiPosting{id: id, url: cadremploiBaseURL + "/emploi/detail_offre?offreId=" + id}
	if t := parseRFC3339(strings.TrimSpace(loc.LastMod)); t != nil {
		p.lastmod = *t
	}
	return p, true
}

// Fetch is the list-only fallback HydratingSource specifies: everything is unseen.
func (s cadremploi) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return s.FetchNew(ctx, e, func(string) bool { return false })
}

// FetchNew lists every posting the sitemap names, refreshes the ones already stored, and reads
// the pages hydrationLanes picks out of the rest. A posting whose page cannot be read is not
// yielded; a later run tries again.
func (s cadremploi) FetchNew(ctx context.Context, _ CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	postings, err := s.list(ctx)
	if err != nil {
		return nil, err
	}
	var jobs []Job
	var unseen []cadremploiPosting
	for _, p := range postings {
		if seen(p.id) {
			jobs = append(jobs, Job{ExternalID: p.id, URL: p.url, SeenRefresh: true})
			continue
		}
		unseen = append(unseen, p)
	}
	picked := hydrationLanes(unseen, s.freshPerRun, s.backlogPerRun, s.now())
	var answered, outOfTime atomic.Int64
	read := fetchDetails(picked, cadremploiDetailWorkers, func(p cadremploiPosting) (Job, bool) {
		if s.nearDeadline(ctx) {
			outOfTime.Add(1)
			return Job{}, false
		}
		job, ok, err := s.detail(ctx, p)
		if err != nil {
			log.Printf("cadremploi: posting %s failed; deferring: %v", p.id, err)
			return Job{}, false
		}
		answered.Add(1)
		return job, ok
	})
	// A crawl that read no page at all is broken, not quiet. "Answered" counts a posting the
	// site reported gone; postings left for want of time are not failures.
	if int64(len(picked)) > outOfTime.Load() && answered.Load() == 0 {
		return nil, fmt.Errorf("cadremploi: requested %d posting pages and none answered", int64(len(picked))-outOfTime.Load())
	}
	if n := outOfTime.Load(); n > 0 {
		log.Printf("cadremploi: %d new postings left unread as the run neared its deadline, deferred to later runs", n)
	}
	if deferred := len(unseen) - len(picked); deferred > 0 {
		log.Printf("cadremploi: %d new postings past this run's bound of %d, deferred to later runs", deferred, len(picked))
	}
	return append(jobs, read...), nil
}

// nearDeadline reports whether the run's deadline is closer than the margin hydration leaves to
// the pipeline's own writes.
func (s cadremploi) nearDeadline(ctx context.Context) bool {
	d, ok := ctx.Deadline()
	return ok && time.Until(d) < s.deadlineMargin
}

// list reads the sitemap's pages until one answers 404 and returns their postings, most recently
// touched first. The sitemap is the whole listing, so any other failure fails the crawl: a
// partial listing would let the company-scoped sweep close the postings of the employers it did
// see on the page it did not.
func (s cadremploi) list(ctx context.Context) ([]cadremploiPosting, error) {
	ids := make(map[string]bool)
	var out []cadremploiPosting
	for page := 1; page <= cadremploiMaxSitemapPages; page++ {
		var doc sitemapDoc
		err := s.retry(ctx, func() error {
			var err error
			doc, err = getSitemap(ctx, s.http, fmt.Sprintf(cadremploiSitemapURL, page))
			return err
		})
		var se *StatusError
		if page > 1 && errors.As(err, &se) && se.Code == http.StatusNotFound {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("cadremploi: sitemap page %d: %w", page, err)
		}
		for _, loc := range doc.URLs {
			p, ok := cadremploiParsePosting(loc)
			if !ok || ids[p.id] {
				continue
			}
			ids[p.id] = true
			out = append(out, p)
		}
		if page == cadremploiMaxSitemapPages {
			return nil, fmt.Errorf("cadremploi: sitemap still not exhausted after %d pages", page)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("cadremploi: sitemap names no posting")
	}
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].lastmod.Equal(out[j].lastmod) {
			return out[i].lastmod.After(out[j].lastmod)
		}
		return out[i].id > out[j].id
	})
	return out, nil
}

// retry paces and runs call, asking again after a refusal (403/429) or a server failure, which
// the Scrapling client does not retry itself.
func (s cadremploi) retry(ctx context.Context, call func() error) error {
	return retryWhile(ctx, cadremploiRetries, s.retryBackoff, cadremploiRefused, func() error {
		if err := s.limiter.Wait(ctx); err != nil {
			return err
		}
		return call()
	})
}

func cadremploiRefused(err error) bool {
	var se *StatusError
	return errors.As(err, &se) && (se.Code == http.StatusForbidden || se.Code == http.StatusTooManyRequests || se.Code >= 500)
}

// cadremploiLD is the part of the posting page's schema.org JobPosting the adapter reads.
type cadremploiLD struct {
	Title          string `json:"title"`
	Description    string `json:"description"`
	Qualifications string `json:"qualifications"`
	DatePosted     string `json:"datePosted"`
	EmploymentType string `json:"employmentType"`
	JobLocation    struct {
		Address struct {
			Locality string `json:"addressLocality"`
			Region   string `json:"addressRegion"`
			Country  string `json:"addressCountry"`
		} `json:"address"`
	} `json:"jobLocation"`
	HiringOrganization struct {
		Name string `json:"name"`
	} `json:"hiringOrganization"`
	BaseSalary struct {
		Currency string `json:"currency"`
		Value    struct {
			UnitText string  `json:"unitText"`
			MinValue float64 `json:"minValue"`
			MaxValue float64 `json:"maxValue"`
		} `json:"value"`
	} `json:"baseSalary"`
}

// cadremploiExpired is the notice an expired posting's page shows, under HTTP 200 and without a
// JobPosting. The sitemap still names such postings for a while: one of fifteen sampled from its
// most recently touched entries had expired that day.
const cadremploiExpired = "cette offre est expirée"

// errCadremploiNoPosting marks a page that answered without a JobPosting. It is not the site's
// answer that the posting is gone — an unknown id answers 404, and a challenge page answers
// without one too — so it counts as no answer.
var errCadremploiNoPosting = errors.New("page carries no JobPosting")

// detail reads one posting page. ok=false with a nil error is the site's answer that the posting
// is gone (404/410, or the expired notice) or carries no usable posting; an error is the absence of an answer.
func (s cadremploi) detail(ctx context.Context, p cadremploiPosting) (Job, bool, error) {
	var ld cadremploiLD
	found, expired := false, false
	err := s.retry(ctx, func() error {
		root, err := s.http.GetHTML(ctx, p.url)
		if err != nil {
			return err
		}
		found = ldJobPosting(root, &ld)
		expired = !found && strings.Contains(textContent(root), cadremploiExpired)
		return nil
	})
	if err != nil {
		var se *StatusError
		if errors.As(err, &se) && (se.Code == http.StatusNotFound || se.Code == http.StatusGone) {
			return Job{}, false, nil
		}
		return Job{}, false, err
	}
	if expired {
		return Job{}, false, nil
	}
	if !found {
		return Job{}, false, errCadremploiNoPosting
	}
	job, ok := cadremploiJob(ld, p)
	return job, ok, nil
}

// cadremploiJob maps a posting's JobPosting to a Job. ok is false for one with no title, employer
// or body.
func cadremploiJob(ld cadremploiLD, p cadremploiPosting) (Job, bool) {
	title := strings.Join(strings.Fields(ld.Title), " ")
	company := strings.Join(strings.Fields(ld.HiringOrganization.Name), " ")
	body := cadremploiBody(ld.Description)
	if q := cadremploiBody(ld.Qualifications); q != "" {
		body += "\n<h2>Profil recherché</h2>\n" + q
	}
	body = sanitizeHTML(body)
	if title == "" || company == "" || strings.TrimSpace(body) == "" {
		return Job{}, false
	}
	j := Job{
		ExternalID:        p.id,
		URL:               p.url,
		Title:             title,
		Company:           company,
		Description:       body,
		FallbackCountries: []string{"fr"},
	}
	addr := ld.JobLocation.Address
	city, region := strings.TrimSpace(addr.Locality), strings.TrimSpace(addr.Region)
	switch {
	case city != "" && region != "" && !strings.EqualFold(city, region):
		j.Location = city + ", " + region
	case city != "":
		j.Location = city
	case region != "":
		j.Location = region
	}
	if code := location.NormalizeCountry(strings.TrimSpace(addr.Country)); code != "" {
		j.Countries = []string{code}
	}
	if t := parseRFC3339(strings.TrimSpace(ld.DatePosted)); t != nil {
		j.PostedAt = NotFuture(t)
	}
	j.EmploymentType = cadremploiEmploymentType(ld.EmploymentType)
	sal := ld.BaseSalary
	if strings.EqualFold(strings.TrimSpace(sal.Value.UnitText), "YEAR") && strings.TrimSpace(sal.Currency) != "" &&
		sal.Value.MinValue > 0 && sal.Value.MaxValue >= sal.Value.MinValue {
		lo, hi := int(sal.Value.MinValue), int(sal.Value.MaxValue)
		j.SalaryMin, j.SalaryMax = &lo, &hi
		j.SalaryCurrency = strings.ToUpper(strings.TrimSpace(sal.Currency))
		j.SalaryPeriod = "year"
	}
	return j, true
}

// cadremploiBlockTag spots block markup in a JobPosting field. The site writes a posting's text
// the way its poster typed it: 31 of 49 sampled descriptions carried markup, and many of the rest
// are plain text whose line breaks are its only structure, here and there with a stray <br> or
// inline tag — so it is not escaped as literal text, only given its paragraphs.
var cadremploiBlockTag = regexp.MustCompile(`(?i)<(p|ul|ol|li|div|table|h[1-6])[\s/>]`)

var cadremploiBlankLine = regexp.MustCompile(`\n\s*\n`)

// cadremploiBody renders one JobPosting text field as HTML: markup with block structure is kept
// as it is, and anything else becomes paragraphs at its blank lines, with <br> at the rest.
func cadremploiBody(s string) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\r\n", "\n"))
	if s == "" || cadremploiBlockTag.MatchString(s) {
		return s
	}
	var out strings.Builder
	for _, para := range cadremploiBlankLine.Split(s, -1) {
		if para = strings.TrimSpace(para); para != "" {
			out.WriteString("<p>" + strings.ReplaceAll(para, "\n", "<br>\n") + "</p>\n")
		}
	}
	return out.String()
}

// cadremploiEmploymentType maps the JobPosting's employment type. "CONTRACTOR" is what the site
// writes for a fixed-term contract, an interim mission and a franchise or independent agent
// alike, all of which our vocabulary files under contract; "OTHER" says nothing.
func cadremploiEmploymentType(s string) string {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "FULL_TIME":
		return "full_time"
	case "PART_TIME":
		return "part_time"
	case "CONTRACTOR", "TEMPORARY":
		return "contract"
	case "INTERN":
		return "internship"
	}
	return ""
}

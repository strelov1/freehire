package sources

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/time/rate"
)

// cryptojobslist adapts CryptoJobsList (cryptojobslist.com), the web3 and crypto job board: ~650
// open postings, most of them reposts of the employer's own Ashby, Lever, Greenhouse or Workday
// board. It is boardless and an aggregator — the employer is read per posting. See the
// "CryptoJobsList traps" section of AGENTS.md for the measurements behind each rule.
//
// The posting set is the jobs sitemap, which carries each posting's slug and last-modified time;
// every fact is bought with the posting page's Next.js state (__NEXT_DATA__ job), which holds the
// whole posting as structured fields. Cloudflare answers a plain client 403 on every path; the
// impersonating fetcher of the Scrapling tier is served (see scraplingProviders). The adapter
// hydrates only postings the catalogue lacks and refreshes the rest by identity.
type cryptojobslist struct {
	sitemap        XMLGetter
	pages          HTMLGetter
	limiter        waiter
	fresh, backlog int
	deadlineMargin time.Duration
	now            func() time.Time
}

const (
	cryptojobslistHost       = "cryptojobslist.com"
	cryptojobslistBaseURL    = "https://" + cryptojobslistHost
	cryptojobslistSitemapURL = cryptojobslistBaseURL + "/sitemap-jobs.xml"
	cryptojobslistJobPrefix  = "/jobs/"

	// cryptojobslistRequestInterval paces every posting page at one a second — a courtesy
	// ceiling, not a measured limit.
	cryptojobslistRequestInterval = time.Second
	cryptojobslistDetailWorkers   = 2
	// A run reads the newest 300 unseen postings and a moving backlog window of 150: the
	// catalogue is ~650 postings, and the ones the pipeline discards unsaved (an employer an ATS
	// already covers) stay unseen, so a newest-only window would stall behind them.
	cryptojobslistFresh          = 300
	cryptojobslistBacklog        = 150
	cryptojobslistDeadlineMargin = 5 * time.Minute
)

// NewCryptoJobsList builds the adapter: the sitemap is read over sitemap, the posting pages over
// pages.
func NewCryptoJobsList(sitemap XMLGetter, pages HTMLGetter) Source {
	return cryptojobslist{
		sitemap:        sitemap,
		pages:          pages,
		limiter:        rate.NewLimiter(rate.Every(cryptojobslistRequestInterval), 1),
		fresh:          cryptojobslistFresh,
		backlog:        cryptojobslistBacklog,
		deadlineMargin: cryptojobslistDeadlineMargin,
		now:            time.Now,
	}
}

func (cryptojobslist) Provider() string { return "cryptojobslist" }

// One board, no per-tenant id.
func (cryptojobslist) boardless() {}

// Hundreds of employers post here, each named per posting.
func (cryptojobslist) aggregator() {}

// cryptojobslistPosting is one sitemap entry: the slug that identifies the posting, its URL and
// the title the slug spells (employer suffix included — only the identity is relied on).
type cryptojobslistPosting struct {
	slug    string
	url     string
	title   string
	lastMod string
}

func cryptojobslistParsePosting(loc sitemapLoc) (cryptojobslistPosting, bool) {
	raw := strings.TrimSpace(loc.Loc)
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host != cryptojobslistHost || !strings.HasPrefix(u.Path, cryptojobslistJobPrefix) {
		return cryptojobslistPosting{}, false
	}
	slug := strings.TrimPrefix(u.Path, cryptojobslistJobPrefix)
	if slug == "" || strings.Contains(slug, "/") {
		return cryptojobslistPosting{}, false
	}
	title := strings.Join(strings.Fields(strings.ReplaceAll(slug, "-", " ")), " ")
	return cryptojobslistPosting{slug: slug, url: raw, title: title, lastMod: strings.TrimSpace(loc.LastMod)}, true
}

// Fetch is the list-only fallback HydratingSource specifies: everything is unseen.
func (s cryptojobslist) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return s.FetchNew(ctx, e, func(string) bool { return false })
}

// FetchNew lists the sitemap, refreshes the postings already stored, and reads a bounded slice of
// the rest. A posting whose page cannot be read is not yielded; a later run tries again.
func (s cryptojobslist) FetchNew(ctx context.Context, _ CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	postings, err := s.list(ctx)
	if err != nil {
		return nil, err
	}
	var jobs []Job
	var unseen []cryptojobslistPosting
	for _, p := range postings {
		if seen(p.slug) {
			jobs = append(jobs, Job{ExternalID: p.slug, URL: p.url, Title: p.title, SeenRefresh: true})
			continue
		}
		unseen = append(unseen, p)
	}
	picked := hydrationLanes(unseen, s.fresh, s.backlog, s.now())
	var answered, outOfTime atomic.Int64
	read := fetchDetails(picked, cryptojobslistDetailWorkers, func(p cryptojobslistPosting) (Job, bool) {
		if d, ok := ctx.Deadline(); ok && time.Until(d) < s.deadlineMargin {
			outOfTime.Add(1)
			return Job{}, false
		}
		job, ok, err := s.detail(ctx, p)
		if err != nil {
			log.Printf("cryptojobslist: posting %s failed; deferring: %v", p.slug, err)
			return Job{}, false
		}
		answered.Add(1)
		return job, ok
	})
	if asked := int64(len(picked)) - outOfTime.Load(); asked > 0 && answered.Load() == 0 {
		return nil, fmt.Errorf("cryptojobslist: requested %d posting pages and none answered", asked)
	}
	if n := outOfTime.Load(); n > 0 {
		log.Printf("cryptojobslist: %d new postings left unread as the run neared its deadline, deferred to later runs", n)
	}
	return append(jobs, read...), nil
}

// list reads the jobs sitemap, newest first. It failing fails the crawl.
func (s cryptojobslist) list(ctx context.Context) ([]cryptojobslistPosting, error) {
	doc, err := getSitemap(ctx, s.sitemap, cryptojobslistSitemapURL)
	if err != nil {
		return nil, fmt.Errorf("cryptojobslist: sitemap: %w", err)
	}
	slugs := make(map[string]bool)
	var out []cryptojobslistPosting
	for _, u := range doc.URLs {
		p, ok := cryptojobslistParsePosting(u)
		if !ok || slugs[p.slug] {
			continue
		}
		slugs[p.slug] = true
		out = append(out, p)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("cryptojobslist: sitemap lists no posting")
	}
	// lastmod is an RFC 3339 timestamp, so it orders as text.
	sort.SliceStable(out, func(i, j int) bool { return out[i].lastMod > out[j].lastMod })
	return out, nil
}

// cryptojobslistNextData is the part of the posting page's __NEXT_DATA__ the adapter reads.
type cryptojobslistNextData struct {
	Props struct {
		PageProps struct {
			Job *cryptojobslistJob `json:"job"`
		} `json:"pageProps"`
	} `json:"props"`
}

type cryptojobslistJob struct {
	Title          string   `json:"jobTitle"`
	Description    string   `json:"jobDescription"`
	Location       string   `json:"jobLocation"`
	Company        string   `json:"companyName"`
	Remote         bool     `json:"remote"`
	EmploymentType []string `json:"employmentType"`
	CreatedAt      string   `json:"createdAt"`
	PublishedAt    string   `json:"publishedAt"`
	Published      bool     `json:"published"`
	Active         bool     `json:"isActive"`
	Filled         bool     `json:"filled"`
	Removed        bool     `json:"removed"`
	Salary         *struct {
		Currency string  `json:"currency"`
		Min      float64 `json:"minValue"`
		Max      float64 `json:"maxValue"`
		Unit     string  `json:"unitText"`
	} `json:"salary"`
}

// errCryptojobslistNoPosting marks a page that answered without a posting. It is not the site's
// answer that the posting is gone — a Cloudflare interstitial answers the same way — so it counts
// as no answer.
var errCryptojobslistNoPosting = errors.New("page carries no posting state")

// detail reads one posting page. ok=false with a nil error is the site's answer that the posting
// is gone (404/410, or its own removed/filled flags); an error is the absence of an answer.
func (s cryptojobslist) detail(ctx context.Context, p cryptojobslistPosting) (Job, bool, error) {
	if err := s.limiter.Wait(ctx); err != nil {
		return Job{}, false, err
	}
	root, err := s.pages.GetHTML(ctx, p.url)
	if err != nil {
		var se *StatusError
		if errors.As(err, &se) && (se.Code == http.StatusNotFound || se.Code == http.StatusGone) {
			return Job{}, false, nil
		}
		return Job{}, false, err
	}
	raw := scriptTextByID(root, "__NEXT_DATA__")
	var nd cryptojobslistNextData
	if raw == "" || json.Unmarshal([]byte(raw), &nd) != nil || nd.Props.PageProps.Job == nil {
		return Job{}, false, errCryptojobslistNoPosting
	}
	job, ok := cryptojobslistMap(*nd.Props.PageProps.Job, p)
	return job, ok, nil
}

// cryptojobslistMap maps a posting's state to a Job. ok is false for a posting the site itself has
// taken down and for one with no title, employer or body.
func cryptojobslistMap(cj cryptojobslistJob, p cryptojobslistPosting) (Job, bool) {
	if !cj.Published || !cj.Active || cj.Filled || cj.Removed {
		return Job{}, false
	}
	title := strings.Join(strings.Fields(cj.Title), " ")
	company := strings.TrimSpace(cj.Company)
	body := sanitizeHTML(cj.Description)
	if title == "" || company == "" || strings.TrimSpace(body) == "" {
		return Job{}, false
	}
	j := Job{
		ExternalID:  p.slug,
		URL:         p.url,
		Title:       title,
		Company:     company,
		Location:    strings.TrimSpace(cj.Location),
		Description: body,
	}
	if cj.Remote {
		j.WorkMode, j.Remote = "remote", true
	}
	// createdAt is when the posting first appeared; publishedAt is moved forward by the site's own
	// refresh (39 of 48 sampled postings differed, most by weeks), so it would pass an old posting
	// off as new.
	posted := strings.TrimSpace(cj.CreatedAt)
	if posted == "" {
		posted = strings.TrimSpace(cj.PublishedAt)
	}
	if t, err := time.Parse(time.RFC3339, posted); err == nil {
		j.PostedAt = NotFuture(&t)
	}
	for _, t := range cj.EmploymentType {
		switch strings.ToUpper(strings.TrimSpace(t)) {
		case "FULL_TIME":
			j.EmploymentType = "full_time"
		case "PART_TIME":
			j.EmploymentType = "part_time"
		case "CONTRACTOR", "CONTRACT":
			j.EmploymentType = "contract"
		case "INTERN", "INTERNSHIP":
			j.EmploymentType = "internship"
		}
		if j.EmploymentType != "" {
			break
		}
	}
	if sal := cj.Salary; sal != nil {
		period := map[string]string{"YEAR": "year", "MONTH": "month", "DAY": "day", "HOUR": "hour"}[strings.ToUpper(strings.TrimSpace(sal.Unit))]
		if cur := strings.TrimSpace(sal.Currency); cur != "" && period != "" && sal.Min > 0 && sal.Max >= sal.Min {
			lo, hi := int(sal.Min), int(sal.Max)
			j.SalaryMin, j.SalaryMax = &lo, &hi
			j.SalaryCurrency = strings.ToUpper(cur)
			j.SalaryPeriod = period
		}
	}
	return j, true
}

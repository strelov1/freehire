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
	"slices"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/net/html"
	"golang.org/x/time/rate"

	"github.com/strelov1/freehire/internal/dict/location"
)

// jobteaser adapts JobTeaser (www.jobteaser.com), the European careers platform of business and
// engineering schools: internships, work-study, graduate programmes and first jobs at large
// employers (KPMG, Capgemini, BNP Paribas, L'Oréal), ~32k open postings across six languages.
// It is an aggregator — the employer is read per posting — whose board is the LANGUAGE a posting
// is published in ("fr", "de", "en", ...), the prefix of its URL; one posting has one primary
// language, so the boards partition the catalogue. See the "JobTeaser traps" section of
// AGENTS.md for the measurements behind each rule.
//
// The posting set is the job-ads sitemaps, which carry each posting's URL and when it was last
// touched. The URL spells the employer and the title after the posting's id, and the companies
// sitemap names the employers with a company page (68% of the French postings), so a posting of an
// employer freehire already covers first-party is skipped before its page is bought. The pages
// sit behind a Cloudflare challenge that only the Scrapling tier's browser passes (see
// scraplingProviders); the sitemaps are on a CDN with no challenge and stay on a plain client.
type jobteaser struct {
	sitemap XMLGetter
	pages   HTMLGetter
	limiter waiter
	// freshPerRun and backlogPerRun bound one run's posting pages; see jobteaserFreshPerRun.
	freshPerRun   int
	backlogPerRun int
	// deadlineMargin is how much of the run's deadline hydration leaves to the pipeline: a
	// hydrating crawl writes nothing until FetchNewGated returns, so one that runs into its
	// deadline stores nothing at all.
	deadlineMargin time.Duration
	now            func() time.Time
}

const (
	jobteaserHost     = "www.jobteaser.com"
	jobteaserSitemaps = "https://assets-cf.jobteaser.com/sitemaps/sitemap.xml"

	// jobteaserRequestInterval paces every request of a run. A browser page takes ~8 s on a warm
	// session, so the workers, not this ceiling, set the pace.
	jobteaserRequestInterval = time.Second
	// jobteaserDetailWorkers keeps three pages in flight on the Scrapling browser session, which
	// serves up to four tabs at once.
	jobteaserDetailWorkers = 3
	// jobteaserFreshPerRun and jobteaserBacklogPerRun split one run's 400 posting pages (~8 s
	// each over three tabs, ~18 minutes, plus ~2 minutes for the first page's challenge) into
	// hydrationLanes' two lanes. A posting whose employer the companies sitemap does not name
	// (a third of the French ones) cannot be judged before its page is bought, and one the
	// pipeline then discards unsaved would be bought again on every run; the moving backlog
	// lane keeps those from filling the budget for good. ~700 French postings are touched a day.
	jobteaserFreshPerRun    = 300
	jobteaserBacklogPerRun  = 100
	jobteaserDeadlineMargin = 5 * time.Minute
)

// NewJobTeaser builds the adapter: the sitemaps are read over sitemap, the posting pages over
// pages.
func NewJobTeaser(sitemap XMLGetter, pages HTMLGetter) Source {
	return jobteaser{
		sitemap:        sitemap,
		pages:          pages,
		limiter:        rate.NewLimiter(rate.Every(jobteaserRequestInterval), 1),
		freshPerRun:    jobteaserFreshPerRun,
		backlogPerRun:  jobteaserBacklogPerRun,
		deadlineMargin: jobteaserDeadlineMargin,
		now:            time.Now,
	}
}

func (jobteaser) Provider() string { return "jobteaser" }

// Thousands of employers post here, each named per posting.
func (jobteaser) aggregator() {}

// jobteaserLanguage is a board: a two-letter URL prefix.
var jobteaserLanguage = regexp.MustCompile(`^[a-z]{2}$`)

// jobteaserPostingPath matches a posting URL, /<language>/job-offers/<uuid>-<employer>-<title>.
var jobteaserPostingPath = regexp.MustCompile(`^/([a-z]{2})/job-offers/([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})-([a-z0-9-]+)$`)

// jobteaserCompanyPath matches a company page URL, /<language>/companies/<slug>.
var jobteaserCompanyPath = regexp.MustCompile(`^/([a-z]{2})/companies/([a-z0-9-]+)$`)

// jobteaserPosting is one sitemap entry. employer is the company slug the URL opens with when the
// companies sitemap names it, and title is the rest of the URL's slug, de-slugged; both are what
// the gate judges before the page is bought.
type jobteaserPosting struct {
	id       string
	url      string
	slug     string
	employer string
	title    string
	lastmod  time.Time
}

// jobteaserParsePosting reads a sitemap entry, or ok=false when it is not a posting of the given
// language on the site's own host.
func jobteaserParsePosting(loc sitemapLoc, lang string) (jobteaserPosting, bool) {
	raw := strings.TrimSpace(loc.Loc)
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host != jobteaserHost {
		return jobteaserPosting{}, false
	}
	m := jobteaserPostingPath.FindStringSubmatch(u.Path)
	if m == nil || m[1] != lang {
		return jobteaserPosting{}, false
	}
	p := jobteaserPosting{id: m[2], url: "https://" + jobteaserHost + u.Path, slug: m[3]}
	if t := parseRFC3339(strings.TrimSpace(loc.LastMod)); t != nil {
		p.lastmod = *t
	}
	return p, true
}

// resolve splits the posting's slug into the longest leading company slug the companies sitemap
// names and the title after it. A posting whose employer has no company page keeps no employer,
// and its whole slug stands in for the title.
func (p *jobteaserPosting) resolve(companies map[string]bool) {
	words := strings.Split(p.slug, "-")
	for k := min(len(words)-1, 8); k > 0; k-- {
		if c := strings.Join(words[:k], "-"); companies[c] {
			p.employer = c
			p.title = strings.Join(words[k:], " ")
			return
		}
	}
	p.title = strings.Join(words, " ")
}

// Fetch is the list-only fallback HydratingSource specifies: everything is unseen.
func (s jobteaser) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return s.FetchNew(ctx, e, func(string) bool { return false })
}

func (s jobteaser) FetchNew(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	return s.FetchNewGated(ctx, e, seen, DetailGate{})
}

// FetchNewGated lists the board's postings, refreshes the ones already stored, yields body-less
// the ones the gate condemns on what their URL spells, and reads the pages hydrationLanes picks
// out of the rest. A posting whose page cannot be read is not yielded; a later run tries again.
func (s jobteaser) FetchNewGated(ctx context.Context, e CompanyEntry, seen func(externalID string) bool, gate DetailGate) ([]Job, error) {
	lang := strings.TrimSpace(e.Board)
	if !jobteaserLanguage.MatchString(lang) {
		return nil, fmt.Errorf("jobteaser: board %q is not a two-letter language", e.Board)
	}
	postings, err := s.list(ctx, lang)
	if err != nil {
		return nil, err
	}
	var covered map[string]bool
	if gate.Covered != nil {
		var employers []string
		for _, p := range postings {
			if p.employer != "" {
				employers = append(employers, p.employer)
			}
		}
		covered = gate.Covered(employers)
	}
	var jobs []Job
	var unseen []jobteaserPosting
	for _, p := range postings {
		base := Job{ExternalID: p.id, URL: p.url, Title: p.title, Company: p.employer}
		if seen(p.id) {
			base.SeenRefresh = true
			jobs = append(jobs, base)
			continue
		}
		if (p.employer != "" && covered[p.employer]) || (gate.Rejected != nil && gate.Rejected(base)) {
			jobs = append(jobs, base)
			continue
		}
		unseen = append(unseen, p)
	}
	picked := hydrationLanes(unseen, s.freshPerRun, s.backlogPerRun, s.now())
	var answered, outOfTime atomic.Int64
	read := fetchDetails(picked, jobteaserDetailWorkers, func(p jobteaserPosting) (Job, bool) {
		if d, ok := ctx.Deadline(); ok && time.Until(d) < s.deadlineMargin {
			outOfTime.Add(1)
			return Job{}, false
		}
		job, ok, err := s.detail(ctx, p)
		if err != nil {
			log.Printf("jobteaser: posting %s failed; deferring: %v", p.id, err)
			return Job{}, false
		}
		answered.Add(1)
		return job, ok
	})
	if asked := int64(len(picked)) - outOfTime.Load(); asked > 0 && answered.Load() == 0 {
		return nil, fmt.Errorf("jobteaser: requested %d posting pages and none answered", asked)
	}
	if n := outOfTime.Load(); n > 0 {
		log.Printf("jobteaser: %d new postings left unread as the run neared its deadline, deferred to later runs", n)
	}
	if deferred := len(unseen) - len(picked); deferred > 0 {
		log.Printf("jobteaser: %d new postings past this run's bound of %d, deferred to later runs", deferred, len(picked))
	}
	return append(jobs, read...), nil
}

// list reads every job-ads sitemap the index names and returns the board's postings, most
// recently touched first, each resolved against the companies sitemap. A job-ads sitemap failing
// fails the crawl — a partial listing would let the company-scoped sweep close the postings of
// the employers it did see in the sitemaps it did not read — while the companies sitemap failing
// costs only the gate.
func (s jobteaser) list(ctx context.Context, lang string) ([]jobteaserPosting, error) {
	if err := s.limiter.Wait(ctx); err != nil {
		return nil, err
	}
	index, err := getSitemap(ctx, s.sitemap, jobteaserSitemaps)
	if err != nil {
		return nil, fmt.Errorf("jobteaser: sitemap index: %w", err)
	}
	var companies map[string]bool
	ids := make(map[string]bool)
	var out []jobteaserPosting
	jobSitemaps := 0
	for _, sm := range index.Sitemaps {
		loc := strings.TrimSpace(sm.Loc)
		switch {
		case strings.Contains(loc, "/companies_sitemap"):
			if companies, err = s.companies(ctx, loc, lang); err != nil {
				log.Printf("jobteaser: companies sitemap: %v; no posting is gated on its employer this run", err)
			}
		case strings.Contains(loc, "/job_ads_sitemap"):
			jobSitemaps++
			if err := s.limiter.Wait(ctx); err != nil {
				return nil, err
			}
			doc, err := getSitemap(ctx, s.sitemap, loc)
			if err != nil {
				return nil, fmt.Errorf("jobteaser: %s: %w", loc, err)
			}
			for _, u := range doc.URLs {
				p, ok := jobteaserParsePosting(u, lang)
				if !ok || ids[p.id] {
					continue
				}
				ids[p.id] = true
				out = append(out, p)
			}
		}
	}
	if jobSitemaps == 0 {
		return nil, fmt.Errorf("jobteaser: sitemap index names no job-ads sitemap")
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("jobteaser: sitemaps name no %q posting", lang)
	}
	for i := range out {
		out[i].resolve(companies)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].lastmod.After(out[j].lastmod) })
	return out, nil
}

// companies reads the companies sitemap's slugs for the language.
func (s jobteaser) companies(ctx context.Context, loc, lang string) (map[string]bool, error) {
	if err := s.limiter.Wait(ctx); err != nil {
		return nil, err
	}
	doc, err := getSitemap(ctx, s.sitemap, loc)
	if err != nil {
		return nil, err
	}
	out := make(map[string]bool)
	for _, u := range doc.URLs {
		pu, err := url.Parse(strings.TrimSpace(u.Loc))
		if err != nil || pu.Host != jobteaserHost {
			continue
		}
		if m := jobteaserCompanyPath.FindStringSubmatch(pu.Path); m != nil && m[1] == lang {
			out[m[2]] = true
		}
	}
	return out, nil
}

// jobteaserLD is the part of the posting page's schema.org JobPosting the adapter reads. Its
// jobLocationType is TELECOMMUTE on every page sampled, on-site ones included, so it is not read;
// the page's own remote-policy line is.
type jobteaserLD struct {
	Title              string          `json:"title"`
	Description        string          `json:"description"`
	DatePosted         string          `json:"datePosted"`
	JobLocation        json.RawMessage `json:"jobLocation"`
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

type jobteaserPlace struct {
	Address struct {
		Locality string `json:"addressLocality"`
		Region   string `json:"addressRegion"`
		Country  string `json:"addressCountry"`
	} `json:"address"`
}

// errJobteaserNoPosting marks a page that answered without a JobPosting. It is not the site's
// answer that the posting is gone — an unknown id answers 404, and a challenge page answers
// without one too — so it counts as no answer.
var errJobteaserNoPosting = errors.New("page carries no JobPosting")

const (
	jobteaserRemoteTestID   = "jobad-DetailView__CandidacyDetails__RemotePolicy"
	jobteaserContractTestID = "jobad-DetailView__CandidacyDetails__Contract"
)

// detail reads one posting page. ok=false with a nil error is the site's answer that the posting
// is gone (404/410) or carries no usable posting; an error is the absence of an answer.
func (s jobteaser) detail(ctx context.Context, p jobteaserPosting) (Job, bool, error) {
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
	var ld jobteaserLD
	if !ldJobPosting(root, &ld) {
		return Job{}, false, errJobteaserNoPosting
	}
	job, ok := jobteaserJob(ld, p, jobteaserFact(root, jobteaserRemoteTestID), jobteaserFact(root, jobteaserContractTestID))
	return job, ok, nil
}

// jobteaserFact is the text of the page's posting-fact line with the given test id.
func jobteaserFact(root *html.Node, testID string) string {
	if n := firstByAttr(root, "data-testid", testID); n != nil {
		return strings.Join(strings.Fields(textContent(n)), " ")
	}
	return ""
}

// jobteaserJob maps a posting's JobPosting and fact lines to a Job. ok is false for one with no
// title, employer or body.
func jobteaserJob(ld jobteaserLD, p jobteaserPosting, remote, contract string) (Job, bool) {
	title := strings.Join(strings.Fields(ld.Title), " ")
	company := strings.Join(strings.Fields(ld.HiringOrganization.Name), " ")
	body := sanitizeHTML(ld.Description)
	if title == "" || company == "" || strings.TrimSpace(body) == "" {
		return Job{}, false
	}
	j := Job{
		ExternalID:  p.id,
		URL:         p.url,
		Title:       title,
		Company:     company,
		Description: body,
	}
	var places []string
	for _, pl := range turijobsOneOrMany[jobteaserPlace](ld.JobLocation) {
		city, region := strings.TrimSpace(pl.Address.Locality), strings.TrimSpace(pl.Address.Region)
		switch {
		case city != "" && region != "" && !strings.EqualFold(city, region):
			places = append(places, city+", "+region)
		case city != "":
			places = append(places, city)
		case region != "":
			places = append(places, region)
		}
		if code := location.NormalizeCountry(strings.TrimSpace(pl.Address.Country)); code != "" && !slices.Contains(j.Countries, code) {
			j.Countries = append(j.Countries, code)
		}
	}
	j.Location = strings.Join(places, "; ")
	if t := parseRFC3339(strings.TrimSpace(ld.DatePosted)); t != nil {
		j.PostedAt = NotFuture(t)
	}
	j.WorkMode = jobteaserWorkMode(remote)
	j.Remote = j.WorkMode == "remote"
	j.EmploymentType = jobteaserEmploymentType(contract)
	sal := ld.BaseSalary
	period := jobteaserSalaryPeriod(sal.Value.UnitText)
	if cur := strings.ToUpper(strings.TrimSpace(sal.Currency)); len(cur) == 3 && period != "" &&
		sal.Value.MinValue > 0 && sal.Value.MaxValue >= sal.Value.MinValue {
		lo, hi := int(sal.Value.MinValue), int(sal.Value.MaxValue)
		j.SalaryMin, j.SalaryMax, j.SalaryCurrency, j.SalaryPeriod = &lo, &hi, cur, period
	}
	return j, true
}

// jobteaserWorkMode reads the page's remote-policy line ("Télétravail"): "Partiel" is hybrid,
// "Pas de télétravail" on-site, a full-remote wording remote; "Non spécifié" states nothing.
func jobteaserWorkMode(s string) string {
	v := strings.ToLower(strings.TrimSpace(s))
	switch {
	case v == "partiel":
		return "hybrid"
	case strings.HasPrefix(v, "pas de télétravail"):
		return "onsite"
	case v == "total", v == "complet", strings.Contains(v, "100"), strings.Contains(v, "full remote"):
		return "remote"
	}
	return ""
}

// jobteaserEmploymentType reads the page's contract line ("CDD 12 à 24 mois"): its first word is
// the contract. An internship and work-study ("Alternance") are internships; a fixed-term
// contract and an international volunteer mission (VIE/VIA) are contracts.
func jobteaserEmploymentType(s string) string {
	words := strings.Fields(strings.ToLower(s))
	if len(words) == 0 {
		return ""
	}
	switch words[0] {
	case "cdi":
		return "full_time"
	case "cdd", "vie", "via", "freelance", "intérim", "interim":
		return "contract"
	case "stage", "alternance":
		return "internship"
	}
	return ""
}

func jobteaserSalaryPeriod(unit string) string {
	switch strings.ToUpper(strings.TrimSpace(unit)) {
	case "YEAR", "YEARLY":
		return "year"
	case "MONTH", "MONTHLY":
		return "month"
	case "DAY", "DAILY":
		return "day"
	case "HOUR", "HOURLY":
		return "hour"
	}
	return ""
}

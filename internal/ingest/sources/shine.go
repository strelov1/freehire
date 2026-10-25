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
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/time/rate"
)

// shine adapts Shine.com (www.shine.com), the HT Media group's Indian general job board: ~223k
// public postings, a sixth or so of them technical, many posted by recruitment consultancies.
// It is boardless and an aggregator — the employer is read per posting. See the "Shine traps"
// section of AGENTS.md for the measurements behind each rule.
//
// The posting set comes from the job sitemaps (robots.txt disallows the site's API), which carry
// nothing but each posting's URL, so every fact is bought with the posting page's schema.org
// JobPosting: the adapter hydrates only postings the catalogue lacks, newest id first, and
// refreshes the rest by identity.
type shine struct {
	http         shineHTTP
	limiter      waiter
	maxNewPerRun int
}

// shineHTTP is the transport the adapter needs: the sitemaps as XML and each posting page.
type shineHTTP interface {
	XMLGetter
	HTMLGetter
}

const (
	shineBaseURL      = "https://www.shine.com"
	shineSitemapIndex = shineBaseURL + "/index-all-sitemap.xml"
	// shineSitemapNeedle selects the posting sitemaps among the index's entries
	// (all_jd_sitemap.xml, all_jd_1_sitemap.xml, ...); the rest list search, company, location
	// and blog pages.
	shineSitemapNeedle = "/sitemaps/all_jd"

	// shineRequestInterval paces every request of a run, sitemap and posting page alike, at one
	// a second — a courtesy ceiling toward a large site, not a measured limit.
	shineRequestInterval = time.Second
	// shineDetailWorkers keeps enough pages in flight that the limiter, not latency, sets the pace.
	shineDetailWorkers = 2
	// shineMaxNewPerRun bounds how many unseen postings one run reads: 1,800 pages at one a second
	// is 30 minutes, inside the scheduler's default 2,400 s crawl budget with the ~25 sitemaps.
	// ~10k postings reach the sitemaps a day — under the ~21k twelve two-hourly runs read — and
	// the first runs face the whole catalogue, so it is taken in slices, newest first; a posting
	// past the bound is read by a later run.
	shineMaxNewPerRun = 1800
)

// NewShine builds the adapter over the given client.
func NewShine(c shineHTTP) Source {
	return shine{
		http:         c,
		limiter:      rate.NewLimiter(rate.Every(shineRequestInterval), 1),
		maxNewPerRun: shineMaxNewPerRun,
	}
}

func (shine) Provider() string { return "shine" }

// One national board, no per-tenant id.
func (shine) boardless() {}

// Tens of thousands of employers and consultancies post here, each named per posting.
func (shine) aggregator() {}

// shinePosting is one sitemap entry: the posting's id, its URL, and the title and employer its
// path spells — which is all the gate needs to judge it before its page is bought.
type shinePosting struct {
	id      int64
	url     string
	title   string
	company string
}

func (p shinePosting) externalID() string { return strconv.FormatInt(p.id, 10) }

// shinePostingPath matches a posting URL, /jobs/<title-slug>/<company-slug>/<id>. Both slugs are
// cosmetic — the site serves the posting under any — so the id alone is the identity.
var shinePostingPath = regexp.MustCompile(`^/jobs/([^/]+)/([^/]+)/(\d+)/?$`)

// shineParsePosting reads a sitemap URL, or ok=false when it is not a posting URL.
func shineParsePosting(raw string) (shinePosting, bool) {
	u, err := url.Parse(raw)
	if err != nil {
		return shinePosting{}, false
	}
	m := shinePostingPath.FindStringSubmatch(u.Path)
	if m == nil {
		return shinePosting{}, false
	}
	id, err := strconv.ParseInt(m[3], 10, 64)
	if err != nil || id <= 0 {
		return shinePosting{}, false
	}
	unslug := func(s string) string { return strings.Join(strings.Fields(strings.ReplaceAll(s, "-", " ")), " ") }
	return shinePosting{id: id, url: raw, title: unslug(m[1]), company: unslug(m[2])}, true
}

// Fetch is the list-only fallback HydratingSource specifies: everything is unseen.
func (s shine) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return s.FetchNew(ctx, e, func(string) bool { return false })
}

func (s shine) FetchNew(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	return s.FetchNewGated(ctx, e, seen, DetailGate{})
}

// FetchNewGated lists every posting the sitemaps name, refreshes the ones already stored, and
// reads up to maxNewPerRun of the rest, newest id first. A posting the pipeline will discard
// anyway — its employer already covered by a non-aggregator source, or its title rejected — is
// judged on what its URL spells and yielded body-less without a request: a discarded posting is
// never stored, so without the gate every run would buy it again. A posting whose page cannot be
// read is not yielded; a later run tries again.
func (s shine) FetchNewGated(ctx context.Context, _ CompanyEntry, seen func(externalID string) bool, gate DetailGate) ([]Job, error) {
	postings, err := s.list(ctx)
	if err != nil {
		return nil, err
	}
	var covered map[string]bool
	if gate.Covered != nil {
		companies := make([]string, 0, len(postings))
		for _, p := range postings {
			companies = append(companies, p.company)
		}
		covered = gate.Covered(companies)
	}
	var jobs []Job
	var unseen []shinePosting
	deferred := 0
	for _, p := range postings {
		base := Job{ExternalID: p.externalID(), URL: p.url, Title: p.title, Company: p.company, FallbackCountries: []string{"in"}}
		if seen(base.ExternalID) {
			base.SeenRefresh = true
			jobs = append(jobs, base)
			continue
		}
		if covered[p.company] || (gate.Rejected != nil && gate.Rejected(base)) {
			jobs = append(jobs, base)
			continue
		}
		if len(unseen) >= s.maxNewPerRun {
			deferred++
			continue
		}
		unseen = append(unseen, p)
	}
	var answered atomic.Int64
	read := fetchDetails(unseen, shineDetailWorkers, func(p shinePosting) (Job, bool) {
		job, ok, err := s.detail(ctx, p)
		if err != nil {
			log.Printf("shine: posting %d failed; deferring: %v", p.id, err)
			return Job{}, false
		}
		answered.Add(1)
		return job, ok
	})
	// A crawl that read no page at all is broken, not quiet. "Answered" counts a posting the
	// site reported gone.
	if len(unseen) > 0 && answered.Load() == 0 {
		return nil, fmt.Errorf("shine: requested %d posting pages and none answered", len(unseen))
	}
	if deferred > 0 {
		log.Printf("shine: %d new postings past this run's bound of %d, deferred to later runs", deferred, s.maxNewPerRun)
	}
	return append(jobs, read...), nil
}

// list reads every posting sitemap the index names and returns their postings, newest id first.
// Any sitemap failing fails the crawl: a short list would read a skewed slice and leave the
// postings it missed un-refreshed. The index is regenerated daily and only names the files that
// exist (a file it has dropped answers 500).
func (s shine) list(ctx context.Context) ([]shinePosting, error) {
	if err := s.limiter.Wait(ctx); err != nil {
		return nil, err
	}
	index, err := getSitemap(ctx, s.http, shineSitemapIndex)
	if err != nil {
		return nil, fmt.Errorf("shine: sitemap index: %w", err)
	}
	ids := make(map[int64]bool)
	var out []shinePosting
	files := 0
	for _, sm := range index.Sitemaps {
		if !strings.Contains(sm.Loc, shineSitemapNeedle) {
			continue
		}
		files++
		if err := s.limiter.Wait(ctx); err != nil {
			return nil, err
		}
		doc, err := getSitemap(ctx, s.http, sm.Loc)
		if err != nil {
			return nil, fmt.Errorf("shine: sitemap %s: %w", sm.Loc, err)
		}
		for _, u := range doc.URLs {
			p, ok := shineParsePosting(strings.TrimSpace(u.Loc))
			if !ok || ids[p.id] {
				continue
			}
			ids[p.id] = true
			out = append(out, p)
		}
	}
	if files == 0 {
		return nil, fmt.Errorf("shine: sitemap index names no %s file", shineSitemapNeedle)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].id > out[j].id })
	return out, nil
}

// shineLD is the part of the posting page's schema.org JobPosting the adapter reads.
type shineLD struct {
	Title              string `json:"title"`
	Description        string `json:"description"`
	DatePosted         string `json:"datePosted"`
	EmploymentType     string `json:"employmentType"`
	HiringOrganization struct {
		Name string `json:"name"`
	} `json:"hiringOrganization"`
	ExperienceRequirements struct {
		MonthsOfExperience int `json:"monthsOfExperience"`
	} `json:"experienceRequirements"`
	JobLocation []struct {
		Address struct {
			Locality string `json:"addressLocality"`
			Region   string `json:"addressRegion"`
		} `json:"address"`
	} `json:"jobLocation"`
	BaseSalary struct {
		Currency string `json:"currency"`
		Value    struct {
			UnitText string  `json:"unitText"`
			MinValue float64 `json:"minValue"`
			MaxValue float64 `json:"maxValue"`
		} `json:"value"`
	} `json:"baseSalary"`
}

// detail reads one posting page. ok=false with a nil error is the site's answer that the posting
// is gone (404) or a page carrying no usable posting; an error is the absence of an answer.
// Either way nothing is stored this run.
func (s shine) detail(ctx context.Context, p shinePosting) (Job, bool, error) {
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
	var ld shineLD
	if !ldJobPosting(root, &ld) {
		return Job{}, false, fmt.Errorf("page carries no JobPosting")
	}
	job, ok := shineJob(ld, p)
	return job, ok, nil
}

// shinePlaceNames rewrites the site's own non-places: "All India" names no city, and the
// location dictionary should read the country it means.
var shinePlaceNames = map[string]string{"all india": "India"}

// shineJob maps a posting's JobPosting to a Job. ok is false for one with no title, employer or
// body.
func shineJob(ld shineLD, p shinePosting) (Job, bool) {
	title := strings.Join(strings.Fields(ld.Title), " ")
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
		FallbackCountries: []string{"in"},
	}
	var places []string
	for _, l := range ld.JobLocation {
		city := strings.TrimSpace(l.Address.Locality)
		if mapped, ok := shinePlaceNames[strings.ToLower(city)]; ok {
			city = mapped
		}
		if city == "" {
			continue
		}
		if region := strings.TrimSpace(l.Address.Region); region != "" && !strings.EqualFold(region, city) {
			city += ", " + region
		}
		places = append(places, city)
	}
	j.Location = strings.Join(places, "; ")
	if t, err := time.Parse(time.RFC3339, strings.TrimSpace(ld.DatePosted)); err == nil {
		j.PostedAt = NotFuture(&t)
	}
	switch strings.ToUpper(strings.TrimSpace(ld.EmploymentType)) {
	case "FULL_TIME":
		j.EmploymentType = "full_time"
	case "PART_TIME":
		j.EmploymentType = "part_time"
	case "INTERN", "INTERNSHIP":
		j.EmploymentType = "internship"
	case "CONTRACTOR", "TEMPORARY":
		j.EmploymentType = "contract"
	}
	// monthsOfExperience is the floor of the range the page shows ("4 to 8 Yrs" is 48).
	if years := ld.ExperienceRequirements.MonthsOfExperience / 12; years > 0 {
		j.ExperienceYearsMin = &years
	}
	sal := ld.BaseSalary
	if sal.Currency != "" && strings.EqualFold(sal.Value.UnitText, "YEAR") && sal.Value.MinValue > 0 && sal.Value.MaxValue >= sal.Value.MinValue {
		lo, hi := int(sal.Value.MinValue), int(sal.Value.MaxValue)
		j.SalaryMin, j.SalaryMax = &lo, &hi
		j.SalaryCurrency = strings.ToUpper(strings.TrimSpace(sal.Currency))
		j.SalaryPeriod = "year"
	}
	return j, true
}

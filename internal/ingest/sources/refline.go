package sources

import (
	"cmp"
	"context"
	"encoding/json"
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

// refline adapts Refline (apply.refline.ch), a Swiss e-recruiting ATS strong in the public
// sector: cantons, municipalities, hospitals, the ETH domain (Empa, PSI, WSL), Zürcher
// Kantonalbank. Every customer's portal lives on the one host as /<customer>/search.html and
// each posting as /<customer>/<job>/pub/<publication>/index.html, where one job is published
// several times — once per channel (pub/1, pub/6) and per language (pub/de, pub/it, pub/rm).
//
// robots.txt advertises the posting set as a list of sitemaps (rec01/0 … rec02/9; 1,735 jobs
// across 120 customers on 2026-10-29) and disallows only two of them, which it does not list.
// The adapter is boardless: it reads the sitemaps robots.txt names, collapses each job's
// publications to one, and reads the employer per posting. Refline offers no job API or feed.
//
// Every fact is bought with the posting page's schema.org JobPosting. The adapter hydrates only
// jobs the catalogue lacks, newest first plus a moving backlog window (hydrationLanes), and
// refreshes the rest by identity. It is marked an aggregator because it serves many employers
// under one provider. It does NOT claim fullCatalog: nothing states how many jobs the sitemaps
// should hold.
type refline struct {
	robots         TextGetter
	sitemap        XMLGetter
	pages          HTMLGetter
	limiter        waiter
	fresh, backlog int
	deadlineMargin time.Duration
	now            func() time.Time
}

const (
	reflineHost      = "apply.refline.ch"
	reflineRobotsURL = "https://" + reflineHost + "/robots.txt"

	reflineRequestInterval = time.Second
	reflineDetailWorkers   = 2
	// reflineFreshPerRun and reflineBacklogPerRun bound one run's page reads to twenty minutes;
	// the catalogue of 2026-10-29 (1,735 jobs) is taken in two runs.
	reflineFreshPerRun    = 900
	reflineBacklogPerRun  = 300
	reflineDeadlineMargin = 5 * time.Minute
)

// NewRefline builds the adapter: robots.txt over robots, the sitemaps over sitemap, the posting
// pages over pages.
func NewRefline(robots TextGetter, sitemap XMLGetter, pages HTMLGetter) Source {
	return refline{
		robots:         robots,
		sitemap:        sitemap,
		pages:          pages,
		limiter:        rate.NewLimiter(rate.Every(reflineRequestInterval), 1),
		fresh:          reflineFreshPerRun,
		backlog:        reflineBacklogPerRun,
		deadlineMargin: reflineDeadlineMargin,
		now:            time.Now,
	}
}

func (refline) Provider() string { return "refline" }

// The sitemaps robots.txt names are the whole crawl; no per-customer board is needed.
func (refline) boardless() {}

// Many employers publish through it, each named per posting.
func (refline) aggregator() {}

// reflinePostingPath matches /<customer>/<job>/pub/<publication>/index.html.
var reflinePostingPath = regexp.MustCompile(`^/([0-9]+)/([0-9]+)/pub/([0-9a-z]+)/index\.html$`)

// reflinePosting is one job, pinned to one of its publications.
type reflinePosting struct {
	id      string // <customer>/<job>
	pub     string
	url     string
	lastmod string
}

func reflineParsePosting(raw, lastmod string) (reflinePosting, bool) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "https" || u.Host != reflineHost || u.RawQuery != "" {
		return reflinePosting{}, false
	}
	m := reflinePostingPath.FindStringSubmatch(u.Path)
	if m == nil {
		return reflinePosting{}, false
	}
	return reflinePosting{
		id:      m[1] + "/" + m[2],
		pub:     m[3],
		url:     "https://" + reflineHost + u.Path,
		lastmod: strings.TrimSpace(lastmod),
	}, true
}

// reflinePreferPub orders a job's publications: channel publications (numeric) before language
// ones, each lowest first, so the same publication is chosen on every run.
func reflinePreferPub(a, b string) bool {
	na, errA := strconv.Atoi(a)
	nb, errB := strconv.Atoi(b)
	switch {
	case errA == nil && errB == nil:
		return na < nb
	case errA == nil:
		return true
	case errB == nil:
		return false
	}
	return a < b
}

// Fetch is the list-only fallback HydratingSource specifies: everything is unseen.
func (s refline) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return s.FetchNew(ctx, e, func(string) bool { return false })
}

// FetchNew lists the sitemaps, refreshes the jobs already stored, and reads a bounded slice of
// the rest. A job whose page cannot be read is not yielded; a later run tries again.
func (s refline) FetchNew(ctx context.Context, _ CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	postings, err := s.list(ctx)
	if err != nil {
		return nil, err
	}
	var jobs []Job
	var unseen []reflinePosting
	for _, p := range postings {
		if seen(p.id) {
			jobs = append(jobs, Job{ExternalID: p.id, URL: p.url, SeenRefresh: true})
			continue
		}
		unseen = append(unseen, p)
	}
	picked := hydrationLanes(unseen, s.fresh, s.backlog, s.now())
	var answered, outOfTime atomic.Int64
	read := fetchDetails(picked, reflineDetailWorkers, func(p reflinePosting) (Job, bool) {
		if d, ok := ctx.Deadline(); ok && time.Until(d) < s.deadlineMargin {
			outOfTime.Add(1)
			return Job{}, false
		}
		job, ok, err := s.detail(ctx, p)
		if err != nil {
			log.Printf("refline: posting %s failed; deferring: %v", p.id, err)
			return Job{}, false
		}
		answered.Add(1)
		return job, ok
	})
	if asked := int64(len(picked)) - outOfTime.Load(); asked > 0 && answered.Load() == 0 {
		return nil, fmt.Errorf("refline: requested %d posting pages and none answered", asked)
	}
	if n := outOfTime.Load(); n > 0 {
		log.Printf("refline: %d new jobs left unread as the run neared its deadline, deferred to later runs", n)
	}
	if n := len(unseen) - len(picked); n > 0 {
		log.Printf("refline: %d new jobs past this run's bound of %d, deferred to later runs", n, s.fresh+s.backlog)
	}
	return append(jobs, read...), nil
}

// list reads robots.txt, then every sitemap on the host it names, and collapses each job's
// publications to the preferred one, newest lastmod first. robots.txt failing, or naming no
// sitemap, fails the crawl; so does every sitemap failing. A single sitemap failing is skipped —
// its customers' jobs go unlisted this run, which the company-scoped sweep does not read as a
// closure.
func (s refline) list(ctx context.Context) ([]reflinePosting, error) {
	if err := s.limiter.Wait(ctx); err != nil {
		return nil, err
	}
	robots, err := s.robots.GetText(ctx, reflineRobotsURL)
	if err != nil {
		return nil, fmt.Errorf("refline: robots.txt: %w", err)
	}
	sitemaps := reflineSitemaps(robots)
	if len(sitemaps) == 0 {
		return nil, fmt.Errorf("refline: robots.txt names no sitemap")
	}
	byID := map[string]int{}
	var out []reflinePosting
	var read, failed int
	for _, loc := range sitemaps {
		if err := s.limiter.Wait(ctx); err != nil {
			return nil, err
		}
		doc, err := getSitemap(ctx, s.sitemap, loc)
		if err != nil {
			failed++
			log.Printf("refline: sitemap %s: %v", loc, err)
			continue
		}
		read++
		for _, u := range doc.URLs {
			p, ok := reflineParsePosting(u.Loc, u.LastMod)
			if !ok {
				continue
			}
			if i, dup := byID[p.id]; dup {
				if reflinePreferPub(p.pub, out[i].pub) {
					p.lastmod = max(p.lastmod, out[i].lastmod)
					out[i] = p
				} else {
					out[i].lastmod = max(p.lastmod, out[i].lastmod)
				}
				continue
			}
			byID[p.id] = len(out)
			out = append(out, p)
		}
	}
	if read == 0 {
		return nil, fmt.Errorf("refline: none of %d sitemaps could be read", failed)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("refline: sitemaps list no posting")
	}
	slices.SortStableFunc(out, func(a, b reflinePosting) int { return cmp.Compare(b.lastmod, a.lastmod) })
	return out, nil
}

// reflineSitemaps returns the Sitemap: lines of robots.txt that point at the host, in order.
func reflineSitemaps(robots string) []string {
	var out []string
	for _, line := range strings.Split(robots, "\n") {
		line = strings.TrimSpace(line)
		if len(line) < len("sitemap:") || !strings.EqualFold(line[:len("sitemap:")], "sitemap:") {
			continue
		}
		loc := strings.TrimSpace(line[len("sitemap:"):])
		if u, err := url.Parse(loc); err != nil || u.Scheme != "https" || u.Host != reflineHost || slices.Contains(out, loc) {
			continue
		}
		out = append(out, loc)
	}
	return out
}

// reflineLD is the part of the posting page's schema.org JobPosting the adapter reads.
type reflineLD struct {
	Title              string          `json:"title"`
	Description        string          `json:"description"`
	DatePosted         string          `json:"datePosted"`
	EmploymentType     json.RawMessage `json:"employmentType"`
	JobLocationType    string          `json:"jobLocationType"`
	JobLocation        schemaPlaces    `json:"jobLocation"`
	HiringOrganization struct {
		Name string `json:"name"`
	} `json:"hiringOrganization"`
}

// errReflineNoPosting marks a page that answered without a JobPosting — an unknown job id answers
// 200 with an empty page — which counts as no answer rather than a removal.
var errReflineNoPosting = errors.New("page carries no JobPosting")

// detail reads one posting page. ok=false with a nil error is the site's answer that the job is
// gone (404/410); an error is the absence of an answer.
func (s refline) detail(ctx context.Context, p reflinePosting) (Job, bool, error) {
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
	var ld reflineLD
	if !ldJobPosting(root, &ld) {
		return Job{}, false, errReflineNoPosting
	}
	job, ok := reflineJob(ld, p)
	return job, ok, nil
}

// reflineJob maps a posting's JobPosting to a Job. ok is false for one with no title, employer
// or body. The title arrives HTML-escaped ("BPE, REST &amp; AbaReport").
func reflineJob(ld reflineLD, p reflinePosting) (Job, bool) {
	title := strings.Join(strings.Fields(html.UnescapeString(ld.Title)), " ")
	company := strings.TrimSpace(html.UnescapeString(ld.HiringOrganization.Name))
	body := sanitizeHTML(ld.Description)
	if title == "" || company == "" || strings.TrimSpace(body) == "" {
		return Job{}, false
	}
	j := Job{
		ExternalID:     p.id,
		URL:            p.url,
		Title:          title,
		Company:        company,
		Description:    body,
		PostedAt:       parseRFC3339OrDate(strings.TrimSpace(ld.DatePosted)),
		EmploymentType: schemaFixedTermEmploymentType(ld.EmploymentType),
	}
	j.Location, j.Countries = schemaPlacesLocation(ld.JobLocation)
	if strings.EqualFold(strings.TrimSpace(ld.JobLocationType), "TELECOMMUTE") {
		j.WorkMode, j.Remote = "remote", true
	}
	return j, true
}
